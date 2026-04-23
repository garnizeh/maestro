package maturin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/rs/zerolog/log"
)

const (
	indexLockTimeout      = 30 * time.Second
	indexLockPollInterval = 50 * time.Millisecond
)

// indexPath returns the path to the local OCI image index file.
func (s *Store) indexPath() string {
	return filepath.Join(s.root, "maturin", "index.json")
}

// indexLockPath returns the path to the exclusive lock file for index.json.
func (s *Store) indexLockPath() string {
	return filepath.Join(s.root, "maturin", ".index.lock")
}

// lockIndex acquires an exclusive [syscall.LOCK_EX] flock on the index lock
// file, waiting up to [indexLockTimeout] or until ctx is cancelled.
func (s *Store) lockIndex(ctx context.Context) (*os.File, error) {
	if mkdirErr := s.fs.MkdirAll(filepath.Join(s.root, "maturin"), dirPerm); mkdirErr != nil {
		return nil, fmt.Errorf("create maturin dir: %w", mkdirErr)
	}

	f, openErr := s.fs.OpenFile(s.indexLockPath(), os.O_CREATE|os.O_RDWR, filePerm)
	if openErr != nil {
		return nil, fmt.Errorf("open index lock: %w", openErr)
	}

	if pollErr := s.pollIndexLock(ctx, f); pollErr != nil {
		return nil, pollErr
	}

	return f, nil
}

// pollIndexLock performs the non-blocking flock loop with retries.
func (s *Store) pollIndexLock(ctx context.Context, f *os.File) error {
	deadline, hasDeadline := ctx.Deadline()
	if !hasDeadline {
		deadline = time.Now().Add(indexLockTimeout)
	}

	for {
		//nolint:gosec // G115: Flock requires int; fd fits in int on all supported 64-bit platforms
		lockErr := s.fs.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if lockErr == nil {
			return nil
		}
		if !errors.Is(lockErr, syscall.EWOULDBLOCK) {
			if errClose := f.Close(); errClose != nil {
				log.Debug().
					Err(errClose).
					Msg("maturin: failed to close index lock file after flock error")
			}
			//coverage:ignore non-EWOULDBLOCK requires invalid fd, unreachable after successful OpenFile
			return fmt.Errorf("flock index: %w", lockErr)
		}
		if time.Now().After(deadline) {
			if errClose := f.Close(); errClose != nil {
				log.Debug().
					Err(errClose).
					Msg("maturin: failed to close index lock file after timeout")
			}
			return errors.New("timeout waiting for index lock")
		}
		select {
		case <-ctx.Done():
			if errClose := f.Close(); errClose != nil {
				log.Debug().
					Err(errClose).
					Msg("maturin: failed to close index lock file after context cancellation")
			}
			return ctx.Err()
		case <-time.After(indexLockPollInterval):
		}
	}
}

// unlockIndex releases the exclusive lock held by f and closes the file.
func (s *Store) unlockIndex(f *os.File) error {
	//nolint:gosec // G115: Flock requires int; fd fits in int on all supported 64-bit platforms
	if unlockErr := s.fs.Flock(int(f.Fd()), syscall.LOCK_UN); unlockErr != nil {
		if errClose := f.Close(); errClose != nil {
			log.Debug().
				Err(errClose).
				Msg("maturin: failed to close index lock file after unlock error")
		}
		//coverage:ignore Flock(LOCK_UN) on a valid fd never fails in normal operation
		return fmt.Errorf("unlock index: %w", unlockErr)
	}
	return f.Close()
}

// withIndexLock acquires the exclusive index lock, runs fn, then releases the
// lock. Both the function error and the unlock error are propagated; fn's error
// takes precedence.
func (s *Store) withIndexLock(ctx context.Context, fn func() error) error {
	lockFile, lockErr := s.lockIndex(ctx)
	if lockErr != nil {
		return lockErr
	}
	fnErr := fn()
	unlockErr := s.unlockIndex(lockFile)
	if fnErr != nil {
		return fnErr
	}
	return unlockErr
}

// readIndex reads and parses index.json. Returns an empty valid index if the
// file does not yet exist.
func (s *Store) readIndex() (v1.Index, error) {
	data, readErr := s.fs.ReadFile(s.indexPath())
	if readErr != nil {
		if os.IsNotExist(readErr) {
			idx := v1.Index{Manifests: []v1.Descriptor{}}
			idx.SchemaVersion = 2
			return idx, nil
		}
		return v1.Index{}, fmt.Errorf("read index: %w", readErr)
	}

	var idx v1.Index
	if unmarshalErr := json.Unmarshal(data, &idx); unmarshalErr != nil {
		return v1.Index{}, fmt.Errorf("parse index: %w", unmarshalErr)
	}
	if idx.Manifests == nil {
		idx.Manifests = []v1.Descriptor{}
	}
	return idx, nil
}

// writeIndex atomically writes idx to index.json via a temp file and rename.
func (s *Store) writeIndex(idx v1.Index) error {
	data, marshalErr := json.Marshal(idx)
	if marshalErr != nil {
		// coverage:ignore v1.Index contains only JSON-serializable types
		return fmt.Errorf("marshal index: %w", marshalErr)
	}

	tmp := s.indexPath() + ".tmp"
	if writeErr := s.fs.WriteFile(tmp, data, filePerm); writeErr != nil {
		return fmt.Errorf("write index temp: %w", writeErr)
	}
	if renameErr := s.fs.Rename(tmp, s.indexPath()); renameErr != nil {
		if errRem := s.fs.Remove(tmp); errRem != nil {
			log.Debug().
				Err(errRem).
				Str("path", tmp).
				Msg("maturin: failed to remove stale index temp file")
		}
		return fmt.Errorf("commit index: %w", renameErr)
	}
	return nil
}

// AddToIndex adds desc to the local OCI image index under an exclusive lock.
// If an entry with the same digest already exists it is replaced in-place.
func (s *Store) AddToIndex(ctx context.Context, desc v1.Descriptor) error {
	return s.withIndexLock(ctx, func() error {
		idx, readErr := s.readIndex()
		if readErr != nil {
			return readErr
		}

		replaced := false
		for i, m := range idx.Manifests {
			if m.Digest == desc.Digest {
				idx.Manifests[i] = desc
				replaced = true
				break
			}
		}
		if !replaced {
			idx.Manifests = append(idx.Manifests, desc)
		}

		return s.writeIndex(idx)
	})
}

// RemoveFromIndex removes the entry with the given digest from index.json under
// an exclusive lock. It is a no-op if no such entry exists.
func (s *Store) RemoveFromIndex(ctx context.Context, dgst digest.Digest) error {
	return s.withIndexLock(ctx, func() error {
		idx, readErr := s.readIndex()
		if readErr != nil {
			return readErr
		}

		filtered := make([]v1.Descriptor, 0, len(idx.Manifests))
		for _, m := range idx.Manifests {
			if m.Digest != dgst {
				filtered = append(filtered, m)
			}
		}
		idx.Manifests = filtered

		return s.writeIndex(idx)
	})
}

// ListIndex returns all descriptors tracked in the local OCI image index.
func (s *Store) ListIndex(ctx context.Context) ([]v1.Descriptor, error) {
	var result []v1.Descriptor
	listErr := s.withIndexLock(ctx, func() error {
		idx, readErr := s.readIndex()
		if readErr != nil {
			return readErr
		}
		result = idx.Manifests
		return nil
	})
	if listErr != nil {
		return nil, listErr
	}
	return result, nil
}

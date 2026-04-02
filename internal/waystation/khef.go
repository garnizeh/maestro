// khef.go — flock-based file locking for the Waystation store.
//
// Khef is the Water of Life in the Dark Tower series — the substance that
// connects all things in ka. Similarly, Khef here connects concurrent
// processes safely through shared-state locking.

package waystation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const (
	defaultLockTimeout = 30 * time.Second
	lockDir            = "locks"
	lockPollInterval   = 50 * time.Millisecond
)

// Lock represents a held file lock.
type Lock struct {
	f    *os.File
	path string
}

// lockPath returns the path for a named lock file.
func (s *Store) lockPath(name string) string {
	return filepath.Join(s.root, lockDir, name+".lock")
}

// AcquireLock acquires an exclusive write lock on name, waiting up to timeout.
// Returns a Lock that must be released with Release.
func (s *Store) AcquireLock(ctx context.Context, name string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Join(s.root, lockDir), 0o700); err != nil {
		return nil, fmt.Errorf("create lock dir: %w", err)
	}

	path := s.lockPath(name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file %s: %w", path, err)
	}

	if flockErr := acquireFlockWithContext(ctx, f, syscall.LOCK_EX); flockErr != nil {
		_ = f.Close()
		return nil, fmt.Errorf("acquire write lock %s: %w", name, flockErr)
	}

	return &Lock{f: f, path: path}, nil
}

// AcquireReadLock acquires a shared read lock on name.
func (s *Store) AcquireReadLock(ctx context.Context, name string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Join(s.root, lockDir), 0o700); err != nil {
		return nil, fmt.Errorf("create lock dir: %w", err)
	}

	path := s.lockPath(name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file %s: %w", path, err)
	}

	if flockErr := acquireFlockWithContext(ctx, f, syscall.LOCK_SH); flockErr != nil {
		_ = f.Close()
		return nil, fmt.Errorf("acquire read lock %s: %w", name, flockErr)
	}

	return &Lock{f: f, path: path}, nil
}

// Release releases the file lock.
func (l *Lock) Release() error {
	//nolint:gosec // G115: Flock requires int; fd fits in int on all supported 64-bit platforms
	if err := syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN); err != nil {
		_ = l.f.Close()
		return fmt.Errorf(
			"unlock: %w",
			err,
		) //coverage:ignore Flock(LOCK_UN) on a valid fd never fails in normal operation
	}
	return l.f.Close()
}

// acquireFlockWithContext attempts to acquire a flock, respecting ctx
// acquireFlockWithContext attempts to acquire a POSIX flock (shared or exclusive) on f,
// polling with non-blocking attempts until the lock is obtained, the context is cancelled,
// or a deadline is reached.
// If ctx carries a deadline it is honored; otherwise a default timeout is applied.
// Returns nil on success, ctx.Err() if the context is canceled, or an error if the syscall fails
// or the timeout elapses.
func acquireFlockWithContext(ctx context.Context, f *os.File, how int) error {
	deadline, hasDeadline := ctx.Deadline()
	if !hasDeadline {
		deadline = time.Now().Add(defaultLockTimeout)
	}

	for {
		//nolint:gosec // G115: Flock requires int; fd fits in int on all supported 64-bit platforms
		err := syscall.Flock(int(f.Fd()), how|syscall.LOCK_NB)
		if err == nil {
			return nil // acquired
		}
		if err != syscall.EWOULDBLOCK {
			return fmt.Errorf(
				"flock: %w",
				err,
			) //coverage:ignore non-EWOULDBLOCK requires invalid fd, unreachable after successful OpenFile
		}

		if time.Now().After(deadline) {
			return errors.New("timeout waiting for lock")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(lockPollInterval):
		}
	}
}

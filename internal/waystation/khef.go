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
	f      *os.File
	path   string
	locker Locker
}

// lockPath returns the path for a named lock file.
func (s *Store) lockPath(name string) string {
	return filepath.Join(s.root, lockDir, name+".lock")
}

// AcquireLock acquires an exclusive write lock on name, waiting up to timeout.
// Returns a Lock that must be released with Release.
func (s *Store) AcquireLock(ctx context.Context, name string) (*Lock, error) {
	return s.acquireLockGeneric(ctx, name, syscall.LOCK_EX, "write")
}

// AcquireReadLock acquires a shared read lock on name.
func (s *Store) AcquireReadLock(ctx context.Context, name string) (*Lock, error) {
	return s.acquireLockGeneric(ctx, name, syscall.LOCK_SH, "read")
}

func (s *Store) acquireLockGeneric(
	ctx context.Context,
	name string,
	how int,
	modeName string,
) (*Lock, error) {
	if err := os.MkdirAll(filepath.Join(s.root, lockDir), 0o700); err != nil {
		return nil, fmt.Errorf("create lock dir: %w", err)
	}

	path := s.lockPath(name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file %s: %w", path, err)
	}

	if flockErr := acquireFlockWithContext(ctx, s.locker, f, how); flockErr != nil {
		if closeErr := f.Close(); closeErr != nil {
			return nil, fmt.Errorf("close lock file %s: %w", path, closeErr)
		}
		return nil, fmt.Errorf("acquire %s lock %s: %w", modeName, name, flockErr)
	}

	return &Lock{f: f, path: path, locker: s.locker}, nil
}

// Release releases the file lock.
func (l *Lock) Release() error {
	//nolint:gosec // G115: Flock requires int; fd fits in int on all supported 64-bit platforms
	if err := l.locker.Flock(int(l.f.Fd()), syscall.LOCK_UN); err != nil {
		if closeErr := l.f.Close(); closeErr != nil {
			return fmt.Errorf("close lock file %s: %w", l.path, closeErr)
		}
		return fmt.Errorf("unlock: %w", err)
	}
	return l.f.Close()
}

// acquireFlockWithContext attempts to acquire a flock, respecting ctx
// cancellation. It polls with LOCK_NB so that cancellation is responsive.
func acquireFlockWithContext(ctx context.Context, locker Locker, f *os.File, how int) error {
	deadline, hasDeadline := ctx.Deadline()
	if !hasDeadline {
		deadline = time.Now().Add(defaultLockTimeout)
	}

	for {
		//nolint:gosec // G115: Flock requires int; fd fits in int on all supported 64-bit platforms
		err := locker.Flock(int(f.Fd()), how|syscall.LOCK_NB)
		if err == nil {
			return nil // acquired
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return fmt.Errorf("flock: %w", err)
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

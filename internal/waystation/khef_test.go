package waystation_test

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestAcquireLock_WriteAndRelease(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	lock, err := s.AcquireLock(ctx, "test-resource")
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	if releaseErr := lock.Release(); releaseErr != nil {
		t.Fatalf("Release: %v", releaseErr)
	}
}

func TestAcquireReadLock_WriteAndRelease(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	lock, err := s.AcquireReadLock(ctx, "test-resource")
	if err != nil {
		t.Fatalf("AcquireReadLock: %v", err)
	}
	if releaseErr := lock.Release(); releaseErr != nil {
		t.Fatalf("Release: %v", releaseErr)
	}
}

// TestAcquireLock_Cancelled verifies that a cancelled context causes the lock
// acquisition to fail promptly.
func TestAcquireLock_Cancelled(t *testing.T) {
	s := newStore(t)

	// Hold the write lock from goroutine 1.
	ctx1 := context.Background()
	lock, err := s.AcquireLock(ctx1, "contested")
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	defer func() { _ = lock.Release() }()

	// Goroutine 2 tries to acquire with an already-cancelled context.
	ctx2, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err = s.AcquireLock(ctx2, "contested")
	if err == nil {
		t.Error("expected error when context is already cancelled")
	}
}

// TestAcquireLock_MultipleReaders verifies that multiple readers can hold the
// lock concurrently (shared read lock).
func TestAcquireReadLock_MultipleReaders(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	const readers = 5

	var wg sync.WaitGroup
	errs := make(chan error, readers)

	for range readers {
		wg.Go(func() {
			lock, err := s.AcquireReadLock(ctx, "shared-read")
			if err != nil {
				errs <- err
				return
			}
			time.Sleep(10 * time.Millisecond)
			errs <- lock.Release()
		})
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("reader error: %v", err)
		}
	}
}

// TestAcquireReadLock_Cancelled verifies that a cancelled context causes the
// read lock acquisition to fail promptly.
func TestAcquireReadLock_Cancelled(t *testing.T) {
	s := newStore(t)

	// Hold a write lock so the read lock cannot be acquired immediately.
	ctx1 := context.Background()
	lock, err := s.AcquireLock(ctx1, "read-contested")
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	defer func() { _ = lock.Release() }()

	// Try to acquire a read lock with an already-cancelled context.
	ctx2, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = s.AcquireReadLock(ctx2, "read-contested")
	if err == nil {
		t.Error("expected error when context is already cancelled")
	}
}

// TestAcquireLock_Timeout verifies that a lock request times out when a write
// lock is held and the deadline expires.
func TestAcquireLock_Timeout(t *testing.T) {
	s := newStore(t)

	// Hold the write lock.
	ctx1 := context.Background()
	lock, err := s.AcquireLock(ctx1, "timeout-test")
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	defer func() { _ = lock.Release() }()

	// Try to acquire with a very short deadline.
	ctx2, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = s.AcquireLock(ctx2, "timeout-test")
	if err == nil {
		t.Error("expected timeout error when lock is held")
	}
}

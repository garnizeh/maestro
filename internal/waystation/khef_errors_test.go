package waystation_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rodrigo-baliza/maestro/internal/waystation"
)

// newStoreAt creates a Store at an arbitrary path without calling Init.
func newStoreAt(t *testing.T, root string) *waystation.Store {
	t.Helper()
	return waystation.New(root)
}

func TestAcquireLock_MkdirError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission error as root")
	}
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })

	// Store root inside the unwriteable parent — MkdirAll(locks/) cannot create it.
	s := newStoreAt(t, filepath.Join(parent, "ws"))
	_, err := s.AcquireLock(context.Background(), "test")
	if err == nil {
		t.Error("expected error when lock dir cannot be created")
	}
}

func TestAcquireReadLock_MkdirError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission error as root")
	}
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })

	s := newStoreAt(t, filepath.Join(parent, "ws"))
	_, err := s.AcquireReadLock(context.Background(), "test")
	if err == nil {
		t.Error("expected error when lock dir cannot be created")
	}
}

func TestAcquireLock_OpenFileError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission error as root")
	}
	dir := t.TempDir()
	s := newStoreAt(t, dir)
	// Create the locks dir and make it unwriteable so OpenFile(O_CREATE) fails.
	locksDir := filepath.Join(dir, "locks")
	if err := os.MkdirAll(locksDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locksDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locksDir, 0o700) })

	_, err := s.AcquireLock(context.Background(), "test")
	if err == nil {
		t.Error("expected error when lock file cannot be created in read-only dir")
	}
}

func TestAcquireReadLock_OpenFileError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission error as root")
	}
	dir := t.TempDir()
	s := newStoreAt(t, dir)
	locksDir := filepath.Join(dir, "locks")
	if err := os.MkdirAll(locksDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locksDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locksDir, 0o700) })

	_, err := s.AcquireReadLock(context.Background(), "test")
	if err == nil {
		t.Error("expected error when lock file cannot be created in read-only dir")
	}
}

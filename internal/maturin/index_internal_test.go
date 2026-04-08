package maturin

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestUnlockIndex_InvalidFd(t *testing.T) {
	t.Parallel()
	f, err := os.CreateTemp(t.TempDir(), "unlock-test-")
	if err != nil {
		t.Fatal(err)
	}
	// Close the file so f.Fd() returns the invalid-fd sentinel (maxuint → -1 as int).
	// syscall.Flock(-1, LOCK_UN) returns EBADF, covering the error branch.
	_ = f.Close()

	if unlockErr := unlockIndex(f); unlockErr == nil {
		t.Fatal("expected EBADF error from Flock on closed file, got nil")
	}
}

func TestWriteIndex_RenameError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	s := New(root)

	if mkdirErr := os.MkdirAll(filepath.Join(root, "maturin"), 0o700); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}
	// Create a DIRECTORY at index.json so os.Rename(index.json.tmp, index.json) fails.
	if mkdirErr := os.MkdirAll(filepath.Join(root, "maturin", "index.json"), 0o700); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}

	idx := v1.Index{Manifests: []v1.Descriptor{}}
	idx.SchemaVersion = 2

	err := s.writeIndex(idx)
	if err == nil {
		t.Fatal("expected rename error (index.json is a directory), got nil")
	}
}

// TestUnlockIndex_ValidFd ensures the happy path is covered from within the
// package (the public methods also cover it, but this keeps the internal file self-contained).
func TestUnlockIndex_ValidFd(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	if mkdirErr := os.MkdirAll(filepath.Join(root, "maturin"), 0o700); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}
	lockPath := filepath.Join(root, "maturin", ".index.lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if flockErr := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); flockErr != nil {
		_ = f.Close()
		t.Fatal(flockErr)
	}

	if unlockErr := unlockIndex(f); unlockErr != nil {
		t.Fatalf("unlockIndex on valid fd: %v", unlockErr)
	}
}

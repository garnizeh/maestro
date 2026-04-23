package waystation_test

// Tests for OS-level error paths that require filesystem permission manipulation.
// Each test skips when running as root (permissions have no effect).

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/garnizeh/maestro/internal/waystation"
)

func TestInit_MkdirError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission error as root")
	}
	// Make the parent unwriteable so Init cannot create subdirectories.
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(parent, 0o700); err != nil {
			t.Fatal(err)
		}
	})

	s := waystation.New(filepath.Join(parent, "maestro"))
	if err := s.Init(); err == nil {
		t.Error("expected error when parent directory is not writable")
	}
}

func TestPut_MkdirError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission error as root")
	}
	// Root is a read-only directory — MkdirAll cannot create subdirectories.
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(parent, 0o700); err != nil {
			t.Fatal(err)
		}
	})

	s := waystation.New(parent)
	if err := s.Put("containers", "key", struct{}{}); err == nil {
		t.Error("expected error when collection dir cannot be created")
	}
}

func TestPut_CreateTempError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission error as root")
	}
	dir := t.TempDir()
	s := waystation.New(dir)
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	// Make the containers dir unwriteable so CreateTemp fails.
	collDir := filepath.Join(dir, "containers")
	if err := os.Chmod(collDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(collDir, 0o700); err != nil {
			t.Fatal(err)
		}
	})

	if err := s.Put("containers", "key", struct{}{}); err == nil {
		t.Error("expected error when collection dir is not writable")
	}
}

func TestGet_ReadError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission error as root")
	}
	dir := t.TempDir()
	s := waystation.New(dir)
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	// Write a file then remove read permission.
	path := filepath.Join(dir, "containers", "unreadable.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(path, 0o600); err != nil {
			t.Fatal(err)
		}
	})

	var v struct{}
	err := s.Get("containers", "unreadable", &v)
	if err == nil {
		t.Error("expected error for unreadable file")
	}
	if errors.Is(err, waystation.ErrNotFound) {
		t.Error("got ErrNotFound, expected real OS error")
	}
}

func TestGet_UnmarshalError(t *testing.T) {
	dir := t.TempDir()
	s := waystation.New(dir)
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "containers", "bad.json")
	if err := os.WriteFile(path, []byte(`not-json-at-all`), 0o600); err != nil {
		t.Fatal(err)
	}
	var v struct{}
	if err := s.Get("containers", "bad", &v); err == nil {
		t.Error("expected unmarshal error for invalid JSON")
	}
}

func TestDelete_RemoveError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission error as root")
	}
	dir := t.TempDir()
	s := waystation.New(dir)
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	// Create the file then make its parent dir unwriteable.
	if err := s.Put("containers", "locked", struct{}{}); err != nil {
		t.Fatal(err)
	}
	collDir := filepath.Join(dir, "containers")
	if err := os.Chmod(collDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(collDir, 0o700); err != nil {
			t.Fatal(err)
		}
	})

	err := s.Delete("containers", "locked")
	if err == nil {
		t.Error("expected error when deleting from read-only directory")
	}
	if errors.Is(err, waystation.ErrNotFound) {
		t.Error("got ErrNotFound, expected real OS error")
	}
}

func TestList_SkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	s := waystation.New(dir)
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	// Put a subdirectory and a non-.json file inside containers.
	if err := os.MkdirAll(filepath.Join(dir, "containers", "subdir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "containers", "ignored.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.Put("containers", "valid", struct{}{}); err != nil {
		t.Fatal(err)
	}

	keys, err := s.List("containers")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 1 || keys[0] != "valid" {
		t.Errorf("List = %v, want [valid]", keys)
	}
}

func TestList_ReadDirError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission error as root")
	}
	dir := t.TempDir()
	s := waystation.New(dir)
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	collDir := filepath.Join(dir, "containers")
	if err := os.Chmod(collDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(collDir, 0o700); err != nil {
			t.Fatal(err)
		}
	})

	if _, err := s.List("containers"); err == nil {
		t.Error("expected error for unreadable collection directory")
	}
}

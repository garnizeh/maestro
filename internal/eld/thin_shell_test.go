package eld //nolint:testpackage // thin shell tests need internal access

import (
	"context"
	"os"
	"testing"
)

func TestRealFS(t *testing.T) {
	fs := RealFS{}
	dir := t.TempDir()

	if err := fs.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	f, tempErr := fs.CreateTemp(dir, "base")
	if tempErr != nil {
		t.Fatalf("CreateTemp: %v", tempErr)
	}
	if f == nil {
		t.Fatal("expected non-nil TempFile")
	}
	defer f.Close()

	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := fs.Stat(f.Name()); err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if err := fs.Chmod(f.Name(), 0644); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	newName := f.Name() + ".new"
	if err := fs.Rename(f.Name(), newName); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if err := fs.Remove(newName); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	f2, err := fs.OpenFile(dir, os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if f2 != nil {
		if closeErr := f2.Close(); closeErr != nil {
			t.Fatalf("Close: %v", closeErr)
		}
	}

	f3, err := fs.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if f3 != nil {
		if closeErr := f3.Close(); closeErr != nil {
			t.Fatalf("Close: %v", closeErr)
		}
	}

	if fs.IsNotExist(nil) {
		t.Fatal("expected IsNotExist(nil) to be false")
	}
	if _, absErr := fs.Abs("."); absErr != nil {
		t.Fatalf("Abs: %v", absErr)
	}
}

func TestRealCommander(t *testing.T) {
	cmd := RealCommander{}
	command := cmd.CommandContext(context.Background(), "true")
	if command == nil {
		t.Fatal("expected non-nil command")
	}
	if _, err := cmd.LookPath("true"); err != nil {
		t.Fatalf("LookPath: %v", err)
	}
}

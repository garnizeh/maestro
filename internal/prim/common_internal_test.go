package prim

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareHelper_CreatesFSDirWithTraversalPerms(t *testing.T) {
	root := t.TempDir()
	var fsMode os.FileMode
	m := &mockFS{
		fallback: RealFS{},
		StatFn: func(string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		},
		MkdirAllFn: func(p string, mode os.FileMode) error {
			if filepath.Base(p) == "fs" {
				fsMode = mode
			}
			return os.MkdirAll(p, mode)
		},
		WriteFileFn: os.WriteFile,
	}

	_, err := prepareHelper(
		context.Background(),
		m,
		nopLocker{},
		func(key string) string { return filepath.Join(root, key) },
		func(string) error { return nil },
		func(dir string, _ VFSMeta) error {
			return os.WriteFile(filepath.Join(dir, "meta.json"), []byte("{}"), 0o600)
		},
		func(string, string) ([]Mount, error) { return []Mount{{Type: "bind"}}, nil },
		"snap1",
		"",
	)
	if err != nil {
		t.Fatalf("prepareHelper: %v", err)
	}
	if fsMode != fsDirPerm {
		t.Fatalf("fs dir mode = %04o; want %04o", fsMode, fsDirPerm)
	}
}

type nopLocker struct{}

func (nopLocker) Lock()   {}
func (nopLocker) Unlock() {}

func TestPrepareHelper_StillCreatesWorkDirPrivate(t *testing.T) {
	root := t.TempDir()
	var workMode os.FileMode
	m := &mockFS{
		fallback: RealFS{},
		StatFn: func(string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		},
		MkdirAllFn: func(p string, mode os.FileMode) error {
			if filepath.Base(p) == "work" {
				workMode = mode
			}
			return os.MkdirAll(p, mode)
		},
	}

	_, err := prepareHelper(
		context.Background(),
		m,
		nopLocker{},
		func(key string) string { return filepath.Join(root, key) },
		func(string) error { return nil },
		func(dir string, _ VFSMeta) error {
			return os.WriteFile(filepath.Join(dir, "meta.json"), []byte("{}"), 0o600)
		},
		func(string, string) ([]Mount, error) { return []Mount{{Type: "bind"}}, nil },
		"snap1",
		"",
	)
	if err != nil {
		t.Fatalf("prepareHelper: %v", err)
	}
	if workMode != dirPerm {
		t.Fatalf("work dir mode = %04o; want %04o", workMode, dirPerm)
	}
}

func TestPrepareHelper_PropagatesFSError(t *testing.T) {
	m := &mockFS{
		MkdirAllFn: func(p string, _ os.FileMode) error {
			if filepath.Base(p) == "fs" {
				return errors.New("fs-create-fail")
			}
			return nil
		},
	}

	_, err := prepareHelper(
		context.Background(),
		m,
		nopLocker{},
		func(key string) string { return filepath.Join(t.TempDir(), key) },
		func(string) error { return nil },
		func(string, VFSMeta) error { return nil },
		func(string, string) ([]Mount, error) { return nil, nil },
		"snap1",
		"",
	)
	if err == nil || err.Error() != "prepare snap1: fs-create-fail" {
		t.Fatalf("got err %v; want fs-create-fail", err)
	}
}

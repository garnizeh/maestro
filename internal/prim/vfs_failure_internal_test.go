package prim

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/garnizeh/maestro/pkg/archive"
)

func TestVFS_Prepare_MkdirFail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m := &mockFS{
		fallback: RealFS{},
		MkdirAllFn: func(path string, mode os.FileMode) error {
			if filepath.Base(path) == "fs" {
				return errors.New("mkdir-fs-fail")
			}
			return os.MkdirAll(path, mode)
		},
	}
	v := newVFS(t).WithFS(m)
	_, err := v.Prepare(ctx, "fail1", "")
	if err == nil || !strings.Contains(err.Error(), "mkdir-fs-fail") {
		t.Errorf("got error %v, want substring mkdir-fs-fail", err)
	}
}

func TestVFS_Prepare_CopyDirFail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// We use a real VFS for setup, then mock it
	v := newVFS(t)
	// Setup parent
	_, err := v.Prepare(ctx, "p1-rw", "")
	if err != nil {
		t.Fatalf("failed to prepare: %v", err)
	}
	err = v.Commit(ctx, "p1", "p1-rw")
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	m := &mockFS{
		fallback: RealFS{},
		MkdirAllFn: func(path string, mode os.FileMode) error {
			if filepath.Base(filepath.Dir(path)) == "fail2" && filepath.Base(path) == "fs" {
				return errors.New("copy-dir-fail")
			}
			return os.MkdirAll(path, mode)
		},
	}
	v.WithFS(m)
	_, err = v.Prepare(ctx, "fail2", "p1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestVFS_Prepare_WriteMetaFail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m := &mockFS{
		fallback: RealFS{},
		WriteFileFn: func(path string, data []byte, mode os.FileMode) error {
			if filepath.Base(path) == "meta.json" {
				return errors.New("write-meta-fail")
			}
			return os.WriteFile(path, data, mode)
		},
	}
	v := newVFS(t).WithFS(m)
	_, err := v.Prepare(ctx, "fail3", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestVFS_Prepare_WithParent_CleanupFail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	v := newVFS(t)
	// Setup parent
	_, err := v.Prepare(ctx, "base-rw", "")
	if err != nil {
		t.Fatalf("failed to prepare: %v", err)
	}
	err = v.Commit(ctx, "base", "base-rw")
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	m := &mockFS{
		fallback: RealFS{},
		WalkFn: func(_ string, _ filepath.WalkFunc) error {
			return errors.New("forced-copy-fail")
		},
	}
	v.WithFS(m)
	// v.Prepare with parent calls v.copyDir.
	// If copyDir fails, it should call v.fs.RemoveAll.
	_, err = v.Prepare(ctx, "fail-cleanup", "base")
	if err == nil || !strings.Contains(err.Error(), "forced-copy-fail") {
		t.Errorf("got error %v, want forced-copy-fail", err)
	}
}

func TestVFS_Commit_StatDestFail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	v := newVFS(t)
	_, err := v.Prepare(ctx, "s1", "")
	if err != nil {
		t.Fatalf("failed to prepare: %v", err)
	}
	m := &mockFS{
		fallback: RealFS{},
		StatFn: func(path string) (os.FileInfo, error) {
			if strings.HasSuffix(path, "/c1") {
				return nil, errors.New("stat-dest-fail")
			}
			return os.Stat(path)
		},
	}
	v.WithFS(m)
	err = v.Commit(ctx, "c1", "s1")
	if err == nil || !strings.Contains(err.Error(), "stat-dest-fail") {
		t.Errorf("expected commit error when stat fails on destination; got: %v", err)
	}
}

func TestVFS_Commit_ZombieCleanupFail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	v := newVFS(t)
	_, err := v.Prepare(ctx, "s1", "")
	if err != nil {
		t.Fatalf("failed to prepare: %v", err)
	}
	// Create a "zombie" destination (dir exists but no meta.json)
	err = os.MkdirAll(v.snapshotDir("zombie"), 0700)
	if err != nil {
		t.Fatalf("failed to prepare: %v", err)
	}
	m := &mockFS{
		fallback:   RealFS{},
		MkdirAllFn: os.MkdirAll,
		readMetaFn: func(f string) ([]byte, error) {
			if strings.Contains(f, "zombie") {
				return nil, os.ErrNotExist
			}
			return os.ReadFile(f)
		},
		removeAllErr: errors.New("remove-zombie-fail"),
	}
	v.WithFS(m)
	// v.readMeta will fail for zombie, then it tries to RemoveAll
	err = v.Commit(ctx, "zombie", "s1")
	if err == nil || !strings.Contains(err.Error(), "remove-zombie-fail") {
		t.Errorf("got error %v, want remove-zombie-fail", err)
	}
}

func TestVFS_Prepare_WithParent_DeepCleanupFail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	v := newVFS(t)
	_, err := v.Prepare(ctx, "base-rw", "")
	if err != nil {
		t.Fatalf("failed to prepare: %v", err)
	}
	err = v.Commit(ctx, "base", "base-rw")
	if err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	m := &mockFS{
		fallback: RealFS{},
		WalkFn: func(_ string, _ filepath.WalkFunc) error {
			return errors.New("copy-fail")
		},
		removeAllErr: errors.New("cleanup-fail"),
	}
	v.WithFS(m)
	_, err = v.Prepare(ctx, "any", "base")
	if err == nil || !strings.Contains(err.Error(), "copy-fail") {
		t.Errorf("got error %v, want copy-fail", err)
	}
}

func TestVFS_Commit_WriteMetaFail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	v := newVFS(t)
	_, err := v.Prepare(ctx, "s1", "")
	if err != nil {
		t.Fatalf("failed to prepare: %v", err)
	}
	m := &mockFS{
		fallback: RealFS{},
		WriteFileFn: func(path string, data []byte, mode os.FileMode) error {
			if strings.HasSuffix(path, "meta.json") {
				return errors.New("write-meta-fail")
			}
			return os.WriteFile(path, data, mode)
		},
	}
	v.WithFS(m)
	err = v.Commit(ctx, "c1", "s1")
	if err == nil || !strings.Contains(err.Error(), "write-meta-fail") {
		t.Errorf("got error %v, want write-meta-fail", err)
	}
}

func TestVFS_Remove_Failures(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("StatFail", func(t *testing.T) {
		t.Parallel()
		v := newVFS(t)
		_, err := v.Prepare(ctx, "s1", "")
		if err != nil {
			t.Fatalf("failed to prepare: %v", err)
		}
		m := &mockFS{
			fallback: RealFS{},
			StatFn: func(path string) (os.FileInfo, error) {
				if filepath.Base(path) == "s1" {
					return nil, errors.New("stat-fail")
				}
				return os.Stat(path)
			},
		}
		v.WithFS(m)
		err = v.Remove(ctx, "s1")
		if err == nil {
			t.Error("expected error on stat failure")
		}
	})

	t.Run("RemoveAllFail", func(t *testing.T) {
		t.Parallel()
		v := newVFS(t)
		_, err := v.Prepare(ctx, "s1", "")
		if err != nil {
			t.Fatalf("failed to prepare: %v", err)
		}
		m := &mockFS{
			fallback:     RealFS{},
			StatFn:       os.Stat,
			removeAllErr: errors.New("remove-all-fail"),
		}
		v.WithFS(m)
		err = v.Remove(ctx, "s1")
		if err == nil {
			t.Error("expected error on removeAll failure")
		}
	})
}

func TestVFS_Walk_Failures(t *testing.T) {
	t.Parallel()
	t.Run("SkipNonDir", func(t *testing.T) {
		t.Parallel()
		v := newVFS(t)
		err := os.WriteFile(filepath.Join(v.snapshotsDir(), "notadir"), nil, 0644)
		if err != nil {
			t.Fatalf("failed to prepare: %v", err)
		}
		count := 0
		err = v.Walk(context.Background(), func(Info) error {
			count++
			return nil
		})
		if err != nil {
			t.Fatalf("failed to walk: %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 snapshots walked, got %d", count)
		}
	})
}

func TestVFS_Usage_Failures(t *testing.T) {
	t.Parallel()
	t.Run("StatFail", func(t *testing.T) {
		t.Parallel()
		v := newVFS(t)
		_, err := v.Prepare(context.Background(), "s1", "")
		if err != nil {
			t.Fatalf("failed to prepare: %v", err)
		}
		m := &mockFS{
			fallback: RealFS{},
			statErr:  errors.New("stat-fail"),
		}
		v.WithFS(m)
		_, err = v.Usage(context.Background(), "s1")
		if err == nil {
			t.Error("expected error on stat failure")
		}
	})

	t.Run("WalkDirFail", func(t *testing.T) {
		t.Parallel()
		v := newVFS(t)
		_, err := v.Prepare(context.Background(), "s1", "")
		if err != nil {
			t.Fatalf("failed to prepare: %v", err)
		}
		m := &mockFS{
			fallback:   RealFS{},
			StatFn:     os.Stat,
			walkDirErr: errors.New("walkdir-fail"),
		}
		v.WithFS(m)
		_, err = v.Usage(context.Background(), "s1")
		if err == nil || !strings.Contains(err.Error(), "walkdir-fail") {
			t.Errorf("got error %v, want walkdir-fail", err)
		}
	})

	t.Run("InfoFail", func(t *testing.T) {
		t.Parallel()
		v := newVFS(t)
		_, err := v.Prepare(context.Background(), "s1", "")
		if err != nil {
			t.Fatalf("failed to prepare: %v", err)
		}
		m := &mockFS{
			fallback: RealFS{},
			StatFn:   os.Stat,
			WalkDirFn: func(path string, fn fs.WalkDirFunc) error {
				entry := &mockDirEntry{name: "failinfo", isDir: false}
				return fn(filepath.Join(path, "failinfo"), entry, nil)
			},
		}
		v.WithFS(m)
		_, err = v.Usage(context.Background(), "s1")
		if err == nil || !strings.Contains(err.Error(), "info-fail") {
			t.Errorf("got error %v, want info-fail", err)
		}
	})
}

type mockDirEntry struct {
	name  string
	isDir bool
}

func (m *mockDirEntry) Name() string               { return m.name }
func (m *mockDirEntry) IsDir() bool                { return m.isDir }
func (m *mockDirEntry) Type() fs.FileMode          { return 0 }
func (m *mockDirEntry) Info() (fs.FileInfo, error) { return nil, errors.New("info-fail") }

func TestVFS_Helpers(t *testing.T) {
	t.Parallel()
	v := newVFS(t)
	if v.WritableDir("s1") == "" {
		t.Error("WritableDir returned empty")
	}
	if v.WhiteoutFormat() != archive.WhiteoutVFS {
		t.Errorf("got whiteout format %v, want %v", v.WhiteoutFormat(), archive.WhiteoutVFS)
	}
}

func TestVFS_CopySymlink_Failures(t *testing.T) {
	t.Parallel()
	t.Run("ReadlinkFail", func(t *testing.T) {
		t.Parallel()
		v := newVFS(t)
		m := &mockFS{
			fallback:    RealFS{},
			readlinkErr: errors.New("readlink-fail"),
		}
		v.WithFS(m)
		err := v.copySymlink("src", "dst")
		if err == nil {
			t.Error("expected error on readlink failure")
		}
	})

	t.Run("SymlinkFail", func(t *testing.T) {
		t.Parallel()
		v := newVFS(t)
		m := &mockFS{
			fallback:    RealFS{},
			readlinkRes: "target",
			symlinkErr:  errors.New("symlink-fail"),
		}
		v.WithFS(m)
		err := v.copySymlink("src", "dst")
		if err == nil {
			t.Error("expected error on symlink failure")
		}
	})
}

func TestVFS_CopyFile_Failures(t *testing.T) {
	t.Parallel()
	t.Run("OpenSrcFail", func(t *testing.T) {
		t.Parallel()
		v := newVFS(t)
		m := &mockFS{
			fallback: RealFS{},
			openErr:  errors.New("open-fail"),
		}
		v.WithFS(m)
		err := v.copyFile("src", "dst")
		if err == nil || !strings.Contains(err.Error(), "open-fail") {
			t.Errorf("got error %v, want open-fail", err)
		}
	})

	t.Run("FileStatFail", func(t *testing.T) {
		t.Parallel()
		v := newVFS(t)
		tmp, err := os.CreateTemp(t.TempDir(), "vfs-filestat-fail-*")
		if err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}
		defer os.Remove(tmp.Name())
		m := &mockFS{
			fallback: RealFS{},
			OpenFn: func(_ string) (*os.File, error) {
				return os.Open(tmp.Name())
			},
			FileStatFn: func(_ *os.File) (os.FileInfo, error) {
				return nil, errors.New("filestat-fail")
			},
		}
		v.WithFS(m)
		err = v.copyFile(tmp.Name(), "dst")
		if err == nil || !strings.Contains(err.Error(), "filestat-fail") {
			t.Errorf("got error %v, want filestat-fail", err)
		}
	})

	t.Run("OpenFileDstFail", func(t *testing.T) {
		t.Parallel()
		v := newVFS(t)
		tmp, err := os.CreateTemp(t.TempDir(), "vfs-openfile-fail-*")
		if err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}
		defer os.Remove(tmp.Name())
		m := &mockFS{
			fallback: RealFS{},
			OpenFn: func(_ string) (*os.File, error) {
				return os.Open(tmp.Name())
			},
			openFileErr: errors.New("openfile-fail"),
		}
		v.WithFS(m)
		err = v.copyFile(tmp.Name(), "dst")
		if err == nil || !strings.Contains(err.Error(), "openfile-fail") {
			t.Errorf("got error %v, want openfile-fail", err)
		}
	})

	t.Run("CopyIOFail", func(t *testing.T) {
		t.Parallel()
		v := newVFS(t)
		tmp, err := os.CreateTemp(t.TempDir(), "vfs-copy-fail-*")
		if err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}
		defer os.Remove(tmp.Name())
		m := &mockFS{
			fallback: RealFS{},
			OpenFn: func(_ string) (*os.File, error) {
				return os.Open(tmp.Name())
			},
			OpenFileFn: func(_ string, _ int, _ os.FileMode) (*os.File, error) {
				return os.CreateTemp(t.TempDir(), "dst-*")
			},
			CopyFn: func(_ io.Writer, _ io.Reader) (int64, error) {
				return 0, errors.New("copy-io-fail")
			},
		}
		v.WithFS(m)
		err = v.copyFile(tmp.Name(), "dst")
		if err == nil || !strings.Contains(err.Error(), "copy-io-fail") {
			t.Errorf("got error %v, want copy-io-fail", err)
		}
	})
}

func TestVFS_hasDependents_Failure(t *testing.T) {
	t.Parallel()
	t.Run("ReadDirFail", func(t *testing.T) {
		t.Parallel()
		v := newVFS(t)
		m := &mockFS{
			fallback:   RealFS{},
			readDirErr: errors.New("readdir-fail"),
		}
		v.WithFS(m)
		_, err := v.hasDependents("k1")
		if err == nil {
			t.Error("expected error on readdir failure in hasDependents")
		}
	})

	t.Run("ReadMetaFailContinue", func(t *testing.T) {
		t.Parallel()
		v2 := newVFS(t)
		_, err := v2.Prepare(context.Background(), "s1", "")
		if err != nil {
			t.Fatalf("failed to prepare: %v", err)
		}
		// Corrupt a meta in a sub-directory
		metaPath := filepath.Join(v2.snapshotDir("s1"), "meta.json")
		err = os.WriteFile(metaPath, []byte("corrupt"), 0644)
		if err != nil {
			t.Fatalf("failed to write file: %v", err)
		}

		has2, err2 := v2.hasDependents("none")
		if err2 != nil {
			t.Fatalf("hasDependents should not fail on meta corrupt: %v", err2)
		}
		if has2 {
			t.Error("should not have dependents")
		}
	})
}

func TestVFS_CopyDir_Failures(t *testing.T) {
	t.Parallel()
	t.Run("MkdirFail", func(t *testing.T) {
		t.Parallel()
		v := newVFS(t)
		m := &mockFS{
			fallback:    RealFS{},
			WalkFn:      filepath.Walk,
			mkdirAllErr: errors.New("mkdir-fail"),
		}
		v.WithFS(m)

		src := t.TempDir()
		err := os.Mkdir(filepath.Join(src, "sub"), 0700)
		if err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}

		err = v.copyDir(src, t.TempDir())
		if err == nil {
			t.Error("expected error on mkdir failure in copyDir")
		}
	})

	t.Run("WalkFail", func(t *testing.T) {
		t.Parallel()
		v := newVFS(t)
		m := &mockFS{
			fallback: RealFS{},
			walkErr:  errors.New("walk-fail"),
		}
		v.WithFS(m)
		err := v.copyDir("src", "dst")
		if err == nil || !strings.Contains(err.Error(), "walk-fail") {
			t.Errorf("got error %v, want walk-fail", err)
		}
	})

	t.Run("WalkCallbackFail", func(t *testing.T) {
		t.Parallel()
		v := newVFS(t)
		m := &mockFS{
			fallback: RealFS{},
			WalkFn: func(_ string, fn filepath.WalkFunc) error {
				// Trigger the inner if err != nil branch
				return fn("any", nil, errors.New("callback-err"))
			},
		}
		v.WithFS(m)
		err := v.copyDir("src", "dst")
		if err == nil || !strings.Contains(err.Error(), "callback-err") {
			t.Errorf("got error %v, want callback-err", err)
		}
	})
}

func TestVFS_ReadMeta_Failures(t *testing.T) {
	t.Parallel()
	v := newVFS(t)
	m := &mockFS{
		fallback:    RealFS{},
		readFileErr: fs.ErrPermission,
	}
	v.WithFS(m)
	_, err := v.readMeta("key")
	if err == nil {
		t.Error("expected error on readFile failure")
	}
}

func TestVFS_Usage_InfoFail_Alt(t *testing.T) {
	t.Parallel()
	v := newVFS(t)
	_, err := v.Prepare(context.Background(), "s1", "")
	if err != nil {
		t.Fatalf("failed to prepare: %v", err)
	}
	m := &mockFS{
		fallback: RealFS{},
		StatFn:   os.Stat,
		WalkDirFn: func(path string, fn fs.WalkDirFunc) error {
			entry := &mockDirEntry{name: "failinfo", isDir: false}
			return fn(filepath.Join(path, "failinfo"), entry, nil)
		},
	}
	v.WithFS(m)
	_, err = v.Usage(context.Background(), "s1")
	if err == nil || !strings.Contains(err.Error(), "info-fail") {
		t.Errorf("got error %v, want info-fail", err)
	}
}

func TestVFS_CopyDir_UnsupportedType(t *testing.T) {
	t.Parallel()
	v := newVFS(t)
	m := &mockFS{
		WalkFn: func(_ string, fn filepath.WalkFunc) error {
			info := &mockFileInfo{mode: os.ModeDevice}
			return fn("devnode", info, nil)
		},
	}
	v.WithFS(m)
	err := v.copyDir("src", "dst")
	if err != nil {
		t.Errorf("expected success (skipping unsupported types) in copyDir; got %v", err)
	}
}

type mockFileInfo struct {
	os.FileInfo

	mode os.FileMode
}

func (m *mockFileInfo) Mode() os.FileMode { return m.mode }
func (m *mockFileInfo) IsDir() bool       { return m.mode.IsDir() }

func TestVFS_Walk_ReadDirFail(t *testing.T) {
	t.Parallel()
	m := &mockFS{
		readDirErr: errors.New("walk-readdir-fail"),
	}
	v := newVFS(t).WithFS(m)
	err := v.Walk(context.Background(), func(Info) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "walk-readdir-fail") {
		t.Errorf("got error %v, want walk-readdir-fail", err)
	}
}

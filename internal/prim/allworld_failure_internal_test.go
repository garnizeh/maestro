package prim

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAllWorld_Prepare_FsFailures(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("MkdirFsFail", func(t *testing.T) {
		t.Parallel()
		a, err := NewAllWorld(t.TempDir())
		if err != nil {
			t.Fatalf("NewAllWorld: %v", err)
		}
		m := &mockFS{
			fallback: RealFS{},
			MkdirAllFn: func(path string, mode os.FileMode) error {
				if filepath.Base(path) == "fs" {
					return errors.New("mkdir-fs-fail")
				}
				return os.MkdirAll(path, mode)
			},
		}
		a.WithFS(m)
		_, err = a.Prepare(ctx, "fail-fs", "")
		if err == nil || !strings.Contains(err.Error(), "mkdir-fs-fail") {
			t.Errorf("got error %v, want mkdir-fs-fail", err)
		}
	})

	t.Run("MkdirAllDirFail", func(t *testing.T) {
		t.Parallel()
		a, err := NewAllWorld(t.TempDir())
		if err != nil {
			t.Fatalf("NewAllWorld: %v", err)
		}
		m := &mockFS{
			fallback: RealFS{},
			MkdirAllFn: func(path string, mode os.FileMode) error {
				if filepath.Base(path) == "work" {
					return errors.New("mkdir-work-fail")
				}
				return os.MkdirAll(path, mode)
			},
		}
		a.WithFS(m)
		_, err = a.Prepare(ctx, "fail1", "")
		if err == nil || !strings.Contains(err.Error(), "mkdir-work-fail") {
			t.Errorf("got error %v, want mkdir-work-fail", err)
		}
	})
}

func TestAllWorld_Prepare_MetaFailures(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("WriteMetaFail", func(t *testing.T) {
		t.Parallel()
		a, err := NewAllWorld(t.TempDir())
		if err != nil {
			t.Fatalf("NewAllWorld: %v", err)
		}
		m := &mockFS{
			fallback: RealFS{},
			WriteFileFn: func(path string, data []byte, mode os.FileMode) error {
				if filepath.Base(path) == "meta.json" {
					return errors.New("write-meta-fail")
				}
				return os.WriteFile(path, data, mode)
			},
		}
		a.WithFS(m)
		_, err = a.Prepare(ctx, "fail2", "")
		if err == nil || !strings.Contains(err.Error(), "write-meta-fail") {
			t.Errorf("got error %v, want write-meta-fail", err)
		}
	})
}

func TestAllWorld_View_Failures(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("MkdirAllDirFail", func(t *testing.T) {
		t.Parallel()
		a, err := NewAllWorld(t.TempDir())
		if err != nil {
			t.Fatalf("NewAllWorld: %v", err)
		}
		m := &mockFS{
			fallback: RealFS{},
			MkdirAllFn: func(path string, mode os.FileMode) error {
				if filepath.Base(path) == "fs" {
					return errors.New("mkdir-fs-fail")
				}
				return os.MkdirAll(path, mode)
			},
		}
		a.WithFS(m)
		_, err = a.View(ctx, "fail1", "")
		if err == nil || !strings.Contains(err.Error(), "mkdir-fs-fail") {
			t.Errorf("got error %v, want mkdir-fs-fail", err)
		}
	})

	t.Run("WriteMetaFail", func(t *testing.T) {
		t.Parallel()
		a, err := NewAllWorld(t.TempDir())
		if err != nil {
			t.Fatalf("NewAllWorld: %v", err)
		}
		m := &mockFS{
			fallback: RealFS{},
			WriteFileFn: func(path string, data []byte, mode os.FileMode) error {
				if filepath.Base(path) == "meta.json" {
					return errors.New("write-meta-fail")
				}
				return os.WriteFile(path, data, mode)
			},
		}
		a.WithFS(m)
		_, err = a.View(ctx, "fail2", "")
		if err == nil || !strings.Contains(err.Error(), "write-meta-fail") {
			t.Errorf("got error %v, want write-meta-fail", err)
		}
	})
}

func TestAllWorld_Commit_StatFailures(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("StatDest", func(t *testing.T) {
		t.Parallel()
		a, err := NewAllWorld(t.TempDir())
		if err != nil {
			t.Fatalf("NewAllWorld: %v", err)
		}
		_, err = a.Prepare(ctx, "s1", "")
		if err != nil {
			t.Fatalf("Prepare: %v", err)
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
		a.WithFS(m)
		err = a.Commit(ctx, "c1", "s1")
		if err == nil || !strings.Contains(err.Error(), "stat-dest-fail") {
			t.Errorf("got error %v, want stat-dest-fail", err)
		}
	})
}

func TestAllWorld_Commit_WriteFailures(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("WriteMeta", func(t *testing.T) {
		t.Parallel()
		a, err := NewAllWorld(t.TempDir())
		if err != nil {
			t.Fatalf("NewAllWorld: %v", err)
		}
		_, err = a.Prepare(ctx, "s1", "")
		if err != nil {
			t.Fatalf("Prepare: %v", err)
		}
		m := &mockFS{
			fallback:   RealFS{},
			MkdirAllFn: os.MkdirAll,
			WriteFileFn: func(path string, data []byte, mode os.FileMode) error {
				if strings.Contains(path, "/s1/meta.json") {
					return errors.New("write-meta-fail")
				}
				return os.WriteFile(path, data, mode)
			},
		}
		a.WithFS(m)
		err = a.Commit(ctx, "c1", "s1")
		if err == nil || !strings.Contains(err.Error(), "write-meta-fail") {
			t.Errorf("got error %v, want write-meta-fail", err)
		}
	})

	t.Run("Rename", func(t *testing.T) {
		t.Parallel()
		a, err := NewAllWorld(t.TempDir())
		if err != nil {
			t.Fatalf("NewAllWorld: %v", err)
		}
		_, err = a.Prepare(ctx, "s1", "")
		if err != nil {
			t.Fatalf("Prepare: %v", err)
		}
		m := &mockFS{
			fallback:   RealFS{},
			MkdirAllFn: os.MkdirAll,
			renameErr:  errors.New("rename-fail"),
		}
		a.WithFS(m)
		err = a.Commit(ctx, "c1", "s1")
		if err == nil || !strings.Contains(err.Error(), "rename-fail") {
			t.Errorf("got error %v, want rename-fail", err)
		}
	})
}

func TestAllWorld_Commit_LogicalFailures(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("DestExists", func(t *testing.T) {
		t.Parallel()
		a, err := NewAllWorld(t.TempDir())
		if err != nil {
			t.Fatalf("NewAllWorld: %v", err)
		}
		_, err = a.Prepare(ctx, "exist", "")
		if err != nil {
			t.Fatalf("Prepare: %v", err)
		}
		_, err = a.Prepare(ctx, "s1", "")
		if err != nil {
			t.Fatalf("Prepare: %v", err)
		}
		err = a.Commit(ctx, "exist", "s1")
		if err == nil || !errors.Is(err, ErrSnapshotAlreadyExists) {
			t.Errorf("got error %v, want ErrSnapshotAlreadyExists", err)
		}
	})

	t.Run("ZombieCleanup", func(t *testing.T) {
		t.Parallel()
		a, err := NewAllWorld(t.TempDir())
		if err != nil {
			t.Fatalf("NewAllWorld: %v", err)
		}
		zDir := a.snapshotDir("zombie")
		err = os.MkdirAll(zDir, 0700)
		if err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		_, err = a.Prepare(ctx, "s1", "")
		if err != nil {
			t.Fatalf("Prepare: %v", err)
		}
		err = a.Commit(ctx, "zombie", "s1")
		if err == nil || !errors.Is(err, ErrSnapshotAlreadyExists) {
			t.Errorf("got error %v, want ErrSnapshotAlreadyExists", err)
		}
	})
}

func TestAllWorld_Remove_Failures(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("HasDependentsFail", func(t *testing.T) {
		t.Parallel()
		a, err := NewAllWorld(t.TempDir())
		if err != nil {
			t.Fatalf("NewAllWorld: %v", err)
		}
		m := &mockFS{
			fallback:   RealFS{},
			readDirErr: errors.New("readdir-fail"),
		}
		a.WithFS(m)
		err = a.Remove(ctx, "any")
		if err == nil || !strings.Contains(err.Error(), "readdir-fail") {
			t.Errorf("got error %v, want readdir-fail", err)
		}
	})

	t.Run("RemoveAllFail", func(t *testing.T) {
		t.Parallel()
		a, err := NewAllWorld(t.TempDir())
		if err != nil {
			t.Fatalf("NewAllWorld: %v", err)
		}
		_, err = a.Prepare(ctx, "s1", "")
		if err != nil {
			t.Fatalf("Prepare: %v", err)
		}
		m := &mockFS{
			fallback:     RealFS{},
			StatFn:       os.Stat,
			removeAllErr: errors.New("remove-all-fail"),
		}
		a.WithFS(m)
		err = a.Remove(ctx, "s1")
		if err == nil || !strings.Contains(err.Error(), "remove-all-fail") {
			t.Errorf("got error %v, want remove-all-fail", err)
		}
	})
}

func TestAllWorld_Walk_Failures(t *testing.T) {
	t.Parallel()
	t.Run("ReadDirFail", func(t *testing.T) {
		t.Parallel()
		a, err := NewAllWorld(t.TempDir())
		if err != nil {
			t.Fatalf("NewAllWorld: %v", err)
		}
		m := &mockFS{
			readDirErr: errors.New("readdir-fail"),
		}
		a.WithFS(m)
		err = a.Walk(context.Background(), func(Info) error { return nil })
		if err == nil || !strings.Contains(err.Error(), "readdir-fail") {
			t.Errorf("got error %v, want readdir-fail", err)
		}
	})

	t.Run("SkipNonDir", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		a, err := NewAllWorld(root)
		if err != nil {
			t.Fatalf("NewAllWorld: %v", err)
		}
		err = os.WriteFile(filepath.Join(a.snapshotsDir(), "somefile"), nil, 0644)
		if err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		count := 0
		err = a.Walk(context.Background(), func(Info) error {
			count++
			return nil
		})
		if err != nil {
			t.Fatalf("Walk: %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 snapshots walked, got %d", count)
		}
	})
}

func TestAllWorld_Usage_Failures(t *testing.T) {
	t.Parallel()
	t.Run("UsageCallbackFail", func(t *testing.T) {
		t.Parallel()
		a, err := NewAllWorld(t.TempDir())
		if err != nil {
			t.Fatalf("NewAllWorld: %v", err)
		}
		m := &mockFS{
			fallback: RealFS{},
			WalkDirFn: func(_ string, fn fs.WalkDirFunc) error {
				return fn("any", nil, errors.New("usage-callback-fail"))
			},
		}
		a.WithFS(m)
		_, err = a.Usage(context.Background(), "any")
		if err == nil || !strings.Contains(err.Error(), "usage-callback-fail") {
			t.Errorf("got error %v, want usage-callback-fail", err)
		}
	})

	t.Run("UsageInfoFail", func(t *testing.T) {
		t.Parallel()
		a, err := NewAllWorld(t.TempDir())
		if err != nil {
			t.Fatalf("NewAllWorld: %v", err)
		}
		m := &mockFS{
			fallback: RealFS{},
			WalkDirFn: func(_ string, fn fs.WalkDirFunc) error {
				entry := &mockDirEntry{name: "failinfo", isDir: false}
				return fn("failinfo", entry, nil)
			},
		}
		a.WithFS(m)
		_, err = a.Usage(context.Background(), "any")
		if err == nil || !strings.Contains(err.Error(), "info-fail") {
			t.Errorf("got error %v, want info-fail", err)
		}
	})
}

func TestProbeOverlay_Failures(t *testing.T) {
	t.Parallel()
	t.Run("MkdirFail", func(t *testing.T) {
		t.Parallel()
		// We can use a path where I don't have permissions or it's a file.
		tmp := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(tmp, nil, 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		err := ProbeOverlay(context.Background(), tmp, nil)
		if err == nil {
			t.Error("expected error probing on a file")
		}
	})

	t.Run("MountFail", func(t *testing.T) {
		t.Parallel()
		m := &mockMounter{mountErr: errors.New("mount-fail")}
		err := ProbeOverlay(context.Background(), t.TempDir(), m)
		if err == nil || !strings.Contains(err.Error(), "mount-fail") {
			t.Errorf("got error %v, want mount-fail", err)
		}
	})
}

func TestAllWorld_Mounts_Failure(t *testing.T) {
	t.Parallel()
	a, err := NewAllWorld(t.TempDir())
	if err != nil {
		t.Fatalf("NewAllWorld: %v", err)
	}
	m := &mockFS{
		readFileErr: errors.New("read-meta-fail"),
	}
	a.WithFS(m)
	_, err = a.mounts("child", "parent")
	if err == nil || !strings.Contains(err.Error(), "read-meta-fail") {
		t.Errorf("got error %v, want read-meta-fail", err)
	}
}

func TestAllWorld_hasDependents_Failure(t *testing.T) {
	t.Parallel()
	a, err := NewAllWorld(t.TempDir())
	if err != nil {
		t.Fatalf("NewAllWorld: %v", err)
	}
	m := &mockFS{
		readDirErr: errors.New("readdir-fail"),
	}
	a.WithFS(m)
	_, err = a.hasDependents("k1")
	if err == nil || !strings.Contains(err.Error(), "readdir-fail") {
		t.Errorf("got error %v, want readdir-fail", err)
	}
}

func TestAllWorld_hasDependents_MetaFail(t *testing.T) {
	t.Parallel()
	a, err := NewAllWorld(t.TempDir())
	if err != nil {
		t.Fatalf("NewAllWorld: %v", err)
	}
	_, err = a.Prepare(context.Background(), "s1", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	// Create another dir to walk
	err = os.MkdirAll(a.snapshotDir("s2"), 0700)
	if err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	m := &mockFS{
		fallback:   RealFS{},
		MkdirAllFn: os.MkdirAll,
		ReadFileFn: func(path string) ([]byte, error) {
			if strings.Contains(path, "meta.json") {
				return nil, errors.New("read-meta-fail")
			}
			return os.ReadFile(path)
		},
	}
	a.WithFS(m)
	has, err := a.hasDependents("s1")
	if err != nil {
		t.Fatalf("hasDependents should not fail on meta read error: %v", err)
	}
	if has {
		t.Error("should not have dependents if all meta reads fail")
	}
}

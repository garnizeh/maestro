package prim

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/garnizeh/maestro/pkg/archive"
)

func TestFuseOverlay_Prepare_Success(t *testing.T) {
	root := t.TempDir()
	f, err := NewFuseOverlay(root)
	if err != nil {
		t.Fatalf("NewFuseOverlay: %v", err)
	}
	ctx := context.Background()

	// Prepare root snapshot.
	mounts, err := f.Prepare(ctx, "s1", "")
	if err != nil {
		t.Fatalf("Prepare root: %v", err)
	}
	if len(mounts) != 1 || mounts[0].Type != "bind" {
		t.Errorf("expected 1 bind mount, got %v", mounts)
	}

	// Prepare child snapshot.
	if commitErr := f.Commit(ctx, "c1", "s1"); commitErr != nil {
		t.Fatalf("Commit: %v", commitErr)
	}
	mounts, err = f.Prepare(ctx, "s2", "c1")
	if err != nil {
		t.Fatalf("Prepare child: %v", err)
	}
	if len(mounts) != 1 || mounts[0].Type != "fuse-overlayfs" {
		t.Errorf("expected 1 fuse-overlayfs mount, got %v", mounts)
	}
}

func TestFuseOverlay_View_Success(t *testing.T) {
	root := t.TempDir()
	f, err := NewFuseOverlay(root)
	if err != nil {
		t.Fatalf("NewFuseOverlay: %v", err)
	}
	ctx := context.Background()

	mounts, err := f.View(ctx, "v1", "")
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	if len(mounts) == 0 {
		t.Fatal("expected mounts")
	}

	meta, err := f.readMeta("v1")
	if err != nil {
		t.Fatalf("readMeta: %v", err)
	}
	if meta.Kind != KindView {
		t.Errorf("expected KindView, got %v", meta.Kind)
	}
}

func TestFuseOverlay_View_CreatesFSDirWithTraversalPerms(t *testing.T) {
	root := t.TempDir()
	f, err := NewFuseOverlay(root)
	if err != nil {
		t.Fatalf("NewFuseOverlay: %v", err)
	}
	var fsMode os.FileMode
	m := &mockFS{
		fallback: RealFS{},
		MkdirAllFn: func(p string, mode os.FileMode) error {
			if path.Base(p) == "fs" {
				fsMode = mode
			}
			return os.MkdirAll(p, mode)
		},
	}
	f.WithFS(m)

	if _, err = f.View(context.Background(), "v1", ""); err != nil {
		t.Fatalf("View: %v", err)
	}
	if fsMode != fsDirPerm {
		t.Fatalf("fs dir mode = %04o; want %04o", fsMode, fsDirPerm)
	}
}

func TestFuseOverlay_Commit_Success(t *testing.T) {
	root := t.TempDir()
	f, err := NewFuseOverlay(root)
	if err != nil {
		t.Fatalf("NewFuseOverlay: %v", err)
	}
	ctx := context.Background()

	_, err = f.Prepare(ctx, "s1", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	err = f.Commit(ctx, "c1", "s1")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	meta, err := f.readMeta("c1")
	if err != nil {
		t.Fatalf("readMeta: %v", err)
	}
	if meta.Kind != KindCommitted {
		t.Errorf("expected KindCommitted, got %v", meta.Kind)
	}
}

func TestFuseOverlay_Remove_Success(t *testing.T) {
	root := t.TempDir()
	f, err := NewFuseOverlay(root)
	if err != nil {
		t.Fatalf("NewFuseOverlay: %v", err)
	}
	ctx := context.Background()

	_, err = f.Prepare(ctx, "s1", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if removeErr := f.Remove(ctx, "s1"); removeErr != nil {
		t.Fatalf("Remove: %v", removeErr)
	}
}

func TestFuseOverlay_Walk(t *testing.T) {
	root := t.TempDir()
	f, err := NewFuseOverlay(root)
	if err != nil {
		t.Fatalf("NewFuseOverlay: %v", err)
	}
	ctx := context.Background()

	_, err = f.Prepare(ctx, "s1", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	_, err = f.Prepare(ctx, "s2", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	count := 0
	err = f.Walk(ctx, func(Info) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if count != 2 {
		t.Errorf("walked %d keys, want 2", count)
	}
}

func TestFuseOverlay_Usage(t *testing.T) {
	root := t.TempDir()
	f, err := NewFuseOverlay(root)
	if err != nil {
		t.Fatalf("NewFuseOverlay: %v", err)
	}
	ctx := context.Background()

	_, err = f.Prepare(ctx, "s1", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	err = os.WriteFile(filepath.Join(f.WritableDir("s1"), "foo"), []byte("bar"), 0644)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	usage, err := f.Usage(ctx, "s1")
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	if usage.Size < 3 {
		t.Errorf("expected size >= 3, got %d", usage.Size)
	}
}

func TestFuseOverlay_FailurePaths(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()

	// 1. NewFuseOverlay failure
	tmpFile, err := os.CreateTemp(root, "file")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	err = tmpFile.Close()
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	_, err = NewFuseOverlay(tmpFile.Name())
	if err == nil {
		t.Error("expected error from NewFuseOverlay on invalid root")
	}

	// 2. Prepare already exists
	f, err := NewFuseOverlay(root)
	if err != nil {
		t.Fatalf("NewFuseOverlay: %v", err)
	}
	_, err = f.Prepare(ctx, "exist", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	_, err = f.Prepare(ctx, "exist", "")
	if err == nil {
		t.Error("expected already exists error")
	}

	// 3. Prepare mkdir fail (work)
	badDir := filepath.Join(root, "bad")
	err = os.Mkdir(badDir, 0755)
	if err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	pBad, err := NewFuseOverlay(badDir)
	if err != nil {
		t.Fatalf("NewFuseOverlay: %v", err)
	}
	err = os.MkdirAll(pBad.snapshotDir("fail"), 0755)
	if err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	err = os.WriteFile(filepath.Join(pBad.snapshotDir("fail"), "work"), []byte("notadir"), 0644)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err = pBad.Prepare(ctx, "fail", "")
	if err == nil {
		t.Error("expected mkdir error for work dir")
	}

	// 4. Prepare mkdir fail (fs)
	err = os.Remove(filepath.Join(pBad.snapshotDir("fail"), "work"))
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	err = os.WriteFile(filepath.Join(pBad.snapshotDir("fail"), "fs"), []byte("notadir"), 0644)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err = pBad.Prepare(ctx, "fail", "")
	if err == nil {
		t.Error("expected mkdir error for fs dir")
	}

	// 5. Commit fail (non-existent)
	err = f.Commit(ctx, "c2", "none")
	if err == nil {
		t.Error("expected error committing non-existent")
	}

	// 6. View mkdir fail
	err = os.MkdirAll(pBad.snapshotDir("fail-view"), 0755)
	if err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	err = os.WriteFile(filepath.Join(pBad.snapshotDir("fail-view"), "fs"), []byte("notadir"), 0644)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err = pBad.View(ctx, "fail-view", "")
	if err == nil {
		t.Error("expected error viewing when fs/ is a file")
	}
}

func TestFuseOverlay_Mounts_Errors(t *testing.T) {
	f, err := NewFuseOverlay(t.TempDir())
	if err != nil {
		t.Fatalf("NewFuseOverlay: %v", err)
	}

	// 1. Missing parent meta
	_, err = f.mounts("s1", "nonexistent")
	if err == nil {
		t.Error("expected error for mounts with missing parent meta")
	}

	// 2. Metadata corruption
	_, err = f.Prepare(context.Background(), "corrupt", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	err = os.WriteFile(filepath.Join(f.snapshotDir("corrupt"), "meta.json"), []byte("{"), 0644)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err = f.mounts("child", "corrupt")
	if err == nil {
		t.Error("expected error with corrupt meta")
	}
}

func TestFuseOverlay_Helpers(t *testing.T) {
	f, err := NewFuseOverlay(t.TempDir())
	if err != nil {
		t.Fatalf("NewFuseOverlay: %v", err)
	}
	f.WithMounter(&RealMounter{})
	f.WithFS(&RealFS{})

	if f.WhiteoutFormat() != archive.WhiteoutOverlay {
		t.Errorf("got %v, want overlay", f.WhiteoutFormat())
	}
}

func TestFuseOverlay_Prepare_WriteMetaError(t *testing.T) {
	root := t.TempDir()
	f, err := NewFuseOverlay(root)
	if err != nil {
		t.Fatalf("NewFuseOverlay: %v", err)
	}
	m := &mockFS{
		MkdirAllFn: os.MkdirAll,
		WriteFileFn: func(string, []byte, os.FileMode) error {
			return errors.New("write-fail")
		},
	}
	f.WithFS(m)
	_, err = f.Prepare(context.Background(), "s1", "")
	if err == nil {
		t.Error("expected write meta error")
	}
}

func TestFuseOverlay_View_WriteMetaError(t *testing.T) {
	root := t.TempDir()
	f, err := NewFuseOverlay(root)
	if err != nil {
		t.Fatalf("NewFuseOverlay: %v", err)
	}
	m := &mockFS{
		MkdirAllFn: os.MkdirAll,
		WriteFileFn: func(string, []byte, os.FileMode) error {
			return errors.New("write-fail")
		},
	}
	f.WithFS(m)
	_, err = f.View(context.Background(), "v1", "")
	if err == nil {
		t.Error("expected write meta error")
	}
}

func TestFuseOverlay_View_MkdirError(t *testing.T) {
	root := t.TempDir()
	f, err := NewFuseOverlay(root)
	if err != nil {
		t.Fatalf("NewFuseOverlay: %v", err)
	}
	m := &mockFS{
		MkdirAllFn: func(path string, mode os.FileMode) error {
			if filepath.Base(path) == "fs" {
				return errors.New("mkdir-fs-fail")
			}
			return os.MkdirAll(path, mode)
		},
	}
	f.WithFS(m)
	_, err = f.View(context.Background(), "v1", "")
	if err == nil || !strings.Contains(err.Error(), "mkdir-fs-fail") {
		t.Errorf("expected mkdir-fs-fail error, got %v", err)
	}
}

func TestFuseOverlay_AllWorld_InitializationFailures(t *testing.T) {
	root := t.TempDir()
	f, err := NewFuseOverlay(root)
	if err != nil {
		t.Fatalf("NewFuseOverlay: %v", err)
	}
	ctx := context.Background()

	// Cause NewAllWorld to fail by setting an invalid root.
	// NewAllWorld uses RealFS initially to create the snapshots dir.
	tmpFile, err := os.CreateTemp(root, "file")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if closeErr := tmpFile.Close(); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}
	originalRoot := f.root
	f.root = tmpFile.Name()
	defer func() { f.root = originalRoot }()

	if commitErr := f.Commit(ctx, "c1", "s1"); commitErr == nil {
		t.Error("expected error from Commit with invalid root")
	}
	if removeErr := f.Remove(ctx, "s1"); removeErr == nil {
		t.Error("expected error from Remove with invalid root")
	}
	if walkErr := f.Walk(ctx, func(Info) error { return nil }); walkErr == nil {
		t.Error("expected error from Walk with invalid root")
	}
	if _, usageErr := f.Usage(ctx, "s1"); usageErr == nil {
		t.Error("expected error from Usage with invalid root")
	}
}

func TestFuseOverlay_Mounts_ReadMetaError(t *testing.T) {
	root := t.TempDir()
	f, err := NewFuseOverlay(root)
	if err != nil {
		t.Fatalf("NewFuseOverlay: %v", err)
	}

	// Create a parent snapshot first.
	_, err = f.Prepare(context.Background(), "p1", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	err = f.Commit(context.Background(), "c1", "p1")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Now set a failing FS for metadata reads.
	m := &mockFS{
		fallback: RealFS{},
		readMetaFn: func(string) ([]byte, error) {
			return nil, errors.New("read-meta-fail")
		},
	}
	f.WithFS(m)

	_, err = f.Prepare(context.Background(), "s1", "c1")
	if err == nil || !strings.Contains(err.Error(), "read meta for c1") {
		t.Errorf("expected read meta error, got %v", err)
	}
}

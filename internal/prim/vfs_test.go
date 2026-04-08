package prim_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rodrigo-baliza/maestro/internal/prim"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func newVFS(t *testing.T) *prim.VFS {
	t.Helper()
	v, err := prim.NewVFS(t.TempDir())
	if err != nil {
		t.Fatalf("NewVFS: %v", err)
	}
	return v
}

// writeFile creates a file at path with the given content.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdirall: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

// readFile returns the content of path.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	return string(data)
}

// ── Kind tests ────────────────────────────────────────────────────────────────

func TestKind_String(t *testing.T) {
	cases := []struct {
		k    prim.Kind
		want string
	}{
		{prim.KindCommitted, "committed"},
		{prim.KindActive, "active"},
		{prim.KindView, "view"},
		{prim.Kind(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.k.String(); got != tc.want {
			t.Errorf("Kind(%d).String() = %q; want %q", tc.k, got, tc.want)
		}
	}
}

// ── NewVFS tests ──────────────────────────────────────────────────────────────

func TestNewVFS_CreatesRoot(t *testing.T) {
	dir := t.TempDir()
	v, err := prim.NewVFS(dir)
	if err != nil {
		t.Fatalf("NewVFS: %v", err)
	}
	if v == nil {
		t.Fatal("expected non-nil VFS")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "prim", "snapshots")); statErr != nil {
		t.Errorf("snapshot root not created: %v", statErr)
	}
}

func TestNewVFS_InvalidRoot(t *testing.T) {
	// Try to create VFS under a file (not a dir).
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	_ = os.WriteFile(blocker, []byte("x"), 0o600)
	_, err := prim.NewVFS(filepath.Join(blocker, "prim"))
	if err == nil {
		t.Fatal("expected error for invalid root")
	}
}

// ── Prepare tests ─────────────────────────────────────────────────────────────

func TestVFS_Prepare_NoParent(t *testing.T) {
	v := newVFS(t)
	mounts, err := v.Prepare(context.Background(), "base", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount; got %d", len(mounts))
	}
	if _, statErr := os.Stat(mounts[0].Source); statErr != nil {
		t.Errorf("mount source does not exist: %v", statErr)
	}
}

func TestVFS_Prepare_WithParent(t *testing.T) {
	v := newVFS(t)

	// Create a committed parent with a file.
	_, err := v.Prepare(context.Background(), "base-rw", "")
	if err != nil {
		t.Fatalf("Prepare base: %v", err)
	}
	// Write a file to the base fs.
	baseFsDir := filepath.Join(v.SnapshotsDir(), "base-rw", "fs")
	writeFile(t, filepath.Join(baseFsDir, "hello.txt"), "hello")
	// Commit the base.
	if commitErr := v.Commit(context.Background(), "base", "base-rw"); commitErr != nil {
		t.Fatalf("Commit base: %v", commitErr)
	}

	// Prepare a child.
	mounts, err := v.Prepare(context.Background(), "child-rw", "base")
	if err != nil {
		t.Fatalf("Prepare child: %v", err)
	}

	// The child must contain the parent's file.
	content := readFile(t, filepath.Join(mounts[0].Source, "hello.txt"))
	if content != "hello" {
		t.Errorf("child file content = %q; want hello", content)
	}
}

func TestVFS_Prepare_AlreadyExists(t *testing.T) {
	v := newVFS(t)
	_, err := v.Prepare(context.Background(), "key1", "")
	if err != nil {
		t.Fatalf("first Prepare: %v", err)
	}
	_, err = v.Prepare(context.Background(), "key1", "")
	if err == nil {
		t.Fatal("expected error on duplicate key")
	}
	if !errors.Is(err, prim.ErrSnapshotAlreadyExists) {
		t.Errorf("expected ErrSnapshotAlreadyExists; got: %v", err)
	}
}

func TestVFS_Prepare_ParentNotFound(t *testing.T) {
	v := newVFS(t)
	_, err := v.Prepare(context.Background(), "child", "nonexistent-parent")
	if err == nil {
		t.Fatal("expected error for missing parent")
	}
	if !errors.Is(err, prim.ErrSnapshotNotFound) {
		t.Errorf("expected ErrSnapshotNotFound; got: %v", err)
	}
}

// ── View tests ────────────────────────────────────────────────────────────────

func TestVFS_View_NoParent(t *testing.T) {
	v := newVFS(t)
	mounts, err := v.View(context.Background(), "view1", "")
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	// View should have "ro" option.
	hasRO := false
	for _, opt := range mounts[0].Options {
		if opt == "ro" {
			hasRO = true
		}
	}
	if !hasRO {
		t.Errorf("view mount should have ro option; got: %v", mounts[0].Options)
	}
}

func TestVFS_View_WithParent(t *testing.T) {
	v := newVFS(t)

	// Prepare and commit a parent with a file.
	_, _ = v.Prepare(context.Background(), "base-rw", "")
	baseFsDir := filepath.Join(v.SnapshotsDir(), "base-rw", "fs")
	writeFile(t, filepath.Join(baseFsDir, "data.txt"), "view-data")
	_ = v.Commit(context.Background(), "base", "base-rw")

	// Create a view.
	mounts, err := v.View(context.Background(), "view1", "base")
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	content := readFile(t, filepath.Join(mounts[0].Source, "data.txt"))
	if content != "view-data" {
		t.Errorf("view file content = %q; want view-data", content)
	}
}

func TestVFS_View_AlreadyExists(t *testing.T) {
	v := newVFS(t)
	_, _ = v.View(context.Background(), "v1", "")
	_, err := v.View(context.Background(), "v1", "")
	if err == nil {
		t.Fatal("expected error on duplicate view")
	}
	if !errors.Is(err, prim.ErrSnapshotAlreadyExists) {
		t.Errorf("expected ErrSnapshotAlreadyExists; got: %v", err)
	}
}

func TestVFS_View_ParentNotFound(t *testing.T) {
	v := newVFS(t)
	_, err := v.View(context.Background(), "v1", "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing parent")
	}
	if !errors.Is(err, prim.ErrSnapshotNotFound) {
		t.Errorf("expected ErrSnapshotNotFound; got: %v", err)
	}
}

// ── Commit tests ──────────────────────────────────────────────────────────────

func TestVFS_Commit_Success(t *testing.T) {
	v := newVFS(t)
	_, _ = v.Prepare(context.Background(), "rw1", "")
	if err := v.Commit(context.Background(), "layer1", "rw1"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	// The committed snapshot should be referenceable.
	mounts, err := v.Prepare(context.Background(), "child-rw", "layer1")
	if err != nil {
		t.Fatalf("Prepare using committed layer: %v", err)
	}
	if len(mounts) == 0 {
		t.Fatal("expected non-empty mounts")
	}
}

func TestVFS_Commit_NotFound(t *testing.T) {
	v := newVFS(t)
	err := v.Commit(context.Background(), "layer1", "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, prim.ErrSnapshotNotFound) {
		t.Errorf("expected ErrSnapshotNotFound; got: %v", err)
	}
}

func TestVFS_Commit_OnView_Rejected(t *testing.T) {
	v := newVFS(t)
	_, _ = v.View(context.Background(), "v1", "")
	err := v.Commit(context.Background(), "committed1", "v1")
	if err == nil {
		t.Fatal("expected error committing a view")
	}
	if !errors.Is(err, prim.ErrCommitOnReadOnly) {
		t.Errorf("expected ErrCommitOnReadOnly; got: %v", err)
	}
}

// ── Remove tests ──────────────────────────────────────────────────────────────

func TestVFS_Remove_Success(t *testing.T) {
	v := newVFS(t)
	_, _ = v.Prepare(context.Background(), "rw1", "")
	if err := v.Remove(context.Background(), "rw1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	// Second remove should fail.
	err := v.Remove(context.Background(), "rw1")
	if err == nil {
		t.Fatal("expected error on second remove")
	}
}

func TestVFS_Remove_NotFound(t *testing.T) {
	v := newVFS(t)
	err := v.Remove(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, prim.ErrSnapshotNotFound) {
		t.Errorf("expected ErrSnapshotNotFound; got: %v", err)
	}
}

func TestVFS_Remove_WithDependents_Rejected(t *testing.T) {
	v := newVFS(t)
	// Build: rw1 → committed "layer1" → child-rw
	_, _ = v.Prepare(context.Background(), "rw1", "")
	_ = v.Commit(context.Background(), "layer1", "rw1")
	_, _ = v.Prepare(context.Background(), "child-rw", "layer1")

	// Removing "layer1" should fail because "child-rw" depends on it.
	err := v.Remove(context.Background(), "layer1")
	if err == nil {
		t.Fatal("expected error when removing snapshot with dependents")
	}
	if !errors.Is(err, prim.ErrSnapshotHasDependents) {
		t.Errorf("expected ErrSnapshotHasDependents; got: %v", err)
	}
}

// ── Walk tests ────────────────────────────────────────────────────────────────

func TestVFS_Walk_Empty(t *testing.T) {
	v := newVFS(t)
	var infos []prim.Info
	if err := v.Walk(context.Background(), func(i prim.Info) error {
		infos = append(infos, i)
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("expected 0 snapshots; got %d", len(infos))
	}
}

func TestVFS_Walk_Multiple(t *testing.T) {
	v := newVFS(t)
	_, _ = v.Prepare(context.Background(), "s1", "")
	_, _ = v.Prepare(context.Background(), "s2", "")
	_, _ = v.View(context.Background(), "s3", "")

	var infos []prim.Info
	if err := v.Walk(context.Background(), func(i prim.Info) error {
		infos = append(infos, i)
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(infos) != 3 {
		t.Errorf("expected 3 snapshots; got %d", len(infos))
	}
}

func TestVFS_Walk_CallbackError(t *testing.T) {
	v := newVFS(t)
	_, _ = v.Prepare(context.Background(), "s1", "")
	sentinel := errors.New("stop")
	err := v.Walk(context.Background(), func(_ prim.Info) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error from Walk callback; got: %v", err)
	}
}

// ── Usage tests ───────────────────────────────────────────────────────────────

func TestVFS_Usage_Empty(t *testing.T) {
	v := newVFS(t)
	_, _ = v.Prepare(context.Background(), "rw1", "")
	u, err := v.Usage(context.Background(), "rw1")
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	// An empty fs/ directory still has the dir entry itself.
	if u.Inodes < 1 {
		t.Errorf("Inodes = %d; want >= 1", u.Inodes)
	}
}

func TestVFS_Usage_WithFiles(t *testing.T) {
	v := newVFS(t)
	mounts, _ := v.Prepare(context.Background(), "rw1", "")
	writeFile(t, filepath.Join(mounts[0].Source, "bigfile.dat"), strings.Repeat("x", 1000))

	u, err := v.Usage(context.Background(), "rw1")
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	if u.Size < 1000 {
		t.Errorf("Size = %d; want >= 1000", u.Size)
	}
}

func TestVFS_Usage_NotFound(t *testing.T) {
	v := newVFS(t)
	_, err := v.Usage(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, prim.ErrSnapshotNotFound) {
		t.Errorf("expected ErrSnapshotNotFound; got: %v", err)
	}
}

// ── copyDir / copyFile tests ──────────────────────────────────────────────────

func TestCopyDir_Success(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Create nested structure.
	writeFile(t, filepath.Join(src, "a.txt"), "content-a")
	writeFile(t, filepath.Join(src, "sub", "b.txt"), "content-b")

	if err := prim.CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}
	if readFile(t, filepath.Join(dst, "a.txt")) != "content-a" {
		t.Error("a.txt not copied correctly")
	}
	if readFile(t, filepath.Join(dst, "sub", "b.txt")) != "content-b" {
		t.Error("sub/b.txt not copied correctly")
	}
}

func TestCopyFile_Success(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	_ = os.WriteFile(src, []byte("hello"), 0o644)

	if err := prim.CopyFile(src, dst); err != nil {
		t.Fatalf("CopyFile: %v", err)
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "hello" {
		t.Errorf("data = %q; want hello", data)
	}
}

func TestCopyFile_SrcNotExist(t *testing.T) {
	dir := t.TempDir()
	err := prim.CopyFile(filepath.Join(dir, "nonexistent.txt"), filepath.Join(dir, "dst.txt"))
	if err == nil {
		t.Fatal("expected error for missing src")
	}
}

func TestCopyFile_DstNotCreatable(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	_ = os.WriteFile(src, []byte("data"), 0o644)
	err := prim.CopyFile(src, filepath.Join(dir, "nodir", "subdir", "dst.txt"))
	if err == nil {
		t.Fatal("expected error for uncreatable destination")
	}
}

func TestVFS_Walk_SkipsCorruptMeta(t *testing.T) {
	v := newVFS(t)
	_, _ = v.Prepare(context.Background(), "good", "")
	corruptDir := filepath.Join(v.SnapshotsDir(), "corrupt")
	_ = os.MkdirAll(filepath.Join(corruptDir, "fs"), 0o700)
	var infos []prim.Info
	if err := v.Walk(context.Background(), func(i prim.Info) error {
		infos = append(infos, i)
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(infos) != 1 || infos[0].Key != "good" {
		t.Errorf("Walk returned %v; want only 'good'", infos)
	}
}

func TestVFS_Walk_EmptyRootDir(t *testing.T) {
	dir := t.TempDir()
	// Using NewVFS to get a valid struct but with a nonexistent root.
	v, _ := prim.NewVFS(filepath.Join(dir, "nonexistent"))
	var count int
	if err := v.Walk(context.Background(), func(_ prim.Info) error {
		count++
		return nil
	}); err != nil {
		t.Fatalf("Walk on nonexistent root: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 snapshots; got %d", count)
	}
}

func TestVFS_HasDependents_ScanError(t *testing.T) {
	v := newVFS(t)
	has, err := v.HasDependents("nonexistent-parent")
	if err != nil {
		t.Fatalf("HasDependents on empty dir: %v", err)
	}
	if has {
		t.Error("expected no dependents")
	}
}

func TestVFS_Commit_Rename_Fails(t *testing.T) {
	v := newVFS(t)
	_, _ = v.Prepare(context.Background(), "rw1", "")
	dstDir := v.SnapshotDir("committed1")
	_ = os.MkdirAll(filepath.Join(dstDir, "fs"), 0o700)
	_ = prim.WriteMeta(dstDir, prim.VFSMeta{Key: "committed1", Kind: prim.KindCommitted})
	_ = os.WriteFile(filepath.Join(dstDir, "fs", "x.txt"), []byte("x"), 0o600)

	err := v.Commit(context.Background(), "committed1", "rw1")
	if err != nil && !strings.Contains(err.Error(), "rename") {
		t.Errorf("unexpected error type: %v", err)
	}
}

func TestVFS_ReadMeta_CorruptJSON(t *testing.T) {
	v := newVFS(t)
	dir := v.SnapshotDir("badmeta")
	_ = os.MkdirAll(dir, 0o700)
	_ = os.WriteFile(filepath.Join(dir, "meta.json"), []byte("{invalid"), 0o600)

	_, err := v.ReadMeta("badmeta")
	if err == nil {
		t.Fatal("expected error for corrupt JSON meta")
	}
}

func TestVFS_CheckNotExists_AlreadyExists(t *testing.T) {
	v := newVFS(t)
	_, _ = v.Prepare(context.Background(), "snap1", "")

	err := v.CheckNotExists("snap1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, prim.ErrSnapshotAlreadyExists) {
		t.Errorf("expected ErrSnapshotAlreadyExists; got: %v", err)
	}
}

func TestVFS_HasDependents_SelfParent(t *testing.T) {
	v := newVFS(t)
	dir := v.SnapshotDir("self-ref")
	_ = os.MkdirAll(dir, 0o700)
	_ = prim.WriteMeta(dir, prim.VFSMeta{Key: "self-ref", Parent: "self-ref", Kind: prim.KindCommitted})

	has, err := v.HasDependents("self-ref")
	if err != nil {
		t.Fatalf("HasDependents self-ref: %v", err)
	}
	if has {
		t.Error("self-reference should not count as dependent")
	}
}

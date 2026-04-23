package prim

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kr/pretty"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func newVFS(t *testing.T) *VFS {
	t.Helper()
	v, err := NewVFS(t.TempDir())
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
		k    Kind
		want string
	}{
		{KindCommitted, "committed"},
		{KindActive, "active"},
		{KindView, "view"},
		{Kind(99), "unknown"},
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
	v, err := NewVFS(dir)
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
	err := os.WriteFile(blocker, []byte("x"), 0o600)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err = NewVFS(filepath.Join(blocker, "prim"))
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

func TestVFS_Prepare_CreatesFSDirWithTraversalPerms(t *testing.T) {
	v := newVFS(t)
	var fsMode os.FileMode
	m := &mockFS{
		fallback: RealFS{},
		MkdirAllFn: func(p string, mode os.FileMode) error {
			if filepath.Base(p) == "fs" {
				fsMode = mode
			}
			return os.MkdirAll(p, mode)
		},
	}
	v.WithFS(m)

	if _, err := v.Prepare(context.Background(), "base", ""); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if fsMode != fsDirPerm {
		t.Fatalf("fs dir mode = %04o; want %04o", fsMode, fsDirPerm)
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
	if !errors.Is(err, ErrSnapshotAlreadyExists) {
		t.Errorf("expected ErrSnapshotAlreadyExists; got: %v", err)
	}
}

func TestVFS_Prepare_ParentNotFound(t *testing.T) {
	v := newVFS(t)
	_, err := v.Prepare(context.Background(), "child", "nonexistent-parent")
	if err == nil {
		t.Fatal("expected error for missing parent")
	}
	if !errors.Is(err, ErrSnapshotNotFound) {
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
	_, err := v.Prepare(context.Background(), "base-rw", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	baseFsDir := filepath.Join(v.SnapshotsDir(), "base-rw", "fs")
	writeFile(t, filepath.Join(baseFsDir, "data.txt"), "view-data")
	err = v.Commit(context.Background(), "base", "base-rw")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

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
	_, err := v.View(context.Background(), "v1", "")
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	_, err = v.View(context.Background(), "v1", "")
	if err == nil {
		t.Fatal("expected error on duplicate view")
	}
	if !errors.Is(err, ErrSnapshotAlreadyExists) {
		t.Errorf("expected ErrSnapshotAlreadyExists; got: %v", err)
	}
}

func TestVFS_View_ParentNotFound(t *testing.T) {
	v := newVFS(t)
	_, err := v.View(context.Background(), "v1", "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing parent")
	}
	if !errors.Is(err, ErrSnapshotNotFound) {
		t.Errorf("expected ErrSnapshotNotFound; got: %v", err)
	}
}

// ── Commit tests ──────────────────────────────────────────────────────────────

func TestVFS_Commit_Success(t *testing.T) {
	v := newVFS(t)
	_, err := v.Prepare(context.Background(), "rw1", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	err = v.Commit(context.Background(), "layer1", "rw1")
	if err != nil {
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
	if !errors.Is(err, ErrSnapshotNotFound) {
		t.Errorf("expected ErrSnapshotNotFound; got: %v", err)
	}
}

func TestVFS_Commit_OnView_Rejected(t *testing.T) {
	v := newVFS(t)
	_, err := v.View(context.Background(), "v1", "")
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	err = v.Commit(context.Background(), "committed1", "v1")
	if err == nil {
		t.Fatal("expected error committing a view")
	}
	if !errors.Is(err, ErrCommitOnReadOnly) {
		t.Errorf("expected ErrCommitOnReadOnly; got: %v", err)
	}
}

// ── Remove tests ──────────────────────────────────────────────────────────────

func TestVFS_Remove_Success(t *testing.T) {
	v := newVFS(t)
	_, err := v.Prepare(context.Background(), "rw1", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	err = v.Remove(context.Background(), "rw1")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	// Second remove should fail.
	err = v.Remove(context.Background(), "rw1")
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
	if !errors.Is(err, ErrSnapshotNotFound) {
		t.Errorf("expected ErrSnapshotNotFound; got: %v", err)
	}
}

func TestVFS_Remove_WithDependents_Rejected(t *testing.T) {
	v := newVFS(t)
	// Build: rw1 → committed "layer1" → child-rw
	_, err := v.Prepare(context.Background(), "rw1", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	err = v.Commit(context.Background(), "layer1", "rw1")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	_, err = v.Prepare(context.Background(), "child-rw", "layer1")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	// Removing "layer1" should fail because "child-rw" depends on it.
	err = v.Remove(context.Background(), "layer1")
	if err == nil {
		t.Fatal("expected error when removing snapshot with dependents")
	}
	if !errors.Is(err, ErrSnapshotHasDependents) {
		t.Errorf("expected ErrSnapshotHasDependents; got: %v", err)
	}
}

// ── Walk tests ────────────────────────────────────────────────────────────────

func TestVFS_Walk_Empty(t *testing.T) {
	v := newVFS(t)
	var infos []Info
	if err := v.Walk(context.Background(), func(i Info) error {
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
	_, err := v.Prepare(context.Background(), "s1", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	_, err = v.Prepare(context.Background(), "s2", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	_, err = v.View(context.Background(), "s3", "")
	if err != nil {
		t.Fatalf("View: %v", err)
	}

	var infos []Info
	if walkErr := v.Walk(context.Background(), func(i Info) error {
		infos = append(infos, i)
		return nil
	}); walkErr != nil {
		t.Fatalf("Walk: %v", walkErr)
	}
	if len(infos) != 3 {
		t.Errorf("expected 3 snapshots; got %d", len(infos))
	}
}

func TestVFS_Walk_CallbackError(t *testing.T) {
	v := newVFS(t)
	_, err := v.Prepare(context.Background(), "s1", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	sentinel := errors.New("stop")
	walkErr := v.Walk(context.Background(), func(_ Info) error {
		return sentinel
	})
	if !errors.Is(walkErr, sentinel) {
		t.Errorf("expected sentinel error from Walk callback; got: %v", walkErr)
	}
}

// ── Usage tests ───────────────────────────────────────────────────────────────

func TestVFS_Usage_Empty(t *testing.T) {
	v := newVFS(t)
	_, err := v.Prepare(context.Background(), "rw1", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
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
	mounts, err := v.Prepare(context.Background(), "rw1", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
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
	if !errors.Is(err, ErrSnapshotNotFound) {
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

	v := newVFS(t)
	if err := v.CopyDir(src, dst); err != nil {
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
	err := os.WriteFile(src, []byte("hello"), 0o644)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	v := newVFS(t)
	if copyErr := v.CopyFile(src, dst); copyErr != nil {
		t.Fatalf("CopyFile: %v", copyErr)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("data = %q; want hello", data)
	}
}

func TestCopyFile_SrcNotExist(t *testing.T) {
	dir := t.TempDir()
	v := newVFS(t)
	err := v.CopyFile(filepath.Join(dir, "nonexistent.txt"), filepath.Join(dir, "dst.txt"))
	if err == nil {
		t.Fatal("expected error for missing src")
	}
}

func TestCopyFile_DstNotCreatable(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	v := newVFS(t)
	err := os.WriteFile(src, []byte("data"), 0o644)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	err = v.CopyFile(src, filepath.Join(dir, "nodir", "subdir", "dst.txt"))
	if err == nil {
		t.Fatal("expected error for uncreatable destination")
	}
}

func TestVFS_Walk_SkipsCorruptMeta(t *testing.T) {
	v := newVFS(t)
	_, err := v.Prepare(context.Background(), "good", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	corruptDir := filepath.Join(v.SnapshotsDir(), "corrupt")
	err = os.MkdirAll(filepath.Join(corruptDir, "fs"), 0o700)
	if err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	var infos []Info
	if walkErr := v.Walk(context.Background(), func(i Info) error {
		infos = append(infos, i)
		return nil
	}); walkErr != nil {
		t.Fatalf("Walk: %v", walkErr)
	}
	want := []string{"good"}
	got := make([]string, len(infos))
	for i, info := range infos {
		got[i] = info.Key
	}
	if diff := pretty.Diff(want, got); len(diff) > 0 {
		t.Log("VFS.Walk() results mismatch")
		t.Logf("want: %v", want)
		t.Logf("got: %v", got)
		t.Errorf("\n%s", diff)
	}
}

func TestVFS_Walk_EmptyRootDir(t *testing.T) {
	dir := t.TempDir()
	// Using NewVFS to get a valid struct but with a nonexistent root.
	v, err := NewVFS(filepath.Join(dir, "nonexistent"))
	if err != nil {
		t.Fatalf("NewVFS: %v", err)
	}
	var count int
	if walkErr := v.Walk(context.Background(), func(_ Info) error {
		count++
		return nil
	}); walkErr != nil {
		t.Fatalf("Walk on nonexistent root: %v", walkErr)
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

func TestVFS_Commit_DestExists(t *testing.T) {
	v := newVFS(t)
	_, err := v.Prepare(context.Background(), "rw1", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	dstDir := v.SnapshotDir("committed1")
	err = os.MkdirAll(filepath.Join(dstDir, "fs"), 0o700)
	if err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	err = v.WriteMeta(dstDir, VFSMeta{Key: "committed1", Kind: KindCommitted})
	if err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}
	err = os.WriteFile(filepath.Join(dstDir, "fs", "x.txt"), []byte("x"), 0o600)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	err = v.Commit(context.Background(), "committed1", "rw1")
	if err != nil && !errors.Is(err, ErrSnapshotAlreadyExists) {
		t.Errorf("unexpected error type: %v", err)
	}
}

func TestVFS_ReadMeta_CorruptJSON(t *testing.T) {
	v := newVFS(t)
	dir := v.SnapshotDir("badmeta")
	err := os.MkdirAll(dir, 0o700)
	if err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	err = os.WriteFile(filepath.Join(dir, "meta.json"), []byte("{invalid"), 0o600)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err = v.ReadMeta("badmeta")
	if err == nil {
		t.Fatal("expected error for corrupt JSON meta")
	}
}

func TestVFS_CheckNotExists_AlreadyExists(t *testing.T) {
	v := newVFS(t)
	_, err := v.Prepare(context.Background(), "snap1", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	err = v.CheckNotExists("snap1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrSnapshotAlreadyExists) {
		t.Errorf("expected ErrSnapshotAlreadyExists; got: %v", err)
	}
}

func TestVFS_HasDependents_SelfParent(t *testing.T) {
	v := newVFS(t)
	dir := v.SnapshotDir("self-ref")
	err := os.MkdirAll(dir, 0o700)
	if err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	err = v.WriteMeta(dir, VFSMeta{Key: "self-ref", Parent: "self-ref", Kind: KindCommitted})
	if err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}

	has, err := v.HasDependents("self-ref")
	if err != nil {
		t.Fatalf("HasDependents self-ref: %v", err)
	}
	if has {
		t.Error("self-reference should not count as dependent")
	}
}

func TestVFS_Prepare_DirBlockedByFile(t *testing.T) {
	t.Parallel()
	v := newVFS(t)
	err := os.WriteFile(v.SnapshotDir("fail"), []byte("blocks"), 0644)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, prepErr := v.Prepare(context.Background(), "fail", ""); prepErr == nil {
		t.Error("expected error creating snapshot dir where a file exists")
	}
}

func TestVFS_Commit_WriteMetaBlocked(t *testing.T) {
	t.Parallel()
	vMeta, err := NewVFS(t.TempDir())
	if err != nil {
		t.Fatalf("NewVFS: %v", err)
	}
	ctx := context.Background()
	if _, prepErr := vMeta.Prepare(ctx, "s1", ""); prepErr != nil {
		t.Fatalf("Prepare: %v", prepErr)
	}
	metaPath := filepath.Join(vMeta.SnapshotDir("s1"), "meta.json")
	if removeErr := os.Remove(metaPath); removeErr != nil {
		t.Fatalf("Remove: %v", removeErr)
	}
	if mkdirErr := os.Mkdir(metaPath, 0755); mkdirErr != nil { // Block WriteFile with a directory
		t.Fatalf("Mkdir: %v", mkdirErr)
	}
	if commitErr := vMeta.Commit(ctx, "c1", "s1"); commitErr == nil {
		t.Error("expected error committing when meta.json is blocked")
	}
}

func TestVFS_Remove_SnapshotsDirBlocked(t *testing.T) {
	t.Parallel()
	vRm, err := NewVFS(t.TempDir())
	if err != nil {
		t.Fatalf("NewVFS: %v", err)
	}
	if removeErr := os.RemoveAll(vRm.SnapshotsDir()); removeErr != nil {
		t.Fatalf("RemoveAll: %v", removeErr)
	}
	if writeErr := os.WriteFile(vRm.SnapshotsDir(), []byte("notadir"), 0644); writeErr != nil {
		t.Fatalf("WriteFile: %v", writeErr)
	}
	if removeErr := vRm.Remove(context.Background(), "any"); removeErr == nil {
		t.Error("expected error removing when snapshots dir is a file")
	}
}

func TestVFS_CopyDir_BlockedPath(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "f1"), nil, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(dst, "f1"), nil, 0644); err != nil { // Block dir creation for f1
		t.Fatalf("WriteFile: %v", err)
	}
	vCopy := newVFS(t)
	if err := vCopy.CopyDir(src, filepath.Join(dst, "f1")); err == nil {
		t.Error("expected error copies to blocked path")
	}
}

func TestVFS_HasDependents_SnapshotsDirBlocked(t *testing.T) {
	t.Parallel()
	vDep, err := NewVFS(t.TempDir())
	if err != nil {
		t.Fatalf("NewVFS: %v", err)
	}
	if removeErr := os.RemoveAll(vDep.SnapshotsDir()); removeErr != nil {
		t.Fatalf("RemoveAll: %v", removeErr)
	}
	if writeErr := os.WriteFile(vDep.SnapshotsDir(), []byte("blocked"), 0644); writeErr != nil {
		t.Fatalf("WriteFile: %v", writeErr)
	}
	if _, dependentsErr := vDep.HasDependents("any"); dependentsErr == nil {
		t.Error("expected error from HasDependents when snapshots dir is a file")
	}
}

func TestVFS_ReadMeta_Missing(t *testing.T) {
	t.Parallel()
	vMeta2, err := NewVFS(t.TempDir())
	if err != nil {
		t.Fatalf("NewVFS: %v", err)
	}
	dirMeta := vMeta2.SnapshotDir("bad")
	if mkdirErr := os.MkdirAll(dirMeta, 0755); mkdirErr != nil {
		t.Fatalf("MkdirAll: %v", mkdirErr)
	}
	if _, readErr := vMeta2.ReadMeta("bad"); readErr == nil {
		t.Error("expected error from ReadMeta with missing file")
	}
}

func TestVFS_Usage_UnreadableDir(t *testing.T) {
	t.Parallel()
	vUsage, err := NewVFS(t.TempDir())
	if err != nil {
		t.Fatalf("NewVFS: %v", err)
	}
	ctx := context.Background()
	if _, prepErr := vUsage.Prepare(ctx, "s1", ""); prepErr != nil {
		t.Fatalf("Prepare: %v", prepErr)
	}
	fsDir := filepath.Join(vUsage.SnapshotDir("s1"), "fs")
	if chmodErr := os.Chmod(fsDir, 0000); chmodErr != nil {
		t.Fatalf("Chmod: %v", chmodErr)
	}
	defer func() { _ = os.Chmod(fsDir, 0700) }()
	if _, usageErr := vUsage.Usage(ctx, "s1"); usageErr == nil {
		t.Error("expected error from Usage on unreadable dir")
	}
}

func TestCopyDir_Symlink(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Create a file and a symlink to it.
	writeFile(t, filepath.Join(src, "real.txt"), "real-content")
	linkPath := filepath.Join(src, "link.txt")
	if err := os.Symlink("real.txt", linkPath); err != nil {
		t.Fatalf("os.Symlink: %v", err)
	}

	v := newVFS(t)
	if err := v.CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir: %v", err)
	}

	// Verify the link exists in dst and points to the right place.
	gotLink, err := os.Readlink(filepath.Join(dst, "link.txt"))
	if err != nil {
		t.Fatalf("Readlink in dst: %v", err)
	}
	if gotLink != "real.txt" {
		t.Errorf("got link target %q; want %q", gotLink, "real.txt")
	}

	// Verify the file content via the link (though not strictly necessary for symlink test).
	content := readFile(t, filepath.Join(dst, "link.txt"))
	if content != "real-content" {
		t.Errorf("content via link = %q; want %q", content, "real-content")
	}
}

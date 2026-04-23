package prim

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rodrigo-baliza/maestro/pkg/archive"
)

// ── tests ──────────────────────────────────────────────────────────────────────

func TestAllWorld_Prepare_Success(t *testing.T) {
	root := t.TempDir()
	p, err := NewAllWorld(root)
	if err != nil {
		t.Fatalf("NewAllWorld: %v", err)
	}
	ctx := context.Background()

	// Prepare a snapshot.
	mounts, err := p.Prepare(ctx, "s1", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if len(mounts) == 0 {
		t.Fatal("expected mounts for OverlayFS")
	}

	// Verify directories exist.
	if _, statErr := os.Stat(mounts[0].Source); statErr != nil {
		t.Errorf("merged dir missing: %v", statErr)
	}
}

func TestAllWorld_Remove_Success(t *testing.T) {
	root := t.TempDir()
	p, err := NewAllWorld(root)
	if err != nil {
		t.Fatalf("NewAllWorld: %v", err)
	}
	ctx := context.Background()

	_, err = p.Prepare(ctx, "s1", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if err = p.Remove(ctx, "s1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Double remove should be fine (idempotent).
	if err = p.Remove(ctx, "s1"); err != nil {
		t.Errorf("second Remove: %v", err)
	}
}

func TestAllWorld_Remove_NotFound(t *testing.T) {
	p, err := NewAllWorld(t.TempDir())
	if err != nil {
		t.Fatalf("NewAllWorld: %v", err)
	}
	// Should not error if snapshot doesn't exist.
	if err = p.Remove(context.Background(), "ghost"); err != nil {
		t.Errorf("Remove non-existent: %v", err)
	}
}

func TestAllWorld_View_Success(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	p, err := NewAllWorld(root)
	if err != nil {
		t.Fatalf("NewAllWorld: %v", err)
	}
	ctx := context.Background()

	// View creates a read-only snapshot
	mounts, err := p.View(ctx, "v1", "")
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	if len(mounts) == 0 {
		t.Fatal("expected mounts")
	}

	meta, err := p.readMeta("v1")
	if err != nil {
		t.Fatalf("readMeta: %v", err)
	}
	if meta.Kind != KindView {
		t.Errorf("expected KindView, got %v", meta.Kind)
	}
}

func TestAllWorld_Commit_Success(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	p, err := NewAllWorld(root)
	if err != nil {
		t.Fatalf("NewAllWorld: %v", err)
	}
	ctx := context.Background()

	_, err = p.Prepare(ctx, "active", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	err = p.Commit(ctx, "committed", "active")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	meta, err := p.readMeta("committed")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Kind != KindCommitted {
		t.Errorf("expected KindCommitted, got %v", meta.Kind)
	}

	// Verify old key is gone (renamed)
	if _, statErr := os.Stat(p.snapshotDir("active")); !os.IsNotExist(statErr) {
		t.Error("expected active snapshot dir to be moved")
	}
}

func TestAllWorld_Commit_Errors(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	p, err := NewAllWorld(root)
	if err != nil {
		t.Fatalf("NewAllWorld: %v", err)
	}
	ctx := context.Background()

	// 1. Commit non-existent
	if err = p.Commit(ctx, "c1", "none"); err == nil {
		t.Error("expected error committing non-existent")
	}

	// 2. Commit a View (should fail)
	_, err = p.View(ctx, "v1", "")
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	if err = p.Commit(ctx, "c1", "v1"); err == nil {
		t.Error("expected error committing a view")
	}
}

func TestAllWorld_Walk(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	p, err := NewAllWorld(root)
	if err != nil {
		t.Fatalf("NewAllWorld: %v", err)
	}
	ctx := context.Background()

	keys := []string{"k1", "k2", "k3"}
	for _, k := range keys {
		if _, prepErr := p.Prepare(ctx, k, ""); prepErr != nil {
			t.Fatalf("Prepare: %v", prepErr)
		}
	}

	var walked []string
	err = p.Walk(ctx, func(info Info) error {
		walked = append(walked, info.Key)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	if len(walked) != 3 {
		t.Errorf("got %d keys, want 3", len(walked))
	}
}

func TestAllWorld_Usage(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	p, err := NewAllWorld(root)
	if err != nil {
		t.Fatalf("NewAllWorld: %v", err)
	}
	ctx := context.Background()

	_, err = p.Prepare(ctx, "s1", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	fsDir := filepath.Join(p.snapshotDir("s1"), "fs")
	err = os.WriteFile(filepath.Join(fsDir, "f1"), []byte("hello"), 0644)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	err = os.WriteFile(filepath.Join(fsDir, "f2"), []byte("world"), 0644)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	usage, err := p.Usage(ctx, "s1")
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}

	// Inodes: f1, f2, and the fs/ dir itself = 3
	if usage.Inodes != 3 {
		t.Errorf("inodes: got %d, want 3", usage.Inodes)
	}
	if usage.Size < 10 {
		t.Errorf("size: got %d, want at least 10", usage.Size)
	}
}

func TestAllWorld_Mounts_Chain(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	p, err := NewAllWorld(root)
	if err != nil {
		t.Fatalf("NewAllWorld: %v", err)
	}
	ctx := context.Background()

	// Chain: s1 (root) -> s2 -> s3
	_, err = p.Prepare(ctx, "s1", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	err = p.Commit(ctx, "c1", "s1")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	_, err = p.Prepare(ctx, "s2", "c1")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	err = p.Commit(ctx, "c2", "s2")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	mounts, err := p.Prepare(ctx, "s3", "c2")
	if err != nil {
		t.Fatalf("Prepare chain: %v", err)
	}

	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}
	m := mounts[0]
	if m.Type != "overlay" {
		t.Errorf("expected overlay, got %s", m.Type)
	}

	// Verify lowerdir contains c2 and c1
	foundC2 := false
	foundC1 := false
	for _, opt := range m.Options {
		if strings.Contains(opt, "lowerdir=") {
			if strings.Contains(opt, "c2/fs") {
				foundC2 = true
			}
			if strings.Contains(opt, "c1/fs") {
				foundC1 = true
			}
		}
	}
	if !foundC2 || !foundC1 {
		t.Errorf("missing layers in lowerdir: c2=%v, c1=%v. Opts: %v", foundC2, foundC1, m.Options)
	}
}

func TestAllWorld_Remove_HasDependents(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	p, err := NewAllWorld(root)
	if err != nil {
		t.Fatalf("NewAllWorld: %v", err)
	}
	ctx := context.Background()

	_, err = p.Prepare(ctx, "parent", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	err = p.Commit(ctx, "p1", "parent")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	_, err = p.Prepare(ctx, "child", "p1")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	// Removing p1 should fail because child depends on it
	err = p.Remove(ctx, "p1")
	if err == nil {
		t.Fatal("expected error removing snapshot with dependents")
	}
}

func TestProbeOverlay_Mock(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	m := &mockMounter{}

	ctx := context.Background()
	err := ProbeOverlay(ctx, dir, m)
	if err != nil {
		t.Fatalf("ProbeOverlay: %v", err)
	}
}

func TestAllWorld_New_InvalidRoot(t *testing.T) {
	t.Parallel()
	tmpFile, err := os.CreateTemp(t.TempDir(), "file")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	_ = tmpFile.Close()
	if _, newAWErr := NewAllWorld(tmpFile.Name()); newAWErr == nil {
		t.Error("expected error from NewAllWorld on invalid root")
	}
}

func TestAllWorld_Prepare_AlreadyExists(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p, err := NewAllWorld(t.TempDir())
	if err != nil {
		t.Fatalf("NewAllWorld: %v", err)
	}
	if _, prepErr := p.Prepare(ctx, "s1", ""); prepErr != nil {
		t.Fatalf("Prepare: %v", prepErr)
	}
	if _, prepErr := p.Prepare(ctx, "s1", ""); prepErr == nil {
		t.Error("expected already exists error")
	}
}

func TestAllWorld_Prepare_MkdirFail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	badDir := t.TempDir()
	pBad, err := NewAllWorld(badDir)
	if err != nil {
		t.Fatalf("NewAllWorld: %v", err)
	}
	if mkdirErr := os.MkdirAll(pBad.snapshotDir("fail"), 0755); mkdirErr != nil {
		t.Fatalf("MkdirAll: %v", mkdirErr)
	}
	if writeErr := os.WriteFile(
		filepath.Join(pBad.snapshotDir("fail"), "fs"),
		[]byte("notadir"),
		0644,
	); writeErr != nil {
		t.Fatalf("WriteFile: %v", writeErr)
	}
	if _, prepErr := pBad.Prepare(ctx, "fail", ""); prepErr == nil {
		t.Error("expected error preparing when fs/ is a file")
	}
}

func TestAllWorld_ReadMeta_Corrupt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pMeta, err := NewAllWorld(t.TempDir())
	if err != nil {
		t.Fatalf("NewAllWorld: %v", err)
	}
	if _, prepErr := pMeta.Prepare(ctx, "corrupt", ""); prepErr != nil {
		t.Fatalf("Prepare: %v", prepErr)
	}
	if writeErr := os.WriteFile(
		filepath.Join(pMeta.snapshotDir("corrupt"), "meta.json"),
		[]byte("{invalid json"),
		0644,
	); writeErr != nil {
		t.Fatalf("WriteFile: %v", writeErr)
	}
	if _, readErr := pMeta.readMeta("corrupt"); readErr == nil {
		t.Error("expected error reading corrupt meta")
	}
}

func TestAllWorld_View_MkdirFail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	badDirView := t.TempDir()
	pBadView, err := NewAllWorld(badDirView)
	if err != nil {
		t.Fatalf("NewAllWorld: %v", err)
	}
	if mkdirErr := os.MkdirAll(pBadView.snapshotDir("fail-view"), 0755); mkdirErr != nil {
		t.Fatalf("MkdirAll: %v", mkdirErr)
	}
	if writeErr := os.WriteFile(
		filepath.Join(pBadView.snapshotDir("fail-view"), "fs"),
		[]byte("blocked"),
		0644,
	); writeErr != nil {
		t.Fatalf("WriteFile: %v", writeErr)
	}
	if _, viewErr := pBadView.View(ctx, "fail-view", ""); viewErr == nil {
		t.Error("expected error viewing when fs/ is a file")
	}
}

func TestAllWorld_View_WriteMetaFail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	badDirMeta := t.TempDir()
	pBadMeta, err := NewAllWorld(badDirMeta)
	if err != nil {
		t.Fatalf("NewAllWorld: %v", err)
	}
	if mkdirErr := os.MkdirAll(pBadMeta.snapshotDir("fail-meta"), 0755); mkdirErr != nil {
		t.Fatalf("MkdirAll: %v", mkdirErr)
	}
	// Block meta file with dir
	if mkdirErr := os.MkdirAll(filepath.Join(pBadMeta.snapshotDir("fail-meta"), "meta.json"), 0755); mkdirErr != nil {
		t.Fatalf("MkdirAll: %v", mkdirErr)
	}
	if _, viewErr := pBadMeta.View(ctx, "fail-meta", ""); viewErr == nil {
		t.Error("expected error viewing when meta.json is blocked")
	}
}

func TestAllWorld_Walk_Errors(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	p, err := NewAllWorld(root)
	if err != nil {
		t.Fatalf("NewAllWorld: %v", err)
	}
	ctx := context.Background()

	_, err = p.Prepare(ctx, "k1", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	// 1. Walk function error
	err = p.Walk(ctx, func(Info) error {
		return errors.New("abort")
	})
	if err == nil || err.Error() != "abort" {
		t.Errorf("expected walk abort error, got %v", err)
	}

	// 2. readMeta error during Walk (corrupt one snapshot)
	_, err = p.Prepare(ctx, "k2", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	m := &mockFS{
		fallback:   RealFS{},
		MkdirAllFn: os.MkdirAll,
		readMetaFn: func(f string) ([]byte, error) {
			if strings.Contains(f, "k1") {
				return nil, errors.New("read-meta-fail")
			}
			return os.ReadFile(f)
		},
	}
	p.WithFS(m)
	count := 0
	err = p.Walk(ctx, func(Info) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 successful walk, got %d", count)
	}
}

func TestAllWorld_Commit_MoreErrors(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	p, err := NewAllWorld(root)
	if err != nil {
		t.Fatalf("NewAllWorld: %v", err)
	}
	ctx := context.Background()

	// 1. writeMeta failure during commit
	_, err = p.Prepare(ctx, "s1", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	m := &mockFS{
		MkdirAllFn: os.MkdirAll,
		WriteFileFn: func(f string, d []byte, perm os.FileMode) error {
			if strings.Contains(f, "meta.json") {
				return errors.New("write-meta-fail")
			}
			return os.WriteFile(f, d, perm)
		},
	}
	p.WithFS(m)

	err = p.Commit(ctx, "c1", "s1")
	if err == nil {
		t.Error("expected error committing when meta.json is blocked")
	}
}

func TestAllWorld_Remove_Errors(t *testing.T) {
	t.Parallel()
	p, err := NewAllWorld(t.TempDir())
	if err != nil {
		t.Fatalf("NewAllWorld: %v", err)
	}
	ctx := context.Background()

	m := &mockFS{
		readDirErr: errors.New("has-dependents-fail"),
	}
	p.WithFS(m)
	err = p.Remove(ctx, "any")
	if err == nil {
		t.Error("expected error removing when snapshots dir is unreadable")
	}
}

func TestAllWorld_Mounts_Errors(t *testing.T) {
	t.Parallel()
	p, err := NewAllWorld(t.TempDir())
	if err != nil {
		t.Fatalf("NewAllWorld: %v", err)
	}

	_, err = p.mounts("child", "nonexistent")
	if err == nil {
		t.Error("expected error for mounts with missing parent meta")
	}
}

func TestAllWorld_CoverageHelpers(t *testing.T) {
	t.Parallel()
	p, err := NewAllWorld(t.TempDir())
	if err != nil {
		t.Fatalf("NewAllWorld: %v", err)
	}
	p.WithMounter(&RealMounter{})
	if p.WritableDir("any") == "" {
		t.Error("WritableDir returned empty")
	}
	if p.WhiteoutFormat() != archive.WhiteoutOverlay {
		t.Errorf("got whiteout format %v, want overlay", p.WhiteoutFormat())
	}
}

func TestRealFS_Coverage(t *testing.T) {
	t.Parallel()
	fs := RealFS{}
	tmp := filepath.Join(t.TempDir(), "realfs-test")
	err := fs.MkdirAll(tmp, 0700)
	if err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if _, err = fs.Stat(tmp); err != nil {
		t.Errorf("Stat failed: %v", err)
	}
	if !fs.IsNotExist(os.ErrNotExist) {
		t.Error("IsNotExist(os.ErrNotExist) should be true")
	}
	err = fs.Remove(tmp)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
}

package prim

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── tests ──────────────────────────────────────────────────────────────────────

func TestDetect_Auto_Success(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	res, err := Detect(ctx, root, false, nil, nil)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if res.Prim == nil {
		t.Fatal("expected non-nil Prim implementation")
	}
	if res.Driver != DriverAllWorld && res.Driver != DriverVFS && res.Driver != DriverFuseOverlay {
		t.Errorf("unexpected driver: %v", res.Driver)
	}
}

func TestDetect_ForceVFS(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ctx := context.Background()
	res, err := Detect(ctx, root, true, nil, nil)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if res.Driver != DriverVFS {
		t.Errorf("expected DriverVFS, got %v", res.Driver)
	}
}

func TestDetect_OverlayProbeFailure_FallbackToVFS(t *testing.T) {
	root := t.TempDir()

	// Mock mnt to always fail
	m := &mockMounter{mountErr: errors.New("overlay mount not supported")}

	// Mock findBinary to fail too, forcing VFS fallback
	oldFind := findBinary
	findBinary = func(_ string) (string, error) {
		return "", errors.New("not found")
	}
	defer func() { findBinary = oldFind }()

	ctx := context.Background()
	res, err := Detect(ctx, root, false, m, nil)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if res.Driver != DriverVFS {
		t.Errorf("expected fallback to DriverVFS, got %v", res.Driver)
	}
}

func TestDetect_RootlessFuseOverlayFallback(t *testing.T) {
	root := t.TempDir()

	// Mock mnt to fail for native overlay
	m := &mockMounter{mountErr: errors.New("overlay mount not supported")}

	// Mock findBinary to succeed for fuse-overlayfs
	oldFind := findBinary
	findBinary = func(name string) (string, error) {
		if name == string(DriverFuseOverlay) {
			return "/fake/fuse-overlayfs", nil
		}
		return "", errors.New("not found")
	}
	defer func() { findBinary = oldFind }()

	ctx := context.Background()
	res, err := Detect(ctx, root, false, m, nil)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}

	// If we are rootless, it should be FuseOverlay. If we are root, it should be VFS.
	expected := DriverVFS
	if isRootless() {
		expected = DriverFuseOverlay
	}

	if res.Driver != expected {
		t.Errorf("expected %v, got %v", expected, res.Driver)
	}
}

func TestDetect_NewVFS_Failure(t *testing.T) {
	t.Parallel()
	// root is a regular file — NewVFS will fail
	tmpFile, createErr := os.CreateTemp(t.TempDir(), "file")
	if createErr != nil {
		t.Fatalf("fail to create temp file: %v", createErr)
	}
	if closeErr := tmpFile.Close(); closeErr != nil {
		t.Fatalf("fail to close temp file: %v", closeErr)
	}

	ctx := context.Background()
	_, err := Detect(ctx, tmpFile.Name(), true, nil, nil)
	if err == nil {
		t.Error("expected error from Detect when NewVFS fails")
	}
}

func TestDetect_ProbeMkdirFailure(t *testing.T) {
	// Root is a file — NewVFS/NewAllWorld should fail during Detect
	root := t.TempDir()
	blocked := filepath.Join(root, "blocked")
	if err := os.WriteFile(blocked, []byte("xxx"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx := context.Background()
	_, err := Detect(ctx, blocked, false, nil, nil)
	if err == nil {
		t.Error("expected error from Detect when root is an existing file")
	}
}

func TestDetect_AllWorldSuccess(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// Mock mnt to succeed
	m := &mockMounter{}
	ctx := context.Background()
	res, err := Detect(ctx, root, false, m, nil)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if res.Driver != DriverAllWorld {
		t.Errorf("expected DriverAllWorld, got %v", res.Driver)
	}
}

func TestDetect_AllWorldInitFailure(t *testing.T) {
	t.Parallel()
	// root is a file — NewAllWorld will fail
	root := t.TempDir()
	blocked := filepath.Join(root, "blocked")
	if err := os.WriteFile(blocked, []byte("xxx"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Mock mnt to succeed so it tries to call NewAllWorld
	m := &mockMounter{}

	ctx := context.Background()
	_, err := Detect(ctx, blocked, false, m, nil)
	if err == nil {
		t.Error("expected error when NewAllWorld fails during Detect")
	}
}

func TestDetect_MkdirTempFail(t *testing.T) {
	root := t.TempDir()
	// To ensure we see the MkdirTemp error from detectAllWorld,
	// we need to make sure subsequent detections (Fuse and VFS) also fail
	// OR we test detectAllWorld directly.
	// Testing detectAllWorld directly is better for this specific unit test.

	m := &mockFS{
		mkdirTempErr: errors.New("mkdir-temp-fail"),
	}
	ctx := context.Background()
	_, err := detectAllWorld(ctx, root, nil, m, false)
	if err == nil || !strings.Contains(err.Error(), "mkdir-temp-fail") {
		t.Errorf("got error %v, want mkdir-temp-fail", err)
	}
}

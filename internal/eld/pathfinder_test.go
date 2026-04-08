package eld_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rodrigo-baliza/maestro/internal/eld"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// makeFakeRuntime creates a tiny shell script at dir/name that outputs version
// on --version and exits 0, or exits 1 for other args.
func makeFakeRuntime(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo \"" + name + " version 1.2.3\"; exit 0; fi\nexit 1\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake runtime %s: %v", name, err)
	}
	return path
}

// makeBrokenRuntime creates a binary that always exits non-zero.
func makeBrokenRuntime(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write broken runtime %s: %v", name, err)
	}
	return path
}

// newPathfinderWith returns a Pathfinder that uses the provided lookPath and
// runVersion functions — allowing full control in tests.
func newPathfinderWith(
	lookPath func(string) (string, error),
	runVersion func(string) (string, error),
) *eld.Pathfinder {
	return &eld.Pathfinder{
		LookPathFn:   lookPath,
		RunVersionFn: runVersion,
	}
}

// ── tests ──────────────────────────────────────────────────────────────────────

func TestPathfinder_Discover_ConfigOverride_Success(t *testing.T) {
	dir := t.TempDir()
	binPath := makeFakeRuntime(t, dir, "crun")

	pf := eld.NewPathfinder()
	info, err := pf.Discover(binPath, "crun")
	if err != nil {
		t.Fatalf("Discover with config override: %v", err)
	}
	if info.Name != "crun" {
		t.Errorf("Name = %q; want crun", info.Name)
	}
	if info.Version == "" {
		t.Error("Version should not be empty")
	}
}

func TestPathfinder_Discover_ConfigOverride_NotFound(t *testing.T) {
	pf := eld.NewPathfinder()
	_, err := pf.Discover("/nonexistent/path/crun", "crun")
	if err == nil {
		t.Fatal("expected error for nonexistent binary")
	}
}

func TestPathfinder_Discover_ConfigOverride_BrokenBinary(t *testing.T) {
	dir := t.TempDir()
	binPath := makeBrokenRuntime(t, dir, "badrunc")

	pf := eld.NewPathfinder()
	_, err := pf.Discover(binPath, "badrunc")
	if err == nil {
		t.Fatal("expected error for runtime that fails --version")
	}
}

func TestPathfinder_Discover_PathSearch_CrunFirst(t *testing.T) {
	dir := t.TempDir()
	makeFakeRuntime(t, dir, "crun")
	makeFakeRuntime(t, dir, "runc")

	pf := newPathfinderWith(
		func(name string) (string, error) {
			p := filepath.Join(dir, name)
			if _, err := os.Stat(p); err != nil {
				return "", err
			}
			return p, nil
		},
		eld.DefaultRunVersionFn,
	)

	info, err := pf.Discover("", "")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if info.Name != "crun" {
		t.Errorf("Name = %q; want crun (highest priority)", info.Name)
	}
}

func TestPathfinder_Discover_PathSearch_RuncFallback(t *testing.T) {
	dir := t.TempDir()
	// Only runc is available.
	makeFakeRuntime(t, dir, "runc")

	pf := newPathfinderWith(
		func(name string) (string, error) {
			p := filepath.Join(dir, name)
			if _, err := os.Stat(p); err != nil {
				return "", err
			}
			return p, nil
		},
		eld.DefaultRunVersionFn,
	)

	info, err := pf.Discover("", "")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if info.Name != "runc" {
		t.Errorf("Name = %q; want runc", info.Name)
	}
}

func TestPathfinder_Discover_PathSearch_YoukiFallback(t *testing.T) {
	dir := t.TempDir()
	makeFakeRuntime(t, dir, "youki")

	pf := newPathfinderWith(
		func(name string) (string, error) {
			p := filepath.Join(dir, name)
			if _, err := os.Stat(p); err != nil {
				return "", err
			}
			return p, nil
		},
		eld.DefaultRunVersionFn,
	)

	info, err := pf.Discover("", "")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if info.Name != "youki" {
		t.Errorf("Name = %q; want youki", info.Name)
	}
}

func TestPathfinder_Discover_NoRuntimeFound(t *testing.T) {
	pf := newPathfinderWith(
		func(_ string) (string, error) {
			return "", errors.New("not found")
		},
		eld.DefaultRunVersionFn,
	)

	_, err := pf.Discover("", "")
	if err == nil {
		t.Fatal("expected ErrRuntimeNotFound")
	}
	if !errors.Is(err, eld.ErrRuntimeNotFound) {
		t.Errorf("expected ErrRuntimeNotFound; got: %v", err)
	}
}

func TestPathfinder_Discover_SkipsBrokenBinary(t *testing.T) {
	dir := t.TempDir()
	makeBrokenRuntime(t, dir, "crun") // crun is broken
	makeFakeRuntime(t, dir, "runc")   // runc is valid
	// youki not present

	pf := newPathfinderWith(
		func(name string) (string, error) {
			p := filepath.Join(dir, name)
			if _, err := os.Stat(p); err != nil {
				return "", err
			}
			return p, nil
		},
		eld.DefaultRunVersionFn,
	)

	info, err := pf.Discover("", "")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if info.Name != "runc" {
		t.Errorf("expected runc (skipping broken crun); got %q", info.Name)
	}
}

func TestPathfinder_Discover_AllBroken_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	makeBrokenRuntime(t, dir, "crun")
	makeBrokenRuntime(t, dir, "runc")
	makeBrokenRuntime(t, dir, "youki")

	pf := newPathfinderWith(
		func(name string) (string, error) {
			p := filepath.Join(dir, name)
			if _, err := os.Stat(p); err != nil {
				return "", err
			}
			return p, nil
		},
		eld.DefaultRunVersionFn,
	)

	_, err := pf.Discover("", "")
	if err == nil {
		t.Fatal("expected error when all runtimes fail validation")
	}
	if !errors.Is(err, eld.ErrRuntimeNotFound) {
		t.Errorf("expected ErrRuntimeNotFound; got: %v", err)
	}
}

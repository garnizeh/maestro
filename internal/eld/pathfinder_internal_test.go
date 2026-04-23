package eld

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// makeFakeRuntime creates a tiny shell script at dir/name that outputs version
// on --version and exits 0, or exits 1 for other args.
func makeFakeRuntime(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo \"" + name + " version 1.2.3\"; exit 0; fi\nexit 1\n"

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		t.Fatalf("open fake runtime %s: %v", name, err)
	}

	if _, err = f.WriteString(script); err != nil {
		f.Close()
		t.Fatalf("write fake runtime %s: %v", name, err)
	}

	if err = f.Sync(); err != nil {
		f.Close()
		t.Fatalf("sync fake runtime %s: %v", name, err)
	}

	if err = f.Close(); err != nil {
		t.Fatalf("close fake runtime %s: %v", name, err)
	}

	return path
}

// makeBrokenRuntime creates a binary that always exits non-zero.
func makeBrokenRuntime(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\nexit 1\n"

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		t.Fatalf("open broken runtime %s: %v", name, err)
	}

	if _, err = f.WriteString(script); err != nil {
		f.Close()
		t.Fatalf("write broken runtime %s: %v", name, err)
	}

	if err = f.Sync(); err != nil {
		f.Close()
		t.Fatalf("sync broken runtime %s: %v", name, err)
	}

	if err = f.Close(); err != nil {
		t.Fatalf("close broken runtime %s: %v", name, err)
	}

	return path
}

// newPathfinderWithDI returns a Pathfinder that uses the provided lookPath
// function — allowing full control in tests.
func newPathfinderWithDI(
	lookPath func(string) (string, error),
) *Pathfinder {
	// We'll wrap lookPath to use a custom implementation
	return NewPathfinder().WithCommander(&wrappedCommander{lookPath: lookPath})
}

type wrappedCommander struct {
	RealCommander

	lookPath func(string) (string, error)
}

func (c *wrappedCommander) LookPath(file string) (string, error) {
	if c.lookPath != nil {
		return c.lookPath(file)
	}
	return c.RealCommander.LookPath(file)
}

// ── tests ──────────────────────────────────────────────────────────────────────

func TestPathfinder_Discover_ConfigOverride_Success(t *testing.T) {
	// t.Parallel() disabled for stability
	dir := t.TempDir()
	binPath := makeFakeRuntime(t, dir, "crun")

	pf := NewPathfinder()
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
	t.Parallel()
	pf := NewPathfinder()
	_, err := pf.Discover("/nonexistent/path/crun", "crun")
	if err == nil {
		t.Fatal("expected error for nonexistent binary")
	}
}

func TestPathfinder_Discover_ConfigOverride_BrokenBinary(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	binPath := makeBrokenRuntime(t, dir, "badrunc")

	pf := NewPathfinder()
	_, err := pf.Discover(binPath, "badrunc")
	if err == nil {
		t.Fatal("expected error for runtime that fails --version")
	}
}

func TestPathfinder_Validate_AbsError(t *testing.T) {
	t.Parallel()
	fs := &mockFS{AbsFn: func(string) (string, error) { return "", errors.New("abs fail") }}
	pf := NewPathfinder().WithFS(fs)
	dir := t.TempDir()
	binPath := makeFakeRuntime(t, dir, "crun")
	_, err := pf.Discover(binPath, "crun")
	if err == nil || !strings.Contains(err.Error(), "abs fail") {
		t.Errorf("expected abs error, got %v", err)
	}
}

func TestPathfinder_Discover_PathSearch_CrunFirst(t *testing.T) {
	// t.Parallel() disabled for stability
	dir := t.TempDir()
	makeFakeRuntime(t, dir, "crun")
	makeFakeRuntime(t, dir, "runc")

	pf := newPathfinderWithDI(
		func(name string) (string, error) {
			p := filepath.Join(dir, name)
			if _, err := os.Stat(p); err != nil {
				return "", err
			}
			return p, nil
		},
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
	// t.Parallel() disabled for stability
	dir := t.TempDir()
	// Only runc is available.
	makeFakeRuntime(t, dir, "runc")

	pf := newPathfinderWithDI(
		func(name string) (string, error) {
			p := filepath.Join(dir, name)
			if _, err := os.Stat(p); err != nil {
				return "", err
			}
			return p, nil
		},
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
	// t.Parallel() disabled for stability
	dir := t.TempDir()
	makeFakeRuntime(t, dir, "youki")

	pf := newPathfinderWithDI(
		func(name string) (string, error) {
			p := filepath.Join(dir, name)
			if _, err := os.Stat(p); err != nil {
				return "", err
			}
			return p, nil
		},
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
	t.Parallel()
	pf := newPathfinderWithDI(
		func(_ string) (string, error) {
			return "", errors.New("not found")
		},
	)

	_, err := pf.Discover("", "")
	if err == nil {
		t.Fatal("expected ErrRuntimeNotFound")
	}
	if !errors.Is(err, ErrRuntimeNotFound) {
		t.Errorf("expected ErrRuntimeNotFound; got: %v", err)
	}
}

func TestPathfinder_Discover_SkipsBrokenBinary(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	makeBrokenRuntime(t, dir, "crun") // crun is broken
	makeFakeRuntime(t, dir, "runc")   // runc is valid
	// youki not present

	pf := newPathfinderWithDI(
		func(name string) (string, error) {
			p := filepath.Join(dir, name)
			if _, err := os.Stat(p); err != nil {
				return "", err
			}
			return p, nil
		},
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
	t.Parallel()
	dir := t.TempDir()
	makeBrokenRuntime(t, dir, "crun")
	makeBrokenRuntime(t, dir, "runc")
	makeBrokenRuntime(t, dir, "youki")

	pf := newPathfinderWithDI(
		func(name string) (string, error) {
			p := filepath.Join(dir, name)
			if _, err := os.Stat(p); err != nil {
				return "", err
			}
			return p, nil
		},
	)

	_, err := pf.Discover("", "")
	if err == nil {
		t.Fatal("expected error when all runtimes fail validation")
	}
	if !errors.Is(err, ErrRuntimeNotFound) {
		t.Errorf("expected ErrRuntimeNotFound; got: %v", err)
	}
}

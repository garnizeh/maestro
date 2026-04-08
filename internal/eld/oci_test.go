package eld_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"

	"github.com/rodrigo-baliza/maestro/internal/eld"
)

// ── fake runtime binary helpers ───────────────────────────────────────────────

// fakeRuntimePath creates a temporary directory containing a fake OCI runtime
// script that behaves predictably for testing. Returns the path to the fake binary.
func fakeRuntimePath(t *testing.T, responses map[string]fakeResponse) (binPath string) {
	t.Helper()
	dir := t.TempDir()
	binPath = dir + "/fake-runtime"

	// Build a shell script that matches $1 (the subcommand) and echos the response.
	var sb strings.Builder
	sb.WriteString("#!/bin/sh\n")
	sb.WriteString("case \"$1\" in\n")
	for sub, resp := range responses {
		fmt.Fprintf(&sb, "  %s)\n", sub)
		if resp.stdout != "" {
			fmt.Fprintf(&sb, "    echo '%s'\n", resp.stdout)
		}
		if resp.stderr != "" {
			fmt.Fprintf(&sb, "    echo '%s' >&2\n", resp.stderr)
		}
		fmt.Fprintf(&sb, "    exit %d\n", resp.exitCode)
		sb.WriteString("    ;;\n")
	}
	sb.WriteString("  *)\n    echo \"unknown subcommand: $1\" >&2\n    exit 1\n    ;;\n")
	sb.WriteString("esac\n")

	if err := os.WriteFile(binPath, []byte(sb.String()), 0o755); err != nil {
		t.Fatalf("write fake runtime: %v", err)
	}
	return binPath
}

type fakeResponse struct {
	stdout   string
	stderr   string
	exitCode int
}

func makeRuntime(t *testing.T, responses map[string]fakeResponse) *eld.OCIRuntime {
	t.Helper()
	binPath := fakeRuntimePath(t, responses)
	return eld.NewOCIRuntime(eld.RuntimeInfo{Name: "fake", Path: binPath, Version: "0.0.1"})
}

// ── tests ──────────────────────────────────────────────────────────────────────

func TestOCIRuntime_Create_Success(t *testing.T) {
	r := makeRuntime(t, map[string]fakeResponse{
		"create": {exitCode: 0},
	})
	if err := r.Create(context.Background(), "ctr1", "/bundle", nil); err != nil {
		t.Fatalf("Create: %v", err)
	}
}

func TestOCIRuntime_Create_NoPivot(t *testing.T) {
	binPath := fakeRuntimePath(t, map[string]fakeResponse{})
	// Override ExecCommandFn to capture args.
	var capturedArgs []string
	r := eld.NewOCIRuntime(eld.RuntimeInfo{Name: "fake", Path: binPath})
	r.ExecCommandFn = func(_ context.Context, _ string, arg ...string) *exec.Cmd {
		capturedArgs = arg
		// Return a command that succeeds immediately.
		return exec.Command("true")
	}
	opts := &eld.CreateOpts{NoPivot: true, ExtraArgs: []string{"--rootless"}}
	_ = r.Create(context.Background(), "ctr1", "/bundle", opts)
	found := false
	for _, a := range capturedArgs {
		if a == "--no-pivot" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected --no-pivot in args %v", capturedArgs)
	}
}

func TestOCIRuntime_Create_Failure(t *testing.T) {
	r := makeRuntime(t, map[string]fakeResponse{
		"create": {stderr: "bundle not found", exitCode: 1},
	})
	err := r.Create(context.Background(), "ctr1", "/bundle", nil)
	if err == nil {
		t.Fatal("expected error on runtime failure")
	}
	if !strings.Contains(err.Error(), "bundle not found") {
		t.Errorf("error message = %q; want it to contain 'bundle not found'", err.Error())
	}
}

func TestOCIRuntime_Start_Success(t *testing.T) {
	r := makeRuntime(t, map[string]fakeResponse{
		"start": {exitCode: 0},
	})
	if err := r.Start(context.Background(), "ctr1"); err != nil {
		t.Fatalf("Start: %v", err)
	}
}

func TestOCIRuntime_Start_Failure(t *testing.T) {
	r := makeRuntime(t, map[string]fakeResponse{
		"start": {stderr: "not found", exitCode: 1},
	})
	if err := r.Start(context.Background(), "ctr1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestOCIRuntime_Kill_Success(t *testing.T) {
	r := makeRuntime(t, map[string]fakeResponse{
		"kill": {exitCode: 0},
	})
	if err := r.Kill(context.Background(), "ctr1", syscall.SIGTERM); err != nil {
		t.Fatalf("Kill: %v", err)
	}
}

func TestOCIRuntime_Delete_Success(t *testing.T) {
	r := makeRuntime(t, map[string]fakeResponse{
		"delete": {exitCode: 0},
	})
	if err := r.Delete(context.Background(), "ctr1", nil); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestOCIRuntime_Delete_Force(t *testing.T) {
	binPath := fakeRuntimePath(t, map[string]fakeResponse{})
	var capturedArgs []string
	r := eld.NewOCIRuntime(eld.RuntimeInfo{Name: "fake", Path: binPath})
	r.ExecCommandFn = func(_ context.Context, _ string, arg ...string) *exec.Cmd {
		capturedArgs = arg
		return exec.Command("true")
	}
	_ = r.Delete(context.Background(), "ctr1", &eld.DeleteOpts{Force: true})
	found := false
	for _, a := range capturedArgs {
		if a == "--force" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected --force in args %v", capturedArgs)
	}
}

func TestOCIRuntime_State_Success(t *testing.T) {
	stateJSON := `{"ociVersion":"1.0.2","id":"ctr1","status":"running","pid":1234,"bundle":"/b"}`
	r := makeRuntime(t, map[string]fakeResponse{
		"state": {stdout: stateJSON, exitCode: 0},
	})
	s, err := r.State(context.Background(), "ctr1")
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if s.Status != eld.StatusRunning {
		t.Errorf("Status = %q; want %q", s.Status, eld.StatusRunning)
	}
	if s.Pid != 1234 {
		t.Errorf("Pid = %d; want 1234", s.Pid)
	}
}

func TestOCIRuntime_State_NotFound(t *testing.T) {
	r := makeRuntime(t, map[string]fakeResponse{
		"state": {stderr: "container does not exist", exitCode: 1},
	})
	_, err := r.State(context.Background(), "ctr1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v; want it to contain 'not found'", err)
	}
}

func TestOCIRuntime_State_ParseError(t *testing.T) {
	r := makeRuntime(t, map[string]fakeResponse{
		"state": {stdout: "invalid json", exitCode: 0},
	})
	_, err := r.State(context.Background(), "ctr1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestOCIRuntime_Features_Success(t *testing.T) {
	featJSON := `{"namespaces":["mount","pid"],"cgroupsV2":true,"seccomp":true}`
	r := makeRuntime(t, map[string]fakeResponse{
		"features": {stdout: featJSON, exitCode: 0},
	})
	f, err := r.Features(context.Background())
	if err != nil {
		t.Fatalf("Features: %v", err)
	}
	if !f.Seccomp {
		t.Error("expected Seccomp=true")
	}
}

func TestOCIRuntime_Features_Fallback(t *testing.T) {
	// runtime returns 1 (doesn't support features command)
	r := makeRuntime(t, map[string]fakeResponse{
		"features": {exitCode: 1},
	})
	f, err := r.Features(context.Background())
	if err != nil {
		t.Fatalf("Features fallback: %v", err)
	}
	if f == nil {
		t.Fatal("expected non-nil default features")
	}
}

func TestOCIRuntime_Run_Failure(t *testing.T) {
	// Test the generic run helper with a command that fails.
	binPath := fakeRuntimePath(t, map[string]fakeResponse{})
	r := eld.NewOCIRuntime(eld.RuntimeInfo{Name: "fake", Path: binPath})
	r.ExecCommandFn = func(_ context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.Command("ls", "/nonexistent") // guaranteed to fail
	}
	err := r.Start(context.Background(), "ctr1")
	if err == nil {
		t.Fatal("expected error from failing command")
	}
}

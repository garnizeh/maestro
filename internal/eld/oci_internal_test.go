package eld

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"

	"github.com/garnizeh/maestro/internal/testutil"
)

type fakeResponse struct {
	stdout   string
	stderr   string
	exitCode int
}

func makeRuntime(t *testing.T, responses map[string]fakeResponse) *OCIRuntime {
	t.Helper()
	cmdFn := func(_ string, args ...string) *exec.Cmd {
		sub := ""
		if len(args) > 0 {
			sub = args[0]
		}
		resp, ok := responses[sub]
		if !ok {
			return exec.Command("sh", "-c", "echo 'unknown subcommand' >&2; exit 1")
		}
		script := fmt.Sprintf(
			"echo '%s'; echo '%s' >&2; exit %d",
			resp.stdout,
			resp.stderr,
			resp.exitCode,
		)
		return exec.Command("sh", "-c", script)
	}
	mc := &mockCommander{
		CommandFn: cmdFn,
		CommandContextFn: func(_ context.Context, name string, args ...string) *exec.Cmd {
			return cmdFn(name, args...)
		},
	}
	r := NewOCIRuntime(RuntimeInfo{Name: "fake", Path: "fake-runtime", Version: "0.0.1"})
	r.WithCommander(mc)
	return r
}

// ── tests ──────────────────────────────────────────────────────────────────────

func TestOCIRuntime_Create_Success(t *testing.T) {
	t.Parallel()
	r := makeRuntime(t, map[string]fakeResponse{
		"create": {exitCode: 0},
	})
	if err := r.Create(context.Background(), "ctr1", "/bundle", nil); err != nil {
		t.Fatalf("Create: %v", err)
	}
}

func TestOCIRuntime_Info(t *testing.T) {
	t.Parallel()
	info := RuntimeInfo{Name: "crun", Path: "/usr/bin/crun", Version: "1.0"}
	r := NewOCIRuntime(info)
	if r.Info() != info {
		t.Errorf("Info() = %v; want %v", r.Info(), info)
	}
}

func TestOCIRuntime_Create_NoPivot(t *testing.T) {
	t.Parallel()
	mc := &mockCommander{
		CommandFn: func(_ string, _ ...string) *exec.Cmd {
			return exec.Command("true")
		},
		CommandContextFn: func(_ context.Context, _ string, _ ...string) *exec.Cmd {
			return exec.Command("true")
		},
	}
	r := NewOCIRuntime(RuntimeInfo{Name: "fake", Path: "/bin/fake"}).WithCommander(mc)
	opts := &CreateOpts{NoPivot: true, ExtraArgs: []string{"--rootless"}}
	if err := r.Create(context.Background(), "ctr1", "/bundle", opts); err != nil {
		t.Fatalf("Create: %v", err)
	}
	found := false
	for _, a := range mc.CapturedArgs {
		if a == "--no-pivot" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected --no-pivot in args %v", mc.CapturedArgs)
	}
}

func TestOCIRuntime_Create_Failure(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	r := makeRuntime(t, map[string]fakeResponse{
		"start": {exitCode: 0},
	})
	if err := r.Start(context.Background(), "ctr1", nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
}

func TestOCIRuntime_Start_Failure(t *testing.T) {
	t.Parallel()
	r := makeRuntime(t, map[string]fakeResponse{
		"start": {stderr: "not found", exitCode: 1},
	})
	if err := r.Start(context.Background(), "ctr1", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestOCIRuntime_Kill_Success(t *testing.T) {
	t.Parallel()
	r := makeRuntime(t, map[string]fakeResponse{
		"kill": {exitCode: 0},
	})
	if err := r.Kill(context.Background(), "ctr1", syscall.SIGTERM); err != nil {
		t.Fatalf("Kill: %v", err)
	}
}

func TestOCIRuntime_Delete_Success(t *testing.T) {
	t.Parallel()
	r := makeRuntime(t, map[string]fakeResponse{
		"delete": {exitCode: 0},
	})
	if err := r.Delete(context.Background(), "ctr1", nil); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestOCIRuntime_Delete_Force(t *testing.T) {
	t.Parallel()
	mc := &mockCommander{
		CommandFn: func(_ string, _ ...string) *exec.Cmd {
			return exec.Command("true")
		},
		CommandContextFn: func(_ context.Context, _ string, _ ...string) *exec.Cmd {
			return exec.Command("true")
		},
	}
	r := NewOCIRuntime(RuntimeInfo{Name: "fake", Path: "/bin/fake"}).WithCommander(mc)
	if err := r.Delete(context.Background(), "ctr1", &DeleteOpts{Force: true}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	found := false
	for _, a := range mc.CapturedArgs {
		if a == "--force" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected --force in args %v", mc.CapturedArgs)
	}
}

func TestOCIRuntime_State_Success(t *testing.T) {
	t.Parallel()
	stateJSON := `{"ociVersion":"1.0.2","id":"ctr1","status":"running","pid":1234,"bundle":"/b"}`
	r := makeRuntime(t, map[string]fakeResponse{
		"state": {stdout: stateJSON, exitCode: 0},
	})
	s, err := r.State(context.Background(), "ctr1")
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if s.Status != StatusRunning {
		t.Errorf("Status = %q; want %q", s.Status, StatusRunning)
	}
	if s.Pid != 1234 {
		t.Errorf("Pid = %d; want 1234", s.Pid)
	}
}

func TestOCIRuntime_State_NotFound(t *testing.T) {
	t.Parallel()
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

func TestOCIRuntime_State_GenericFailure(t *testing.T) {
	t.Parallel()
	r := makeRuntime(t, map[string]fakeResponse{
		"state": {stderr: "permission denied", exitCode: 1},
	})
	_, err := r.State(context.Background(), "ctr1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error = %v; want it to contain 'permission denied'", err)
	}
}

func TestOCIRuntime_State_ParseError(t *testing.T) {
	t.Parallel()
	r := makeRuntime(t, map[string]fakeResponse{
		"state": {stdout: "invalid json", exitCode: 0},
	})
	_, err := r.State(context.Background(), "ctr1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestOCIRuntime_Features_Success(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

func TestOCIRuntime_Features_JSONError(t *testing.T) {
	t.Parallel()
	r := makeRuntime(t, map[string]fakeResponse{
		"features": {stdout: "{invalid", exitCode: 0},
	})
	f, err := r.Features(context.Background())
	if err != nil {
		t.Fatalf("Features: %v", err)
	}
	if !f.Seccomp {
		t.Error("expected default Seccomp=true on JSON error")
	}
}

func TestOCIRuntime_Features_CgroupsV2(t *testing.T) {
	t.Parallel()
	featJSON := `{"linux":{"cgroups":["v2"]},"seccomp":{"enabled":true}}`
	r := makeRuntime(t, map[string]fakeResponse{
		"features": {stdout: featJSON, exitCode: 0},
	})
	f, err := r.Features(context.Background())
	if err != nil {
		t.Fatalf("Features: %v", err)
	}
	if !f.CgroupsV2 {
		t.Error("expected CgroupsV2=true")
	}
}

func TestOCIRuntime_Run_Failure(t *testing.T) {
	t.Parallel()
	// Test the generic run helper with a command that fails.
	mc := &mockCommander{
		CommandFn: func(_ string, _ ...string) *exec.Cmd {
			return exec.Command("ls", "/nonexistent")
		},
		CommandContextFn: func(_ context.Context, _ string, _ ...string) *exec.Cmd {
			return exec.Command("ls", "/nonexistent")
		},
	}
	r := NewOCIRuntime(RuntimeInfo{Name: "fake", Path: "/bin/fake"}).WithCommander(mc)
	err := r.Start(context.Background(), "ctr1", nil)
	if err == nil {
		t.Fatal("expected error from failing command")
	}
}

func TestOCIRuntime_Run_CreateTempFailure(t *testing.T) {
	t.Parallel()
	fs := &testutil.MockFS{
		CreateTempFn: func(_, _ string) (*os.File, error) {
			return nil, errors.New("temp fail")
		},
	}
	r := NewOCIRuntime(RuntimeInfo{Name: "fake", Path: "/bin/true"}).WithFS(fs)
	err := r.Start(context.Background(), "ctr1", nil)
	if err == nil || !strings.Contains(err.Error(), "create stderr temp file") {
		t.Errorf("expected temp file error, got %v", err)
	}
}

func TestHelpers(t *testing.T) {
	t.Parallel()
	t.Run("isNotFoundErr", func(t *testing.T) {
		t.Parallel()
		if !isNotFoundErr(errors.New("fail"), []byte("container not found")) {
			t.Error("expected true for 'not found'")
		}
		if isNotFoundErr(nil, nil) {
			t.Error("expected false for nil error")
		}
		if isNotFoundErr(errors.New("fail"), []byte("generic error")) {
			t.Error("expected false for generic error")
		}
	})

	t.Run("containsInsensitive", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			s, sub string
			want   bool
		}{
			{"HELLO WORLD", "hello", true},
			{"foo", "FOO", true},
			{"abc", "abcd", false},
			{"mixed CASE", "Case", true},
			{"", "a", false},
			{"a", "", true},
		}
		for _, tc := range cases {
			if got := containsInsensitive(tc.s, tc.sub); got != tc.want {
				t.Errorf("containsInsensitive(%q, %q) = %v; want %v", tc.s, tc.sub, got, tc.want)
			}
		}
	})

	t.Run("fmtError", func(t *testing.T) {
		t.Parallel()
		err := errors.New("base")

		var buf bytes.Buffer
		buf.WriteString("  extra details  ")
		formatted := fmtError(err, &buf)
		if !strings.Contains(formatted.Error(), "extra details") {
			t.Errorf("expected extra details, got %v", formatted)
		}
		if got := fmtError(err, nil); !errors.Is(got, err) {
			t.Error("expected original error when buffer is nil")
		}
		if got := fmtError(err, &bytes.Buffer{}); !errors.Is(got, err) {
			t.Error("expected original error when buffer is empty")
		}
	})
}

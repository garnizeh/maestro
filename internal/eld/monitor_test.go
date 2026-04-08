package eld_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rodrigo-baliza/maestro/internal/eld"
)

// ── fake Eld implementation ───────────────────────────────────────────────────

type fakeEld struct {
	createErr    error
	startErr     error
	stateResults []*eld.State
	stateIdx     int
	stateErr     error
}

func (f *fakeEld) Create(_ context.Context, _, _ string, _ *eld.CreateOpts) error {
	return f.createErr
}

func (f *fakeEld) Start(_ context.Context, _ string) error {
	return f.startErr
}

func (f *fakeEld) Kill(_ context.Context, _ string, _ syscall.Signal) error {
	return nil
}

func (f *fakeEld) Delete(_ context.Context, _ string, _ *eld.DeleteOpts) error {
	return nil
}

func (f *fakeEld) State(_ context.Context, id string) (*eld.State, error) {
	if f.stateErr != nil {
		return nil, f.stateErr
	}
	if f.stateIdx >= len(f.stateResults) {
		return &eld.State{ID: id, Status: eld.StatusStopped}, nil
	}
	s := f.stateResults[f.stateIdx]
	f.stateIdx++
	return s, nil
}

func (f *fakeEld) Features(_ context.Context) (*eld.Features, error) {
	return &eld.Features{Seccomp: true}, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func monitorCfg(t *testing.T) eld.MonitorConfig {
	t.Helper()
	dir := t.TempDir()
	return eld.MonitorConfig{
		ContainerID: "test-ctr",
		BundlePath:  dir,
		LogPath:     filepath.Join(dir, "container.log"),
		PidFile:     filepath.Join(dir, "container.pid"),
		ExitFile:    filepath.Join(dir, "exit.code"),
		Timeout:     2 * time.Second,
	}
}

// ── Monitor tests ─────────────────────────────────────────────────────────────

func TestMonitor_Run_Foreground_Success(t *testing.T) {
	fe := &fakeEld{
		stateResults: []*eld.State{
			{ID: "test-ctr", Status: eld.StatusRunning, Pid: 12345},
			{ID: "test-ctr", Status: eld.StatusStopped, Pid: 0},
		},
	}
	m := eld.NewMonitor(fe)
	cfg := monitorCfg(t)

	result, err := m.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Pid != 12345 {
		t.Errorf("Pid = %d; want 12345", result.Pid)
	}

	// PID file should be written.
	data, readErr := os.ReadFile(cfg.PidFile)
	if readErr != nil {
		t.Fatalf("read pid file: %v", readErr)
	}
	if strings.TrimSpace(string(data)) != "12345" {
		t.Errorf("pid file = %q; want 12345", string(data))
	}
}

func TestMonitor_Run_Detached_Success(t *testing.T) {
	fe := &fakeEld{
		stateResults: []*eld.State{
			{ID: "test-ctr", Status: eld.StatusRunning, Pid: 99999},
		},
	}
	m := eld.NewMonitor(fe)
	cfg := monitorCfg(t)
	cfg.Detach = true

	result, err := m.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run detach: %v", err)
	}
	if result.Pid != 99999 {
		t.Errorf("Pid = %d; want 99999", result.Pid)
	}
}

func TestMonitor_Run_CreateError(t *testing.T) {
	fe := &fakeEld{createErr: errors.New("bundle missing")}
	m := eld.NewMonitor(fe)
	cfg := monitorCfg(t)

	_, err := m.Run(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error on Create failure")
	}
	if !strings.Contains(err.Error(), "bundle missing") {
		t.Errorf("error = %q; want it to contain 'bundle missing'", err)
	}
}

func TestMonitor_Run_StartError(t *testing.T) {
	fe := &fakeEld{startErr: errors.New("start failed")}
	m := eld.NewMonitor(fe)
	cfg := monitorCfg(t)

	_, err := m.Run(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error on Start failure")
	}
}

func TestMonitor_Run_TimeoutWaitingForPid(t *testing.T) {
	fe := &fakeEld{
		stateResults: []*eld.State{
			{ID: "test-ctr", Status: eld.StatusCreated, Pid: 0},
			{ID: "test-ctr", Status: eld.StatusCreated, Pid: 0},
		},
	}
	m := eld.NewMonitor(fe)
	cfg := monitorCfg(t)
	cfg.Timeout = 100 * time.Millisecond

	_, err := m.Run(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error = %q; want it to contain 'timed out'", err)
	}
}

func TestMonitor_Run_ContainerExitsFast(t *testing.T) {
	fe := &fakeEld{
		stateResults: []*eld.State{
			{ID: "test-ctr", Status: eld.StatusStopped, Pid: 11223},
		},
	}
	m := eld.NewMonitor(fe)
	cfg := monitorCfg(t)

	result, err := m.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Pid != 11223 {
		t.Errorf("Pid = %d; want 11223", result.Pid)
	}
}

func TestMonitor_Run_ContextCancelled(t *testing.T) {
	fe := &fakeEld{stateErr: eld.ErrContainerNotFound}
	m := eld.NewMonitor(fe)
	cfg := monitorCfg(t)
	cfg.Timeout = 5 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	_, err := m.Run(ctx, cfg)
	if err == nil {
		t.Fatal("expected context error")
	}
}

// ── StreamLogs tests ───────────────────────────────────────────────────────────

func TestStreamLogs_ValidLog(t *testing.T) {
	cfg := monitorCfg(t)
	var buf bytes.Buffer

	now := time.Now().UTC().Format(time.RFC3339Nano)
	line1 := eld.LogLine{Stream: "stdout", Log: "hello\n", Time: now}
	line2 := eld.LogLine{Stream: "stderr", Log: "world\n", Time: now}

	eld.WriteLogLine(&buf, line1)
	eld.WriteLogLine(&buf, line2)

	// Create log file.
	_ = os.WriteFile(cfg.LogPath, buf.Bytes(), 0o644)

	var out bytes.Buffer
	err := eld.StreamLogs(context.Background(), cfg.LogPath, -1, false, false, &out)
	if err != nil {
		t.Fatalf("StreamLogs: %v", err)
	}
	if !strings.Contains(out.String(), "hello\n") || !strings.Contains(out.String(), "world\n") {
		t.Errorf("out = %q; want it to contain 'hello\\n' and 'world\\n'", out.String())
	}
}

// ── helper tests ──────────────────────────────────────────────────────────────

func TestParseSignal(t *testing.T) {
	cases := []struct {
		in   string
		want syscall.Signal
	}{
		{"SIGTERM", syscall.SIGTERM},
		{"SIGKILL", syscall.SIGKILL},
		{"9", syscall.SIGKILL},
		{"SIGINT", syscall.SIGINT},
		{"SIGHUP", syscall.SIGHUP},
	}
	for _, tc := range cases {
		got, err := eld.ParseSignal(tc.in)
		if err != nil {
			t.Errorf("ParseSignal(%q) error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseSignal(%q) = %v; want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseSignal_Invalid(t *testing.T) {
	_, err := eld.ParseSignal("NOT_A_SIGNAL")
	if err == nil {
		t.Fatal("expected error for invalid signal")
	}
	if !errors.Is(err, eld.ErrInvalidSignal) {
		t.Errorf("expected ErrInvalidSignal; got: %v", err)
	}
}

func TestAtomicWriteFile_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "atomic.txt")
	data := []byte("secret")

	if err := eld.AtomicWriteFile(path, data, 0o600); err != nil {
		t.Fatalf("AtomicWriteFile: %v", err)
	}

	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, data) {
		t.Errorf("got %q; want %q", got, data)
	}
}

func TestAtomicWriteFile_Error(t *testing.T) {
	// Destination in a nonexistent directory.
	err := eld.AtomicWriteFile("/nonexistent/path/file", []byte("x"), 0o600)
	if err == nil {
		t.Fatal("expected error on invalid path")
	}
}

func TestWriteLogLine_Success(t *testing.T) {
	var buf bytes.Buffer
	line := eld.LogLine{Stream: "stdout", Log: "msg\n", Time: time.Now().Format(time.RFC3339Nano)}
	eld.WriteLogLine(&buf, line)
	if buf.Len() == 0 {
		t.Error("expected non-empty output")
	}
}

func TestWriteLogLine_Unmarshal(t *testing.T) {
	var buf bytes.Buffer
	now := time.Now().UTC().Format(time.RFC3339Nano)
	line := eld.LogLine{Stream: "stdout", Log: "data\n", Time: now}
	eld.WriteLogLine(&buf, line)

	var out eld.LogLine
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Stream != line.Stream || out.Log != line.Log || out.Time != line.Time {
		t.Errorf("got %+v; want %+v", out, line)
	}
}

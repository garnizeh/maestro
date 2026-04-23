package eld

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// ── fake Eld implementation ───────────────────────────────────────────────────

type fakeEld struct {
	createErr    error
	startErr     error
	stateResults []*State
	stateIdx     int
	stateErr     error
}

func (f *fakeEld) Create(_ context.Context, _, _ string, _ *CreateOpts) error {
	return f.createErr
}
func (f *fakeEld) Start(_ context.Context, _ string, _ *StartOpts) error {
	return f.startErr
}

func (f *fakeEld) Kill(_ context.Context, _ string, _ syscall.Signal) error {
	return nil
}

func (f *fakeEld) Delete(_ context.Context, _ string, _ *DeleteOpts) error {
	return nil
}

func (f *fakeEld) State(_ context.Context, id string) (*State, error) {
	if f.stateIdx < len(f.stateResults) {
		s := f.stateResults[f.stateIdx]
		f.stateIdx++
		return s, nil
	}
	if f.stateErr != nil {
		return nil, f.stateErr
	}
	return &State{ID: id, Status: StatusStopped}, nil
}

func (f *fakeEld) Features(_ context.Context) (*Features, error) {
	return &Features{Seccomp: true}, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func monitorCfg(t *testing.T) MonitorConfig {
	t.Helper()
	dir := t.TempDir()
	return MonitorConfig{
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
	t.Parallel()
	fe := &fakeEld{
		stateResults: []*State{
			{ID: "test-ctr", Status: StatusRunning, Pid: 12345},
			{ID: "test-ctr", Status: StatusStopped, Pid: 0},
		},
	}
	m := NewMonitor(fe)
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
	t.Parallel()
	fe := &fakeEld{}
	cmd := exec.Command("true")
	mc := &mockCommander{
		CommandFn: func(_ string, _ ...string) *exec.Cmd {
			return cmd
		},
	}

	m := NewMonitor(fe).WithCommander(mc)
	cfg := monitorCfg(t)
	cfg.Detach = true

	result, err := m.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run detach: %v", err)
	}
	if result.Pid == 0 {
		t.Fatal("Pid should not be 0")
	}
	if result.Pid != cmd.Process.Pid {
		t.Errorf("Pid = %d; want %d", result.Pid, cmd.Process.Pid)
	}
}

func TestMonitor_Run_CreateError(t *testing.T) {
	t.Parallel()
	fe := &fakeEld{createErr: errors.New("bundle missing")}
	m := NewMonitor(fe)
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
	t.Parallel()
	fe := &fakeEld{startErr: errors.New("start failed")}
	m := NewMonitor(fe)
	cfg := monitorCfg(t)

	_, err := m.Run(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error on Start failure")
	}
}

func TestMonitor_Run_TimeoutWaitingForPid(t *testing.T) {
	t.Parallel()
	fe := &fakeEld{
		stateResults: []*State{
			{ID: "test-ctr", Status: StatusCreated, Pid: 0},
			{ID: "test-ctr", Status: StatusCreated, Pid: 0},
		},
	}
	m := NewMonitor(fe)
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
	t.Parallel()
	fe := &fakeEld{
		stateResults: []*State{
			{ID: "test-ctr", Status: StatusStopped, Pid: 11223},
		},
	}
	m := NewMonitor(fe)
	cfg := monitorCfg(t)

	result, err := m.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Pid != 11223 {
		t.Errorf("Pid = %d; want 11223", result.Pid)
	}
}

func TestMonitor_Run_WaitForExitError(t *testing.T) {
	t.Parallel()
	fe := &fakeEld{
		stateResults: []*State{
			{ID: "test-ctr", Status: StatusRunning, Pid: 12345},
		},
		stateErr: errors.New("state poll failed"),
	}
	m := NewMonitor(fe)
	cfg := monitorCfg(t)
	// Force failure on second call
	fe.stateIdx = 0

	_, err := m.Run(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error from state poll failure in loop")
	}
}

func TestMonitor_Run_ContainerNotFoundDuringPoll(t *testing.T) {
	t.Parallel()
	fe := &fakeEld{
		stateResults: []*State{
			{ID: "test-ctr", Status: StatusRunning, Pid: 12345},
		},
		stateErr: ErrContainerNotFound,
	}
	m := NewMonitor(fe)
	cfg := monitorCfg(t)
	fe.stateIdx = 0

	_, err := m.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("expected no error (exit 0) when container vanishes, got %v", err)
	}
}

func TestMonitor_Run_StatusStoppedDuringPoll(t *testing.T) {
	t.Parallel()
	fe := &fakeEld{
		stateResults: []*State{
			{ID: "test-ctr", Status: StatusRunning, Pid: 12345},
			{ID: "test-ctr", Status: StatusStopped, Pid: 0},
		},
	}
	m := NewMonitor(fe)
	cfg := monitorCfg(t)
	fe.stateIdx = 0

	_, err := m.Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("expected no error when status is stopped, got %v", err)
	}
}

func TestMonitor_Run_ContextCancelled(t *testing.T) {
	t.Parallel()
	fe := &fakeEld{stateErr: ErrContainerNotFound}
	m := NewMonitor(fe)
	cfg := monitorCfg(t)
	cfg.Timeout = 5 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	_, err := m.Run(ctx, cfg)
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestMonitor_WaitForExit_ContextCancelled(t *testing.T) {
	t.Parallel()
	fe := &fakeEld{
		stateResults: []*State{
			{ID: "test-ctr", Status: StatusRunning},
		},
	}
	m := NewMonitor(fe)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := m.waitForExit(ctx, "id")
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Errorf("expected context cancelled, got %v", err)
	}
}

func TestMonitor_WaitForExit_Sleep(t *testing.T) {
	t.Parallel()
	fe := &fakeEld{
		stateResults: []*State{
			{ID: "test-ctr", Status: StatusRunning},
			{ID: "test-ctr", Status: StatusStopped},
		},
	}
	m := NewMonitor(fe)
	_, err := m.waitForExit(context.Background(), "ctr1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestMonitor_Run_MkdirError(t *testing.T) {
	t.Parallel()
	fs := &mockFS{MkdirAllFn: func(string, os.FileMode) error { return errors.New("mkdir fail") }}
	m := NewMonitor(&fakeEld{}).WithFS(fs)
	cfg := monitorCfg(t)
	_, err := m.Run(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "mkdir fail") {
		t.Errorf("expected mkdir error, got %v", err)
	}
}

func TestMonitor_Run_LogOpenError(t *testing.T) {
	t.Parallel()
	fs := &mockFS{
		OpenFileFn: func(string, int, os.FileMode) (*os.File, error) { return nil, errors.New("open fail") },
	}
	m := NewMonitor(&fakeEld{}).WithFS(fs)
	cfg := monitorCfg(t)
	_, err := m.Run(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "open log file") {
		t.Errorf("expected log open error, got %v", err)
	}
}

func TestMonitor_Run_AtomicWritePidError(t *testing.T) {
	t.Parallel()
	fs := &mockFS{
		CreateTempFn: func(string, string) (*os.File, error) { return nil, errors.New("write pid file") },
	}
	fe := &fakeEld{
		stateResults: []*State{
			{ID: "test-ctr", Status: StatusRunning, Pid: 12345},
		},
	}
	m := NewMonitor(fe).WithFS(fs)
	cfg := monitorCfg(t)
	cfg.PidFile = "/some/pid/file"
	_, err := m.Run(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "write pid file") {
		t.Errorf("expected atomic write error, got %v", err)
	}
}

func TestMonitor_WaitForPid_ImmediateError(t *testing.T) {
	t.Parallel()
	fe := &fakeEld{stateErr: errors.New("immediate state fail")}
	m := NewMonitor(fe)
	_, err := m.waitForPid(context.Background(), "ctr1", 0)
	if err == nil || !strings.Contains(err.Error(), "immediate state fail") {
		t.Errorf("expected immediate error, got %v", err)
	}
}

// ── StreamLogs tests ───────────────────────────────────────────────────────────

func TestStreamLogs_ValidLog(t *testing.T) {
	t.Parallel()
	cfg := monitorCfg(t)
	var buf bytes.Buffer

	now := time.Now().UTC().Format(time.RFC3339Nano)
	line1 := logLine{Stream: "stdout", Log: "hello\n", Time: now}
	line2 := logLine{Stream: "stderr", Log: "world\n", Time: now}

	writeLogLine(&buf, line1)
	writeLogLine(&buf, line2)

	if err := os.WriteFile(cfg.LogPath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("fail to write log file: %v", err)
	}

	var out bytes.Buffer
	err := StreamLogs(context.Background(), cfg.LogPath, -1, false, false, &out)
	if err != nil {
		t.Fatalf("StreamLogs: %v", err)
	}
	if !strings.Contains(out.String(), "hello\n") || !strings.Contains(out.String(), "world\n") {
		t.Errorf("out = %q; want it to contain 'hello\\n' and 'world\\n'", out.String())
	}
}

func TestStreamLogs_Follow(t *testing.T) {
	t.Parallel()
	cfg := monitorCfg(t)
	if err := os.WriteFile(cfg.LogPath, []byte(""), 0644); err != nil {
		t.Fatalf("fail to write log file: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var out bytes.Buffer

	go func() {
		time.Sleep(200 * time.Millisecond)
		line := logLine{Stream: "stdout", Log: "late\n", Time: "now"}
		f, openErr := os.OpenFile(cfg.LogPath, os.O_APPEND|os.O_WRONLY, 0644)
		if openErr != nil {
			t.Logf("fail to open file: %v", openErr)
		}
		writeLogLine(f, line)
		if closeErr := f.Close(); closeErr != nil {
			t.Logf("fail to close file: %v", closeErr)
		}
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	err := StreamLogs(ctx, cfg.LogPath, -1, true, false, &out)
	if err != nil {
		t.Fatalf("StreamLogs follow: %v", err)
	}
	if !strings.Contains(out.String(), "late\n") {
		t.Errorf("out = %q; want it to contain 'late\\n'", out.String())
	}
}

func TestStreamLogs_Errors(t *testing.T) {
	t.Parallel()
	cfg := monitorCfg(t)
	if err := StreamLogs(context.Background(), "/nonexistent", -1, false, false, io.Discard); err != nil {
		t.Errorf("expected no error for nonexistent file, got %v", err)
	}

	if err := os.WriteFile(cfg.LogPath, []byte("{invalid\n"), 0644); err != nil {
		t.Fatalf("fail to write log file: %v", err)
	}
	var out bytes.Buffer
	if err := StreamLogs(context.Background(), cfg.LogPath, -1, false, false, &out); err != nil {
		t.Fatalf("expected no error (skips bad JSON), got %v", err)
	}
	if out.Len() != 0 {
		t.Error("expected empty output for bad JSON")
	}

	line := logLine{Stream: "stdout", Log: "msg\n", Time: "2024-01-01"}
	if err := os.WriteFile(cfg.LogPath, nil, 0644); err != nil {
		t.Fatalf("fail to write log file: %v", err)
	}
	f, openErr := os.OpenFile(cfg.LogPath, os.O_WRONLY, 0644)
	if openErr != nil {
		t.Fatalf("fail to open file: %v", openErr)
	}
	writeLogLine(f, line)
	if err := f.Close(); err != nil {
		t.Fatalf("fail to close file: %v", err)
	}
	out.Reset()
	err := StreamLogs(context.Background(), cfg.LogPath, -1, false, true, &out)
	if err != nil {
		t.Fatalf("StreamLogs follow: %v", err)
	}
	if !strings.Contains(out.String(), "2024-01-01 msg\n") {
		t.Errorf("expected timestamps in output, got %q", out.String())
	}
}

func TestStreamLogs_MoreErrors(t *testing.T) {
	t.Parallel()
	cfg := monitorCfg(t)

	t.Run("OpenFail", func(it *testing.T) {
		it.Parallel()
		fs := &mockFS{
			OpenFn: func(string) (*os.File, error) { return nil, errors.New("open fail") },
		}
		s := NewLogStreamer(fs)
		err := s.StreamLogs(context.Background(), cfg.LogPath, -1, false, false, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "open fail") {
			it.Errorf("expected open error, got %v", err)
		}
	})

	t.Run("SeekFail", func(it *testing.T) {
		it.Parallel()
		r, w, err := os.Pipe()
		if err != nil {
			it.Fatalf("fail to create pipe: %v", err)
		}
		if closeErr := w.Close(); closeErr != nil {
			it.Fatalf("fail to close pipe: %v", closeErr)
		}
		fs := &mockFS{OpenFn: func(string) (*os.File, error) { return r, nil }}
		s := NewLogStreamer(fs)
		errStream := s.StreamLogs(context.Background(), "any", -1, true, false, &bytes.Buffer{})
		if errStream == nil || !strings.Contains(errStream.Error(), "seek log") {
			it.Errorf("expected seek error, got %v", errStream)
		}
	})

	t.Run("ScanFail", func(it *testing.T) {
		it.Parallel()
		dir := it.TempDir()
		f, createErr := os.Create(filepath.Join(dir, "fail"))
		if createErr != nil {
			it.Fatalf("fail to create file: %v", createErr)
		}
		if closeErr := f.Close(); closeErr != nil {
			it.Fatalf("fail to close file: %v", closeErr)
		}
		fs := &mockFS{OpenFn: func(string) (*os.File, error) { return f, nil }}
		s := NewLogStreamer(fs)
		err := s.StreamLogs(context.Background(), "any", -1, false, false, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "scan log") {
			it.Errorf("expected scan error, got %v", err)
		}
	})
}

func TestStreamLogs_TailEdges(t *testing.T) {
	t.Parallel()
	cfg := monitorCfg(t)
	lines := []logLine{
		{Log: "1\n"}, {Log: "2\n"}, {Log: "3\n"},
	}
	f, err := os.Create(cfg.LogPath)
	if err != nil {
		t.Fatalf("fail to create file: %v", err)
	}
	for _, l := range lines {
		writeLogLine(f, l)
	}
	if closeErr := f.Close(); closeErr != nil {
		t.Fatalf("fail to close file: %v", closeErr)
	}

	var out bytes.Buffer
	err = StreamLogs(context.Background(), cfg.LogPath, 1, false, false, &out)
	if err != nil {
		t.Fatalf("StreamLogs follow: %v", err)
	}
	if out.String() != "3\n" {
		t.Errorf("tail 1 got %q", out.String())
	}

	out.Reset()
	err = StreamLogs(context.Background(), cfg.LogPath, 5, false, false, &out)
	if err != nil {
		t.Fatalf("StreamLogs follow: %v", err)
	}
	if !strings.Contains(out.String(), "1\n2\n3\n") {
		t.Errorf("tail 5 got %q", out.String())
	}
}

// ── helper tests ──────────────────────────────────────────────────────────────

func TestParseSignal(t *testing.T) {
	t.Parallel()
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
		got, err := ParseSignal(tc.in)
		if err != nil {
			t.Errorf("ParseSignal(%q) error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseSignal(%q) = %v; want %v", tc.in, got, tc.want)
		}
	}

	extra := []string{
		"USR1",
		"USR2",
		"QUIT",
		"HUP",
		"INT",
		"TERM",
		"KILL",
		"1",
		"2",
		"3",
		"9",
		"10",
		"12",
		"15",
	}
	for _, s := range extra {
		if _, err := ParseSignal(s); err != nil {
			t.Errorf("ParseSignal(%q) failed: %v", s, err)
		}
	}
}

func TestParseSignal_Invalid(t *testing.T) {
	t.Parallel()
	_, err := ParseSignal("NOT_A_SIGNAL")
	if err == nil {
		t.Fatal("expected error for invalid signal")
	}
	if !errors.Is(err, ErrInvalidSignal) {
		t.Errorf("expected ErrInvalidSignal; got: %v", err)
	}
}

func (m *Monitor) Test_atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	return m.atomicWriteFile(path, data, perm)
}

func TestAtomicWriteFile_Success(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "atomic.txt")
	data := []byte("secret")

	m := NewMonitor(&fakeEld{})
	if err := m.Test_atomicWriteFile(path, data, 0o600); err != nil {
		t.Fatalf("AtomicWriteFile: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("fail to read file: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("got %q; want %q", got, data)
	}
}

func TestMonitor_AtomicWriteFile_Errors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	t.Run("CreateTempFail", func(it *testing.T) {
		it.Parallel()
		fs := &mockFS{
			CreateTempFn: func(string, string) (*os.File, error) { return nil, errors.New("temp fail") },
		}
		m := NewMonitor(&fakeEld{}).WithFS(fs)
		err := m.Test_atomicWriteFile(filepath.Join(dir, "file"), []byte("data"), 0644)
		if err == nil || !strings.Contains(err.Error(), "create temp") {
			it.Errorf("expected temp create error, got %v", err)
		}
	})

	t.Run("WriteFail", func(it *testing.T) {
		it.Parallel()
		// Create a real temp file but close it immediately to cause Write to fail.
		f, createErr := os.CreateTemp(dir, "write-fail")
		if createErr != nil {
			it.Fatalf("fail to create temp file: %v", createErr)
		}
		if closeErr := f.Close(); closeErr != nil {
			it.Fatalf("fail to close temp file: %v", closeErr)
		}
		fs := &mockFS{CreateTempFn: func(string, string) (*os.File, error) { return f, nil }}
		m := NewMonitor(&fakeEld{}).WithFS(fs)
		err := m.Test_atomicWriteFile(filepath.Join(dir, "file1"), []byte("data"), 0644)
		if err == nil || !strings.Contains(err.Error(), "write temp") {
			it.Errorf("expected write error, got %v", err)
		}
	})

	t.Run("ChmodFail", func(it *testing.T) {
		it.Parallel()
		// Use a real file for Write success, but mock Chmod error.
		f, createErr := os.CreateTemp(dir, "chmod-fail")
		if createErr != nil {
			it.Fatalf("fail to create temp file: %v", createErr)
		}
		fs := &mockFS{
			CreateTempFn: func(string, string) (*os.File, error) { return f, nil },
			ChmodFn:      func(string, os.FileMode) error { return errors.New("chmod fail") },
		}
		m := NewMonitor(&fakeEld{}).WithFS(fs)
		err := m.Test_atomicWriteFile(filepath.Join(dir, "file"), []byte("data"), 0644)
		if err == nil || !strings.Contains(err.Error(), "chmod fail") {
			it.Errorf("expected chmod error, got %v", err)
		}
	})

	t.Run("RenameFail", func(it *testing.T) {
		it.Parallel()
		// Use a real file, mock Rename error.
		f, createErr := os.CreateTemp(dir, "rename-fail")
		if createErr != nil {
			it.Fatalf("fail to create temp file: %v", createErr)
		}
		fs := &mockFS{
			CreateTempFn: func(string, string) (*os.File, error) { return f, nil },
			RenameFn:     func(string, string) error { return errors.New("rename fail") },
		}
		m := NewMonitor(&fakeEld{}).WithFS(fs)
		err := m.Test_atomicWriteFile(filepath.Join(dir, "file"), []byte("data"), 0644)
		if err == nil || !strings.Contains(err.Error(), "rename fail") {
			it.Errorf("expected rename error, got %v", err)
		}
	})
}

func TestWriteLogLine_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	line := logLine{Stream: "stdout", Log: "msg\n", Time: time.Now().Format(time.RFC3339Nano)}
	writeLogLine(&buf, line)
	if buf.Len() == 0 {
		t.Error("expected non-empty output")
	}
}

func TestWriteLogLine_Unmarshal(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	now := time.Now().UTC().Format(time.RFC3339Nano)
	line := logLine{Stream: "stdout", Log: "data\n", Time: now}
	writeLogLine(&buf, line)

	var out logLine
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Stream != line.Stream || out.Log != line.Log || out.Time != line.Time {
		t.Errorf("got %+v; want %+v", out, line)
	}
}

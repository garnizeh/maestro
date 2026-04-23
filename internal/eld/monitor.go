package eld

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	stdioCount        = 2
	copierWaitTimeout = 5 * time.Second
)

// MonitorResult carries the outcome of a monitored container run.
type MonitorResult struct {
	// Pid is the container's init process PID while running.
	Pid int
	// ExitCode is the container process exit code (valid after Wait returns).
	ExitCode int
}

// Monitor supervises the container process and manages its lifecycle.
type Monitor struct {
	runtime   Eld
	commander Commander
	fs        FS
}

// NewMonitor returns a new [Monitor] for the given runtime.
func NewMonitor(runtime Eld) *Monitor {
	return &Monitor{
		runtime:   runtime,
		commander: RealCommander{},
		fs:        RealFS{},
	}
}

// WithFS sets a custom filesystem implementation for the monitor.
func (m *Monitor) WithFS(fs FS) *Monitor {
	m.fs = fs
	return m
}

// WithCommander sets a custom commander implementation for the monitor.
func (m *Monitor) WithCommander(c Commander) *Monitor {
	m.commander = c
	return m
}

// Run creates and starts the container described by cfg, then supervises it.
func (m *Monitor) Run(ctx context.Context, cfg MonitorConfig) (*MonitorResult, error) {
	log.Debug().Str("id", cfg.ContainerID).Bool("detach", cfg.Detach).
		Msg("monitor: starting supervision")

	// ── Background Handling (Detach) ──────────────────────────────────────────
	if cfg.Detach && os.Getenv("MAESTRO_MONITOR_ID") == "" {
		return m.startBackground(cfg)
	}

	// ── Setup Log Redirection ─────────────────────────────────────────────────
	logFile, err := m.setupLogFile(cfg.LogPath)
	if err != nil {
		return nil, err
	}
	defer logFile.Close()

	// Create pipes for stdout/stderr capture.
	outR, outW, errR, errW, err := m.createStdioPipes()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = outR.Close()
		_ = errR.Close()
	}()

	// ── Stdio Capture Loop ────────────────────────────────────────────────────
	done := make(chan struct{}, stdioCount)
	m.startStdioCopiers(outR, errR, logFile, cfg, done)

	// ── Execute and Supervise ─────────────────────────────────────────────────
	return m.executeAndSupervise(ctx, cfg, outW, errW, done)
}

func (m *Monitor) createStdioPipes() (*os.File, *os.File, *os.File, *os.File, error) {
	outR, outW, err := os.Pipe()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("monitor: pipe stdout: %w", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		_ = outR.Close()
		_ = outW.Close()
		return nil, nil, nil, nil, fmt.Errorf("monitor: pipe stderr: %w", err)
	}
	return outR, outW, errR, errW, nil
}

func (m *Monitor) executeAndSupervise(
	ctx context.Context,
	cfg MonitorConfig,
	outW, errW *os.File,
	done chan struct{},
) (*MonitorResult, error) {
	// ── Create and start container ────────────────────────────────────────────
	createOpts := &CreateOpts{
		Stdout:       outW,
		Stderr:       errW,
		LauncherPath: cfg.LauncherPath,
	}
	createErr := m.runtime.Create(ctx, cfg.ContainerID, cfg.BundlePath, createOpts)
	// We MUST close the write ends so readers get EOF when the container exits.
	_ = outW.Close()
	_ = errW.Close()

	if createErr != nil {
		return nil, fmt.Errorf("monitor: eld create: %w", createErr)
	}

	startOpts := &StartOpts{
		LauncherPath: cfg.LauncherPath,
	}
	if startErr := m.runtime.Start(ctx, cfg.ContainerID, startOpts); startErr != nil {
		return nil, fmt.Errorf("monitor: eld start: %w", startErr)
	}

	// ── Poll for PID ──────────────────────────────────────────────────────────
	pid, err := m.waitForPid(ctx, cfg.ContainerID, cfg.Timeout)
	if err != nil {
		return nil, fmt.Errorf("monitor: wait for pid: %w", err)
	}

	if cfg.PidFile != "" {
		pidData := strconv.Itoa(pid) + "\n"
		if wErr := m.atomicWriteFile(cfg.PidFile, []byte(pidData), filePerm); wErr != nil {
			return nil, fmt.Errorf("monitor: write pid file: %w", wErr)
		}
	}

	// ── Wait for Exit ─────────────────────────────────────────────────────────
	exitCode, err := m.waitForExit(ctx, cfg.ContainerID)
	if err != nil {
		return nil, fmt.Errorf("monitor: wait for exit: %w", err)
	}

	// ── Wait for Goroutines ───────────────────────────────────────────────────
	log.Debug().Msg("monitor: waiting for copier goroutines")
	for i := range stdioCount {
		select {
		case <-done:
			log.Debug().Int("i", i).Msg("monitor: goroutine finished")
		case <-time.After(copierWaitTimeout):
			log.Warn().Int("i", i).Msg("monitor: timed out waiting for goroutine")
		}
	}

	if cfg.ExitFile != "" {
		exitData := strconv.Itoa(exitCode) + "\n"
		if wErr := m.atomicWriteFile(cfg.ExitFile, []byte(exitData), filePerm); wErr != nil {
			log.Warn().
				Err(wErr).
				Str("exitFile", cfg.ExitFile).
				Msg("monitor: failed to write exit file")
		}
	}

	return &MonitorResult{Pid: pid, ExitCode: exitCode}, nil
}

// startBackground re-executes the current process as a detached monitor.
func (m *Monitor) startBackground(cfg MonitorConfig) (*MonitorResult, error) {
	self, executableErr := os.Executable()
	if executableErr != nil {
		return nil, executableErr
	}

	// Execute the current process as a detached monitor.
	monitorArgs := []string{"system", "monitor",
		"--id", cfg.ContainerID,
		"--bundle", cfg.BundlePath,
		"--log", cfg.LogPath,
		"--pid-file", cfg.PidFile,
		"--exit-file", cfg.ExitFile,
	}
	if cfg.LauncherPath != "" {
		monitorArgs = append(monitorArgs, "--launcher", cfg.LauncherPath)
	}
	cmd := m.commander.Command(self, monitorArgs...)
	cmd.Env = append(os.Environ(), "MAESTRO_MONITOR_ID="+cfg.ContainerID)

	// Detach from the terminal.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	log.Debug().Str("id", cfg.ContainerID).Strs("args", monitorArgs).
		Msg("monitor: re-executing in background")

	if startErr := cmd.Start(); startErr != nil {
		return nil, fmt.Errorf("monitor: start background: %w", startErr)
	}

	// Return the PID of the background monitor.
	return &MonitorResult{Pid: cmd.Process.Pid}, nil
}

func (m *Monitor) copyToLog(r io.Reader, w io.Writer, console io.Writer, stream string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		entry := logLine{
			Stream: stream,
			Time:   time.Now().UTC().Format(time.RFC3339Nano),
			Log:    line + "\n",
		}
		writeLogLine(w, entry)

		if console != nil {
			fmt.Fprintln(console, line)
		}
	}
}

// waitForPid polls the OCI runtime state until the container reaches the
// "running" status and its PID is non-zero.
func (m *Monitor) waitForPid(ctx context.Context, id string, timeout time.Duration) (int, error) {
	if timeout == 0 {
		timeout = 10 * time.Second //nolint:mnd // default 10s PID wait timeout
	}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		state, err := m.runtime.State(ctx, id)
		if err != nil {
			if errors.Is(err, ErrContainerNotFound) {
				time.Sleep(50 * time.Millisecond) //nolint:mnd // standard polling interval
				continue
			}
			return 0, err
		}

		if state.Status == StatusRunning && state.Pid > 0 {
			return state.Pid, nil
		}
		if state.Status == StatusStopped {
			// Container exited very fast (e.g. `echo hello`).
			return state.Pid, nil
		}

		time.Sleep(50 * time.Millisecond) //nolint:mnd // standard polling interval
	}
	return 0, fmt.Errorf("timed out waiting for container %s to start", id)
}

// waitForExit polls the OCI runtime state until the container stops,
// and returns the exit code.
func (m *Monitor) waitForExit(
	ctx context.Context,
	id string,
) (int, error) { //nolint:unparam // exit code tracking WIP
	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		state, err := m.runtime.State(ctx, id)
		if err != nil {
			if errors.Is(err, ErrContainerNotFound) {
				// Container already cleaned up — treat as exit 0.
				return 0, nil
			}
			return 0, err
		}

		if state.Status == StatusStopped {
			return 0, nil
		}

		time.Sleep(100 * time.Millisecond) //nolint:mnd // standard polling interval
	}
}

// logLine is the json-file log format (compatible with Docker/Podman log drivers).
type logLine struct {
	Stream string `json:"stream"`
	Time   string `json:"time"`
	Log    string `json:"log"`
}

// writeLogLine serialises l to logFile as a single JSON line.
func writeLogLine(w io.Writer, l logLine) {
	data, errMarshal := json.Marshal(l)
	if errMarshal != nil {
		return // very unlikely for this struct
	}
	if _, errWrite := w.Write(append(data, '\n')); errWrite != nil {
		return // failing to write a log line is the terminal destiny of this caller
	}
}

// StreamLogs reads the container log file at logPath and writes each log
// entry to w. If follow is true, it remains open and streams new entries.
func StreamLogs(
	ctx context.Context,
	logPath string,
	tail int,
	follow bool,
	timestamps bool,
	w io.Writer,
) error {
	return DefaultStreamLogs(ctx, logPath, tail, follow, timestamps, w)
}

// DefaultStreamLogs is the package-level StreamLogs using RealFS.
func DefaultStreamLogs(
	ctx context.Context,
	logPath string,
	tail int,
	follow bool,
	timestamps bool,
	w io.Writer,
) error {
	return NewLogStreamer(RealFS{}).StreamLogs(ctx, logPath, tail, follow, timestamps, w)
}

// LogStreamer handles log file streaming with an injected FS.
type LogStreamer struct {
	fs FS
}

// NewLogStreamer returns a [LogStreamer] with the given [FS].
func NewLogStreamer(fs FS) *LogStreamer {
	return &LogStreamer{fs: fs}
}

// StreamLogs reads the container log file at logPath and writes each log
// entry to w.
func (s *LogStreamer) StreamLogs( //nolint:gocognit // log streaming loop
	ctx context.Context,
	logPath string,
	tail int,
	follow bool,
	timestamps bool,
	w io.Writer,
) error {
	f, err := s.fs.Open(logPath)
	if err != nil {
		if s.fs.IsNotExist(err) {
			// No logs yet — that's fine.
			return nil
		}
		return fmt.Errorf("open log file: %w", err)
	}
	defer f.Close()

	// Collect all lines for tail support.
	var lines []logLine
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var l logLine
		if jsonErr := json.Unmarshal(scanner.Bytes(), &l); jsonErr != nil {
			continue
		}
		lines = append(lines, l)
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return fmt.Errorf("scan log: %w", scanErr)
	}

	// Apply tail.
	if tail >= 0 && tail < len(lines) {
		lines = lines[len(lines)-tail:]
	}

	// Write selected lines.
	for _, l := range lines {
		printLogLine(w, l, timestamps)
	}

	if !follow {
		return nil
	}

	// Follow mode: seek to end and watch for new lines.
	if _, seekErr := f.Seek(0, io.SeekEnd); seekErr != nil {
		return fmt.Errorf("seek log: %w", seekErr)
	}

	scanner = bufio.NewScanner(f)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		for scanner.Scan() {
			var l logLine
			if jsonErr := json.Unmarshal(scanner.Bytes(), &l); jsonErr != nil {
				continue
			}
			printLogLine(w, l, timestamps)
		}
		if scanErr := scanner.Err(); scanErr != nil {
			return fmt.Errorf("tail log: %w", scanErr)
		}
		// Reset scanner for next iteration.
		scanner = bufio.NewScanner(f)
		time.Sleep(100 * time.Millisecond) //nolint:mnd // standard polling interval
	}
}

// printLogLine writes a single log line to w.
func printLogLine(w io.Writer, l logLine, timestamps bool) {
	if timestamps {
		if _, err := fmt.Fprintf(w, "%s %s", l.Time, l.Log); err != nil {
			log.Debug().Err(err).Msg("monitor: failed to write log line")
		}
		return
	}

	if _, err := fmt.Fprint(w, l.Log); err != nil {
		log.Debug().Err(err).Msg("monitor: failed to write log line")
	}
}

// atomicWriteFile writes data to path atomically using write-to-temp + rename.
func (m *Monitor) atomicWriteFile(
	path string,
	data []byte,
	perm os.FileMode,
) error {
	dir := filepath.Dir(path)
	tmp, err := m.fs.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	if _, writeErr := tmp.Write(data); writeErr != nil {
		if errClose := tmp.Close(); errClose != nil {
			log.Debug().Err(errClose).Msg("monitor: failed to close temp file after write error")
		}
		if errRem := m.fs.Remove(tmpName); errRem != nil {
			log.Debug().Err(errRem).Msg("monitor: failed to remove temp file after write error")
		}
		return fmt.Errorf("write temp: %w", writeErr)
	}
	if closeErr := tmp.Close(); closeErr != nil {
		if errRem := m.fs.Remove(tmpName); errRem != nil {
			log.Debug().Err(errRem).Msg("monitor: failed to remove temp file after close error")
		}
		return fmt.Errorf("close temp: %w", closeErr)
	}
	if chmodErr := m.fs.Chmod(tmpName, perm); chmodErr != nil {
		if errRem := m.fs.Remove(tmpName); errRem != nil {
			log.Debug().Err(errRem).Msg("monitor: failed to remove temp file after chmod error")
		}
		return fmt.Errorf("chmod temp: %w", chmodErr)
	}
	if renameErr := m.fs.Rename(tmpName, path); renameErr != nil {
		if errRem := m.fs.Remove(tmpName); errRem != nil {
			log.Debug().Err(errRem).Msg("monitor: failed to remove temp file after rename error")
		}
		return fmt.Errorf("rename: %w", renameErr)
	}
	return nil
}

// ParseSignal converts a string (number or name) to a [syscall.Signal].
func ParseSignal(s string) (syscall.Signal, error) {
	// Try numeric first.
	if n, err := strconv.Atoi(s); err == nil {
		return syscall.Signal(n), nil
	}

	// Try names.
	switch strings.ToUpper(s) {
	case "SIGKILL", "KILL", "9":
		return syscall.SIGKILL, nil
	case "SIGTERM", "TERM", "15":
		return syscall.SIGTERM, nil
	case "SIGINT", "INT", "2":
		return syscall.SIGINT, nil
	case "SIGQUIT", "QUIT", "3":
		return syscall.SIGQUIT, nil
	case "SIGHUP", "HUP", "1":
		return syscall.SIGHUP, nil
	case "SIGUSR1", "USR1", "10":
		return syscall.SIGUSR1, nil
	case "SIGUSR2", "USR2", "12":
		return syscall.SIGUSR2, nil
	default:
		return 0, fmt.Errorf("%w: %s", ErrInvalidSignal, s)
	}
}
func (m *Monitor) setupLogFile(path string) (*os.File, error) {
	if mkdirErr := m.fs.MkdirAll(filepath.Dir(path), dirPerm); mkdirErr != nil {
		return nil, fmt.Errorf("monitor: create log dir: %w", mkdirErr)
	}

	logFile, err := m.fs.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, filePerm)
	if err != nil {
		return nil, fmt.Errorf("monitor: open log file: %w", err)
	}
	return logFile, nil
}

func (m *Monitor) startStdioCopiers(outR, errR io.ReadCloser, logFile io.Writer,
	cfg MonitorConfig, done chan struct{}) {
	go func() {
		defer func() {
			log.Debug().Msg("monitor: stdout copier exited")
			done <- struct{}{}
		}()
		m.copyToLog(outR, logFile, cfg.Stdout, "stdout")
	}()
	go func() {
		defer func() {
			log.Debug().Msg("monitor: stderr copier exited")
			done <- struct{}{}
		}()
		m.copyToLog(errR, logFile, cfg.Stderr, "stderr")
	}()
}

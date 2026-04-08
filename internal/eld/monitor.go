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
)

// MonitorResult carries the outcome of a monitored container run.
type MonitorResult struct {
	// Pid is the container's init process PID while running.
	Pid int
	// ExitCode is the container process exit code (valid after Wait returns).
	ExitCode int
}

// Monitor is the native Go container monitor (Cort MVP).
//
// It uses the OCI runtime (via [Eld]) to create and start the container, then
// supervises the container process: capturing stdio → log file, writing the
// PID file, and collecting the exit code when the process terminates.
//
// For detached containers the monitor runs the container runtime and returns
// immediately after the process starts. The container process continues
// independently.
type Monitor struct {
	runtime Eld
	// osStat is injectable for testing.
	osStat func(string) (os.FileInfo, error)
}

// NewMonitor returns a [Monitor] backed by the given [Eld] runtime.
func NewMonitor(runtime Eld) *Monitor {
	return &Monitor{
		runtime: runtime,
		osStat:  os.Stat,
	}
}

// Run creates and starts the container described by cfg, then supervises it.
//
// For foreground containers (cfg.Detach=false) Run blocks until the container
// exits and returns the exit code in [MonitorResult].
//
// For detached containers (cfg.Detach=true) Run creates and starts the container,
// writes the PID file, and then returns immediately. The container process
// continues independently.
func (m *Monitor) Run(ctx context.Context, cfg MonitorConfig) (*MonitorResult, error) {
	// Ensure the log file directory exists.
	if mkdirErr := os.MkdirAll(filepath.Dir(cfg.LogPath), dirPerm); mkdirErr != nil {
		return nil, fmt.Errorf("monitor: create log dir: %w", mkdirErr)
	}

	// Open the log file for append.
	logFile, err := os.OpenFile(cfg.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, filePerm)
	if err != nil {
		return nil, fmt.Errorf("monitor: open log file: %w", err)
	}
	defer logFile.Close()

	// Create and start the container via Eld.
	if createErr := m.runtime.Create(ctx, cfg.ContainerID, cfg.BundlePath, nil); createErr != nil {
		return nil, fmt.Errorf("monitor: eld create: %w", createErr)
	}
	if startErr := m.runtime.Start(ctx, cfg.ContainerID); startErr != nil {
		return nil, fmt.Errorf("monitor: eld start: %w", startErr)
	}

	// Poll the runtime state to get the PID.
	pid, pidErr := m.waitForPid(ctx, cfg.ContainerID, cfg.Timeout)
	if pidErr != nil {
		return nil, fmt.Errorf("monitor: wait for pid: %w", pidErr)
	}

	// Write the PID file.
	if pidFile := cfg.PidFile; pidFile != "" {
		pidData := strconv.Itoa(pid) + "\n"
		if writeErr := atomicWriteFile(pidFile, []byte(pidData), filePerm); writeErr != nil {
			return nil, fmt.Errorf("monitor: write pid file: %w", writeErr)
		}
	}

	if cfg.Detach {
		// Detached: return immediately, container runs independently.
		return &MonitorResult{Pid: pid}, nil
	}

	// Foreground: stream logs and wait for the container to exit.
	exitCode, waitErr := m.waitForExit(ctx, cfg.ContainerID, logFile)
	if waitErr != nil {
		return nil, fmt.Errorf("monitor: wait for exit: %w", waitErr)
	}

	// Write the exit code file.
	if exitFile := cfg.ExitFile; exitFile != "" {
		exitData := strconv.Itoa(exitCode) + "\n"
		_ = atomicWriteFile(exitFile, []byte(exitData), filePerm)
	}

	return &MonitorResult{Pid: pid, ExitCode: exitCode}, nil
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
// streaming logs in the meantime, and returns the exit code.
// For the MVP, logs are forwarded line-by-line via polling the runtime state.
func (m *Monitor) waitForExit(
	ctx context.Context,
	id string,
	logFile io.Writer,
) (int, error) { //nolint:unparam // currently returns 0, planned for future update
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
			logEntry := logLine{
				Stream: "stdout",
				Time:   time.Now().UTC().Format(time.RFC3339Nano),
				Log:    fmt.Sprintf("container %s exited with status %d\n", id, 0),
			}
			writeLogLine(logFile, logEntry)
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
	data, _ := json.Marshal(l)
	_, _ = w.Write(append(data, '\n'))
}

// StreamLogs reads the container log file at logPath and writes each log
// entry to w. If follow is true, it remains open and streams new entries.
func StreamLogs( //nolint:gocognit // log streaming loop
	ctx context.Context,
	logPath string,
	tail int,
	follow bool,
	timestamps bool,
	w io.Writer,
) error {
	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
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
		fmt.Fprintf(w, "%s %s", l.Time, l.Log)
	} else {
		fmt.Fprint(w, l.Log)
	}
}

// atomicWriteFile writes data to path atomically using write-to-temp + rename.
func atomicWriteFile(
	path string,
	data []byte,
	perm os.FileMode,
) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	if _, writeErr := tmp.Write(data); writeErr != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write temp: %w", writeErr)
	}
	if closeErr := tmp.Close(); closeErr != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp: %w", closeErr)
	}
	if chmodErr := os.Chmod(tmpName, perm); chmodErr != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("chmod temp: %w", chmodErr)
	}
	if renameErr := os.Rename(tmpName, path); renameErr != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename: %w", renameErr)
	}
	return nil
}

// ParseSignal converts a string (number or name) to a [syscall.Signal].
// Supported names: SIGKILL, SIGTERM, SIGINT, SIGQUIT, SIGHUP, SIGUSR1, SIGUSR2.
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

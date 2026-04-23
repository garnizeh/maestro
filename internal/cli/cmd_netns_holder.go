package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"context"

	"github.com/rs/zerolog/log"

	"github.com/spf13/cobra"

	"github.com/rodrigo-baliza/maestro/internal/beam"
	"github.com/rodrigo-baliza/maestro/internal/bin"
)

// newNetNSHolderCmd creates the hidden _netns_holder command.
// This command holds namespaces alive and executes OCI commands (Launcher Holder).
func newNetNSHolderCmd(_ *Handler) *cobra.Command {
	var sockPath string

	cmd := &cobra.Command{
		Use:    "_netns_holder",
		Short:  "Internal: Launcher Holder for rootless namespaces",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if sockPath == "" {
				return errors.New("socket path is required")
			}

			// 1. Setup the control socket
			var lc net.ListenConfig
			l, err := lc.Listen(cmd.Context(), "unix", sockPath)
			if err != nil {
				return fmt.Errorf("failed to listen on %s: %w", sockPath, err)
			}
			defer l.Close()
			defer os.Remove(sockPath)

			log.Debug().Str("socket", sockPath).Msg("netns_holder: control socket active")

			// Handle termination signals
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

			// protocol state

			// Close the listener on signal reception to break Accept()
			go func() {
				select {
				case sig := <-sigCh:
					log.Debug().Stringer("signal", sig).Msg("netns_holder: signal received")
				case <-cmd.Context().Done():
					log.Debug().Msg("netns_holder: context cancelled")
				}
				if closeErr := l.Close(); closeErr != nil {
					log.Error().Err(closeErr).Msg("netns_holder: failed to close listener")
				}
			}()

			// 2. Accept loop
			runNetNSHolderLoop(l)
			return nil
		},
	}

	cmd.Flags().StringVar(&sockPath, "socket", "", "Unix socket for control")

	return cmd
}

func runNetNSHolderLoop(l net.Listener) {
	mounted := false

	for {
		conn, err := l.Accept()
		if err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				log.Debug().Msg("netns_holder: terminating on closed listener")
				return
			}
			log.Warn().Err(err).Msg("netns_holder: accept error")
			continue
		}

		if !mounted {
			cmd := handleInitialMount(conn)
			if cmd != nil {
				log.Debug().
					Int("pid", cmd.Process.Pid).
					Msg("netns_holder: started foreground FUSE mount helper")
			}
			mounted = true
			continue
		}

		handleExecConnection(conn)
	}
}

func handleInitialMount(conn net.Conn) *exec.Cmd {
	defer conn.Close()
	var req beam.MountRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		log.Error().Err(err).Msg("netns_holder: failed to decode mount request")
		return nil
	}

	log.Debug().Interface("req", req).Msg("netns_holder: performing initial mount")
	if req.Source != "" && req.Target != "" {
		mountCmd, err := performRootlessMount(req)
		if err != nil {
			log.Error().Err(err).Msg("netns_holder: mount failed")
		}
		return mountCmd
	}
	return nil
}

func handleExecConnection(conn net.Conn) {
	defer conn.Close()
	var req beam.ExecRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		log.Error().Err(err).Msg("netns_holder: failed to decode exec request")
		return
	}

	log.Debug().Strs("args", req.Args).Msg("netns_holder: performing execution")
	res := handleExecRequest(req)
	if err := json.NewEncoder(conn).Encode(res); err != nil {
		log.Error().Err(err).Msg("netns_holder: failed to encode response")
	}
}

func handleExecRequest(req beam.ExecRequest) beam.ExecResponse {
	if len(req.Args) == 0 {
		return beam.ExecResponse{Error: "empty command"}
	}

	//nolint:gosec // G204: Args are validated and controlled within the system
	cmd := exec.CommandContext(context.Background(), req.Args[0], req.Args[1:]...)
	// Connect to host's stdio (the holder inherited them)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if !req.Wait {
		if startErr := cmd.Start(); startErr != nil {
			return beam.ExecResponse{Error: startErr.Error()}
		}
		return beam.ExecResponse{Pid: cmd.Process.Pid}
	}

	if runErr := cmd.Run(); runErr != nil {
		exitCode := -1
		var exitError *exec.ExitError
		if errors.As(runErr, &exitError) {
			exitCode = exitError.ExitCode()
		}
		return beam.ExecResponse{Error: runErr.Error(), ExitCode: exitCode}
	}

	return beam.ExecResponse{ExitCode: 0, Pid: cmd.Process.Pid}
}

func performRootlessMount(req beam.MountRequest) (*exec.Cmd, error) {
	exe := req.Source
	var args []string
	foreground := false

	if req.Type == "fuse-overlayfs" {
		if found, err := bin.Find("fuse-overlayfs"); err == nil {
			exe = found
		}
		foreground = true
		if filtered := sanitizeFuseOverlayFSOptions(req.Options); len(filtered) > 0 {
			args = append(args, "-o", strings.Join(filtered, ","))
		}
		args = append(args, "-f")
	} else {
		args = append(args, req.Options...)
	}

	args = append(args, req.Target)

	log.Debug().Str("exe", exe).Strs("args", args).Msg("netns_holder: executing mount command")
	cmd := exec.CommandContext(context.Background(), exe, args...)
	cmd.Stdout = os.Stdout

	if !foreground {
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return nil, err
		}
		// Return a fake "finished" command to satisfy the nilnil check.
		return exec.CommandContext(context.Background(), "true"), nil
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}
	go streamFuseOverlayFSStderr(stderrPipe, os.Stderr)

	if errStart := cmd.Start(); errStart != nil {
		return nil, errStart
	}
	log.Debug().Int("pid", cmd.Process.Pid).Str("target", req.Target).
		Msg("netns_holder: started foreground FUSE mount helper")

	if errMountReady := waitForMountReady(req.Target); errMountReady != nil {
		if sigErr := cmd.Process.Signal(syscall.SIGTERM); sigErr != nil {
			log.Error().Err(sigErr).Msg("netns_holder: failed to send SIGTERM to FUSE mount helper")
		}
		if _, waitErr := cmd.Process.Wait(); waitErr != nil {
			log.Error().Err(waitErr).Msg("netns_holder: failed to wait for FUSE mount helper")
		}
		return nil, errMountReady
	}

	go func() {
		errWait := cmd.Wait()
		if errWait != nil {
			log.Error().
				Err(errWait).
				Int("pid", cmd.Process.Pid).
				Msg("netns_holder: FUSE mount helper exited")
			return
		}
		log.Debug().
			Int("pid", cmd.Process.Pid).
			Msg("netns_holder: FUSE mount helper exited cleanly")
	}()

	return cmd, nil
}

func sanitizeFuseOverlayFSOptions(options []string) []string {
	filtered := make([]string, 0, len(options))
	for _, optStr := range options {
		for part := range strings.SplitSeq(optStr, ",") {
			opt := strings.TrimSpace(part)
			if opt == "" {
				continue
			}
			switch opt {
			case "lazytime", "relatime":
				continue
			default:
				filtered = append(filtered, opt)
			}
		}
	}
	return filtered
}

func streamFuseOverlayFSStderr(r io.Reader, w io.Writer) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if shouldSuppressFuseOverlayFSWarning(line) {
			log.Debug().
				Str("line", line).
				Msg("netns_holder: suppressed benign fuse-overlayfs warning")
			continue
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			log.Error().Err(err).Msg("netns_holder: failed to write to stderr pipe")
		}
	}
	if err := scanner.Err(); err != nil {
		log.Debug().Err(err).Msg("netns_holder: stopped reading fuse-overlayfs stderr")
	}
}

func shouldSuppressFuseOverlayFSWarning(line string) bool {
	return strings.TrimSpace(line) == "unknown argument ignored: lazytime"
}

const (
	mountWaitInterval = 20 * time.Millisecond
	mountWaitTimeout  = 2 * time.Second
)

func waitForMountReady(target string) error {
	mountInfo, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return fmt.Errorf("open mountinfo: %w", err)
	}
	if errClose := mountInfo.Close(); errClose != nil {
		return fmt.Errorf("close mountinfo: %w", errClose)
	}

	cleanTarget := filepath.Clean(target)
	deadline := time.Now().Add(mountWaitTimeout)
	for time.Now().Before(deadline) {
		ready, errMount := isMountedAt(cleanTarget)
		if errMount != nil {
			return errMount
		}
		if ready {
			return nil
		}
		time.Sleep(mountWaitInterval)
	}

	return fmt.Errorf("timed out waiting for mount at %s", target)
}

func isMountedAt(target string) (bool, error) {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return false, fmt.Errorf("open mountinfo: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 5 && fields[4] == target {
			return true, nil
		}
	}
	if errScan := scanner.Err(); errScan != nil {
		return false, fmt.Errorf("scan mountinfo: %w", errScan)
	}
	return false, nil
}

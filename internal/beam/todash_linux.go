package beam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"

	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/unix"

	"github.com/garnizeh/maestro/internal/bin"
	"github.com/garnizeh/maestro/internal/white"
)

const (
	holderIDLen          = 12
	holderSocketTimeout  = 2 * time.Second
	holderNSReadyTimeout = 1 * time.Second
)

func newDefaultMounter() Mounter {
	return &RealMounter{
		sys:      realSyscallMounter{},
		fs:       RealFS{},
		cmd:      RealCommander{},
		lookPath: bin.Find,
	}
}

type RealMounter struct {
	sys      SyscallMounter
	fs       FS
	cmd      Commander
	rootless bool
	lookPath func(string) (string, error)
}

func (m *RealMounter) SetFS(fs FS) {
	m.fs = fs
}

func (m *RealMounter) SetCommander(cmd Commander) {
	m.cmd = cmd
}

// NewNS create a persistent network namespace by unsharing the thread's netns
// and bind-mounting it to the target path. In rootless mode, it starts a holder process.
func (m *RealMounter) NewNS(nsPath string, mount *MountRequest) (string, string, error) {
	if m.rootless {
		log.Debug().Str("nsPath", nsPath).Msg("beam: creating rootless network namespace")
		return m.newNSRootless(nsPath, mount)
	}

	// Create the file that will act as the mount point.
	f, err := m.fs.Create(nsPath)
	if err != nil {
		if !m.fs.IsExist(err) {
			return "", "", fmt.Errorf("failed to create mount point %s: %w", nsPath, err)
		}
	} else {
		if errClose := f.Close(); errClose != nil {
			log.Debug().Err(errClose).Str("path", nsPath).Msg("todash: failed to close mount point file")
		}
	}

	var g errgroup.Group
	g.Go(func() error {
		// Lock the OS thread to prevent other goroutines from being scheduled here.
		runtime.LockOSThread()

		flags := unix.CLONE_NEWNET
		if unshareErr := m.sys.Unshare(flags); unshareErr != nil {
			return fmt.Errorf("failed to unshare network namespace: %w", unshareErr)
		}

		if mountErr := m.sys.Mount("/proc/self/ns/net", nsPath, "none", unix.MS_BIND, ""); mountErr != nil {
			return fmt.Errorf("failed to bind mount network namespace to %s: %w", nsPath, mountErr)
		}
		log.Debug().Str("path", nsPath).Msg("todash: network namespace bind-mounted successfully")
		return nil
	})

	if gErr := g.Wait(); gErr != nil {
		return "", "", gErr
	}
	return nsPath, "", nil
}

func (m *RealMounter) newNSRootless(nsPath string, mount *MountRequest) (string, string, error) {
	// Rootless EINVAL fix: Use a dedicated process with SysProcAttr to unshare.
	log.Debug().Msg("todash: launching rootless netns holder process")

	uids, gids, errMaps := m.resolveIDMappings()

	sockPath := m.prepareHolderSocket(nsPath)

	cmd, err := m.launchHolder(sockPath)
	if err != nil {
		return "", "", err
	}

	pid := cmd.Process.Pid
	if errMaps == nil {
		if errApply := white.ApplyIDMappings(pid, uids, gids); errApply != nil {
			m.killHolder(cmd, "mapping failure")
			return "", "", fmt.Errorf(
				"todash: fatal: failed to apply extended ID mappings to PID %d: %w",
				pid, errApply)
		}
		log.Debug().Int("pid", pid).Msg("todash: successfully applied extended ID mappings")
	}

	conn, err := m.connectToHolder(sockPath, cmd)
	if err != nil {
		return "", "", err
	}
	defer conn.Close()

	if errEnc := m.sendMountRequest(conn, mount, cmd); errEnc != nil {
		return "", "", errEnc
	}

	// Store the PID to allow DeleteNS to kill it
	pidFile := nsPath + ".pid"
	if writeErr := m.fs.WriteFile(pidFile, []byte(strconv.Itoa(pid)), mappingPerm); writeErr != nil {
		log.Warn().
			Err(writeErr).
			Str("path", pidFile).
			Msg("todash: failed to write PID file for netns holder")
	}

	// Give the child a moment to setup its NS views
	effectivePath := fmt.Sprintf("/proc/%d/ns/net", pid)
	if errReady := m.waitForNSReady(effectivePath); errReady != nil {
		return "", "", errReady
	}

	return effectivePath, sockPath, nil
}

func (m *RealMounter) killHolder(cmd *exec.Cmd, reason string) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if errKill := cmd.Process.Kill(); errKill != nil {
		log.Debug().
			Err(errKill).
			Int("pid", cmd.Process.Pid).
			Msgf("todash: failed to kill holder after %s", reason)
	}
}

func (m *RealMounter) connectToHolder(sockPath string, cmd *exec.Cmd) (net.Conn, error) {
	deadline := time.Now().Add(holderSocketTimeout)
	var dialer net.Dialer
	for time.Now().Before(deadline) {
		if c, errDial := dialer.DialContext(context.Background(), "unix", sockPath); errDial == nil {
			return c, nil
		}
		time.Sleep(pollInterval)
	}

	m.killHolder(cmd, "socket timeout")
	return nil, fmt.Errorf("todash: timeout waiting for holder socket at %s", sockPath)
}

func (m *RealMounter) sendMountRequest(conn net.Conn, mount *MountRequest, cmd *exec.Cmd) error {
	payload := mount
	if payload == nil {
		payload = &MountRequest{}
	}
	log.Debug().Interface("mount", payload).Msg("todash: sending mount request to holder")
	if errEnc := json.NewEncoder(conn).Encode(payload); errEnc != nil {
		m.killHolder(cmd, "encode failure")
		return fmt.Errorf("todash: failed to encode mount request: %w", errEnc)
	}
	return nil
}

func (m *RealMounter) waitForNSReady(path string) error {
	deadline := time.Now().Add(holderNSReadyTimeout)
	for time.Now().Before(deadline) {
		if _, statErr := m.fs.Stat(path); statErr == nil {
			return nil
		}
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("timeout waiting for namespace at %s", path)
}

// HolderInvoke sends a command to an active holder and waits for response.
func HolderInvoke(ctx context.Context, nsPath string, req ExecRequest) (*ExecResponse, error) {
	base := filepath.Base(nsPath)
	if len(base) > holderIDLen {
		base = base[:holderIDLen]
	}
	sockPath := filepath.Join(filepath.Dir(nsPath), base+".sock")
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("todash: dial holder socket %s: %w", sockPath, err)
	}
	defer conn.Close()

	// 1. Send the ExecRequest
	if errEnc := json.NewEncoder(conn).Encode(req); errEnc != nil {
		return nil, fmt.Errorf("todash: encode exec request: %w", errEnc)
	}

	// 2. Read the ExecResponse
	var res ExecResponse
	if errDec := json.NewDecoder(conn).Decode(&res); errDec != nil {
		return nil, fmt.Errorf("todash: decode exec response: %w", errDec)
	}

	if res.Error != "" {
		return &res, fmt.Errorf("todash: holder execution error: %s", res.Error)
	}

	return &res, nil
}

// DeleteNS removes the persistent network namespace by unmounting and removing the file.
func (m *RealMounter) DeleteNS(nsPath string) error {
	if m.rootless {
		return m.deleteNSRootless(nsPath)
	}

	if err := m.sys.Unmount(nsPath, unix.MNT_DETACH); err != nil {
		if !errors.Is(err, syscall.EINVAL) &&
			!errors.Is(err, syscall.ENOENT) {
			return fmt.Errorf("failed to unmount network namespace %s: %w", nsPath, err)
		}
	}
	if removeErr := m.fs.Remove(nsPath); removeErr != nil && !m.fs.IsNotExist(removeErr) {
		return fmt.Errorf("failed to remove network namespace file %s: %w", nsPath, removeErr)
	}
	return nil
}

func (m *RealMounter) deleteNSRootless(nsPath string) error {
	pidFile := nsPath + ".pid"
	data, readErr := m.fs.ReadFile(pidFile)
	if readErr == nil {
		m.cleanupHolderByPIDFile(data)
		if errRem := m.fs.Remove(pidFile); errRem != nil && !m.fs.IsNotExist(errRem) {
			log.Debug().Err(errRem).Str("path", pidFile).Msg("todash: failed to remove PID file")
		}
	}
	base := filepath.Base(nsPath)
	if len(base) > holderIDLen {
		base = base[:holderIDLen]
	}
	sockPath := filepath.Join(filepath.Dir(nsPath), base+".sock")
	if errRem1 := os.Remove(sockPath); errRem1 != nil && !os.IsNotExist(errRem1) {
		log.Debug().Err(errRem1).Str("path", sockPath).Msg("todash: failed to remove holder socket")
	}
	if errRem2 := m.fs.Remove(nsPath); errRem2 != nil && !m.fs.IsNotExist(errRem2) {
		log.Debug().Err(errRem2).Str("path", nsPath).Msg("todash: failed to remove netns file")
	}
	return nil
}

func (m *RealMounter) cleanupHolderByPIDFile(data []byte) {
	pid, errPid := strconv.Atoi(string(data))
	if errPid != nil || pid <= 1 {
		return
	}

	log.Debug().Int("pid", pid).Msg("todash: killing rootless netns holder")
	proc, findErr := os.FindProcess(pid)
	if findErr != nil {
		return
	}

	if errSig := proc.Signal(syscall.SIGTERM); errSig != nil {
		log.Debug().Err(errSig).Int("pid", pid).Msg("todash: failed to signal SIGTERM to holder")
	}
	// Phase 2: Wait for it to avoid zombies
	state, errWait := proc.Wait()
	if errWait != nil {
		log.Debug().Err(errWait).Int("pid", pid).Msg("todash: failed to wait for holder process")
	} else {
		log.Debug().Int("pid", pid).Str("status", state.String()).Msg("todash: holder process exited")
	}
}
func (m *RealMounter) resolveIDMappings() ([]white.IDMapping, []white.IDMapping, error) {
	username := "userone" // Fallback
	if u, err := user.Current(); err == nil {
		username = u.Username
	}

	//nolint:gosec // G115: UIDs are non-negative and within 32-bit range on Linux
	uids, gids, err := white.BuildIDMappings(username, uint32(os.Getuid()), uint32(os.Getgid()))
	if err == nil {
		log.Debug().
			Str("user", username).
			Msg("todash: using extended ID mappings for rootless holder")
	} else {
		log.Warn().Err(err).Msg("todash: failing back to single ID mapping")
	}
	return uids, gids, err
}

func (m *RealMounter) prepareHolderSocket(nsPath string) string {
	base := filepath.Base(nsPath)
	if len(base) > holderIDLen {
		base = base[:holderIDLen]
	}
	sockPath := filepath.Join(filepath.Dir(nsPath), base+".sock")
	if errRem := os.Remove(sockPath); errRem != nil && !os.IsNotExist(errRem) {
		log.Debug().
			Err(errRem).
			Str("path", sockPath).
			Msg("todash: failed to remove stale holder socket")
	}
	return sockPath
}

func (m *RealMounter) launchHolder(sockPath string) (*exec.Cmd, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get maestro executable path: %w", err)
	}

	holderArgs := []string{"_netns_holder", "--socket", sockPath}
	for i, arg := range os.Args {
		if strings.HasPrefix(arg, "--log-level") {
			if strings.Contains(arg, "=") {
				holderArgs = append(holderArgs, arg)
			} else if i+1 < len(os.Args) {
				holderArgs = append(holderArgs, "--log-level", os.Args[i+1])
			}
			break
		}
	}

	cmd := m.cmd.CommandContext(context.Background(), exe, holderArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Unshareflags: syscall.CLONE_NEWUSER | syscall.CLONE_NEWNET | syscall.CLONE_NEWNS,
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if errStart := cmd.Start(); errStart != nil {
		return nil, fmt.Errorf("failed to start netns holder process: %w", errStart)
	}
	return cmd, nil
}

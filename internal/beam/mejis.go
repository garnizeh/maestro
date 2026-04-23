package beam

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/rs/zerolog/log"

	"github.com/rodrigo-baliza/maestro/internal/bin"
)

const (
	// DriverPasta is the name of the pasta rootless networking binary.
	DriverPasta = "pasta"
	// DriverSlirp is the name of the slirp4netns rootless networking binary.
	DriverSlirp = "slirp4netns"
)

// Mejis implements rootless networking using pasta or slirp4netns.
// "The Mejis of Eld" - The Dark Tower.
type Mejis struct {
	fs            FS
	stateDir      string
	lookPath      func(string) (string, error)
	runCmd        func(*exec.Cmd) error
	killProcessFn func(int) error
}

// NewMejis creates a new Mejis rootless network driver.
func NewMejis(stateDir string) *Mejis {
	return &Mejis{
		fs:       RealFS{},
		stateDir: stateDir,
		lookPath: bin.Find,
		runCmd: func(cmd *exec.Cmd) error {
			return cmd.Start()
		},
		killProcessFn: killProcess,
	}
}

// WithFS sets a custom filesystem implementation.
func (m *Mejis) WithFS(fs FS) *Mejis {
	m.fs = fs
	return m
}

// FindBinary returns the path to the first available rootless network binary.
// It prioritizes pasta over slirp4netns.
func (m *Mejis) FindBinary() (string, string, error) {
	if p, errPasta := m.lookPath(DriverPasta); errPasta == nil {
		return p, DriverPasta, nil
	}
	if p, errSlirp := m.lookPath(DriverSlirp); errSlirp == nil {
		return p, DriverSlirp, nil
	}
	return "", "", errors.New(
		"rootless networking requires 'pasta' or 'slirp4netns' to be installed",
	)
}

// holderPIDFromNSPath extracts the holder PID from a nsPath of the form
// /proc/<pid>/ns/net. Returns 0 if the path does not match the pattern.
func holderPIDFromNSPath(nsPath string) int {
	// expected: /proc/<pid>/ns/net
	parts := strings.Split(filepath.ToSlash(nsPath), "/")
	if len(parts) >= 3 && parts[1] == "proc" {
		pid, err := strconv.Atoi(parts[2])
		if err == nil {
			return pid
		}
	}
	return 0
}

// Attach connects a network namespace to the host network (rootless) using pasta.
// In rootless mode (launcherPath != "") pasta must run in the holder's user namespace
// so it can setns into the container netns, but it must stay in the HOST network
// namespace so it can bind the forwarded port there. We achieve this by launching
// pasta via "nsenter --user=/proc/<holderPID>/ns/user" from the maestro process
// (which is in the host netns). This is different from going through the holder
// socket, which would land pasta in the holder's own netns.
func (m *Mejis) Attach(
	ctx context.Context,
	containerID string,
	nsPath string,
	launcherPath string,
	portMappings []PortMapping,
) error {
	binPath, name, errBin := m.FindBinary()
	if errBin != nil {
		return errBin
	}

	log.Debug().
		Str("containerID", containerID).
		Str("nsPath", nsPath).
		Interface("portMappings", portMappings).
		Msg("mejis: attach rootless network")

	if name != DriverPasta {
		return errors.New("only 'pasta' is currently supported for rootless networking " +
			"(slirp4netns support coming soon)")
	}

	// pasta --netns <nsPath> -f --config-net -T none -U none
	// Note: pasta quits when the target netns is gone by default (since 2025);
	// the older --quit-if-ns-gone flag no longer exists in current builds.
	// --config-net tells pasta to configure the tap interface inside the namespace
	// (bring it up, assign IP via DHCP-like config); without this the container's
	// veth stays DOWN and no packets reach it.
	// -T none / -U none disable outbound (container→host) auto-scan forwarding;
	// without this pasta tries to splice-listen on ALL loopback ports inside the
	// namespace (incl. 80) and conflicts with services already bound there.
	args := []string{
		"--netns",
		nsPath,
		"-f",
		"--config-net",
		"--host-lo-to-ns-lo",
		"-T",
		"none",
		"-U",
		"none",
	}

	args = append(args, m.buildPortMappingArgs(portMappings)...)

	var cmd *exec.Cmd
	if launcherPath != "" {
		// Rootless mode: pasta must be in the holder's user NS (to have permission to
		// setns into the container netns) but must stay in the HOST netns (to bind the
		// forwarded port on the host). We use nsenter to enter only the user namespace.
		holderPID := holderPIDFromNSPath(nsPath)
		if holderPID == 0 {
			return fmt.Errorf("mejis: cannot derive holder PID from nsPath %q", nsPath)
		}
		nsenterPath, errNs := exec.LookPath("nsenter")
		if errNs != nil {
			return fmt.Errorf(
				"mejis: nsenter not found (required for rootless port forwarding): %w",
				errNs,
			)
		}
		userNSPath := fmt.Sprintf("/proc/%d/ns/user", holderPID)
		nsenterArgs := append([]string{"--user=" + userNSPath, "--", binPath}, args...)
		log.Debug().
			Str("containerID", containerID).
			Int("holderPID", holderPID).
			Str("userNS", userNSPath).
			Str("nsenter", nsenterPath).
			Strs("nsenterArgs", nsenterArgs).
			Msg("mejis: launching pasta via nsenter")
		cmd = exec.CommandContext(ctx, nsenterPath, nsenterArgs...)
	} else {
		log.Debug().
			Str("containerID", containerID).
			Str("binPath", binPath).
			Strs("args", args).
			Msg("mejis: launching pasta")
		cmd = exec.CommandContext(ctx, binPath, args...)
	}

	var pid int
	if errStart := m.runCmd(cmd); errStart != nil {
		return fmt.Errorf("failed to start %s: %w", name, errStart)
	}
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}

	// Save the PID if we need to kill it manually later
	if pid > 0 {
		pidPath := filepath.Join(m.stateDir, containerID+".pid")
		if errMk := m.fs.MkdirAll(m.stateDir, dirPerm); errMk != nil {
			return errMk
		}
		pidStr := strconv.Itoa(pid)
		if errWr := m.fs.WriteFile(pidPath, []byte(pidStr), filePerm); errWr != nil {
			return fmt.Errorf("failed to save networking PID: %w", errWr)
		}
	}

	return nil
}

// Detach disconnects the container from the network.
func (m *Mejis) Detach(_ context.Context, containerID string) error {
	pidPath := filepath.Join(m.stateDir, containerID+".pid")
	data, errRead := m.fs.ReadFile(pidPath)
	if errRead != nil {
		if m.fs.IsNotExist(errRead) {
			return nil // No PID file, assume already detached
		}
		return errRead
	}

	pid, errPid := strconv.Atoi(string(data))
	if errPid == nil && pid > 0 {
		m.cleanupPastaByPID(pid)
	}

	if errRem := m.fs.Remove(pidPath); errRem != nil && !m.fs.IsNotExist(errRem) {
		log.Debug().
			Err(errRem).
			Str("path", pidPath).
			Msg("mejis: failed to remove networking PID file")
	}
	return nil
}

func (m *Mejis) cleanupPastaByPID(pid int) {
	// Attempt to kill gracefully (SIGTERM)
	if errKill := m.killProcessFn(pid); errKill != nil {
		log.Debug().Err(errKill).Int("pid", pid).Msg("mejis: failed to signal pasta")
	}

	// Phase 2: Wait for it to avoid zombies
	if pid == os.Getpid() {
		// Don't wait for self in tests
		return
	}
	proc, errProc := os.FindProcess(pid)
	if errProc == nil {
		state, errWait := proc.Wait()
		if errWait != nil {
			log.Debug().Err(errWait).Int("pid", pid).Msg("mejis: failed to wait for pasta")
		} else {
			log.Debug().Int("pid", pid).Str("status", state.String()).Msg("mejis: pasta process exited")
		}
	}
}

func killProcess(pid int) error {
	proc, errProc := os.FindProcess(pid)
	if errProc != nil {
		return errProc
	}
	return proc.Signal(unix.SIGTERM)
}
func (m *Mejis) buildPortMappingArgs(portMappings []PortMapping) []string {
	var args []string
	for _, mapping := range portMappings {
		proto := strings.ToLower(mapping.Protocol)
		if proto == "" {
			proto = ProtoTCP
		}
		switch proto {
		case ProtoTCP:
			if mapping.HostPort < 1024 && os.Geteuid() != 0 {
				log.Warn().Int("port", mapping.HostPort).Msg(
					"binding to a privileged host port (<1024) " +
						"may fail without 'sys.net.ipv4.ip_unprivileged_port_start=0' sysctl")
			}
			args = append(args, "-t", fmt.Sprintf("%d:%d", mapping.HostPort, mapping.ContainerPort))
		case ProtoUDP:
			if mapping.HostPort < 1024 && os.Geteuid() != 0 {
				log.Warn().Int("port", mapping.HostPort).Msg(
					"binding to a privileged host port (<1024) " +
						"in rootless mode may fail without 'sys.net.ipv4.ip_unprivileged_port_start=0' sysctl")
			}
			args = append(args, "-u", fmt.Sprintf("%d:%d", mapping.HostPort, mapping.ContainerPort))
		}
	}
	return args
}

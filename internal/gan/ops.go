package gan

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	imagespec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/rodrigo-baliza/maestro/internal/eld"
	"github.com/rodrigo-baliza/maestro/internal/prim"
	"github.com/rodrigo-baliza/maestro/pkg/specgen"
)

const (
	// dirPerm is the default permission for container data directories.
	dirPerm = 0o700
)

// RunOpts holds the parameters for creating and starting a container.
type RunOpts struct {
	// Name is the human-readable container name. Auto-generated if empty.
	Name string
	// Image is the image reference (e.g. "nginx:latest").
	Image string
	// ImageDigest is the digest of the resolved image manifest.
	ImageDigest string
	// ImageConfig is the OCI image configuration (ENV, CMD, ENTRYPOINT, etc.).
	ImageConfig imagespec.ImageConfig
	// Cmd overrides the image's CMD.
	Cmd []string
	// Entrypoint overrides the image's ENTRYPOINT.
	Entrypoint []string
	// Env adds/overrides environment variables.
	Env []string
	// WorkDir overrides the working directory.
	WorkDir string
	// Labels are arbitrary key-value annotations.
	Labels map[string]string
	// ReadOnly makes the root filesystem read-only.
	ReadOnly bool
	// CapAdd adds Linux capabilities.
	CapAdd []string
	// CapDrop removes Linux capabilities.
	CapDrop []string
	// NetworkMode is "none", "host", or "private" (default).
	NetworkMode string
	// Detach runs the container in the background.
	Detach bool
	// Timeout is how long to wait for the container to start (default: 10s).
	Timeout time.Duration
}

// StopOpts holds the parameters for stopping a container.
type StopOpts struct {
	// Signal is the signal to send (default: SIGTERM).
	Signal syscall.Signal
	// Timeout is how long to wait for the process to exit (default: 10s).
	Timeout time.Duration
	// Force sends SIGKILL immediately.
	Force bool
}

// RmOpts holds parameters for removing a container.
type RmOpts struct {
	// Force removes a running container (implies Stop first).
	Force bool
}

// Ops is the high-level operations layer for Gan.
type Ops struct {
	// Manager is the container state manager.
	Manager *Manager
	// runtime is the OCI runtime driver.
	runtime eld.Eld
	// runtimeInfo is the discovered metadata for the OCI runtime.
	runtimeInfo eld.RuntimeInfo
	// snapshotter is the Prim storage driver.
	snapshotter prim.Prim
	// monitor is the process supervisor.
	monitor *eld.Monitor
	// dataRoot is the root directory for bundle/log/pid data.
	dataRoot string
	// newID is the container ID generator; replaced in tests.
	newID func() (string, error)
}

// NewOps returns an [Ops] instance.
func NewOps(
	manager *Manager,
	runtime eld.Eld,
	runtimeInfo eld.RuntimeInfo,
	snapshotter prim.Prim,
	dataRoot string,
) *Ops {
	return &Ops{
		Manager:     manager,
		runtime:     runtime,
		runtimeInfo: runtimeInfo,
		snapshotter: snapshotter,
		monitor:     eld.NewMonitor(runtime),
		dataRoot:    dataRoot,
		newID:       generateID,
	}
}

// ListContainers returns all containers (delegates to Manager).
func (o *Ops) ListContainers(ctx context.Context) ([]*Container, error) {
	return o.Manager.ListContainers(ctx)
}

// LoadContainer retrieves a container by ID (delegates to Manager).
func (o *Ops) LoadContainer(ctx context.Context, id string) (*Container, error) {
	return o.Manager.LoadContainer(ctx, id)
}

// Run creates and starts a new container.
func (o *Ops) Run( //nolint:funlen // complex setup orchestration
	ctx context.Context,
	opts RunOpts,
) (*Container, error) {
	// ── Generate container ID and name ────────────────────────────────────────
	id, err := o.newID()
	if err != nil {
		return nil, fmt.Errorf("gan: run: generate id: %w", err)
	}

	name := opts.Name
	if name == "" {
		name = id[:12]
	} else {
		existing, findErr := o.Manager.FindByName(ctx, name)
		if findErr != nil {
			return nil, fmt.Errorf("gan: run: find by name: %w", findErr)
		}
		if existing != nil {
			return nil, fmt.Errorf("%w: %s", ErrNameAlreadyInUse, name)
		}
	}

	// ── Prepare rootfs via snapshotter ────────────────────────────────────────
	snapshotKey := "rw-" + id
	rootfsMounts, snapErr := o.snapshotter.Prepare(ctx, snapshotKey, "")
	if snapErr != nil {
		return nil, fmt.Errorf("gan: run: prepare rootfs: %w", snapErr)
	}

	rootfsPath := ""
	if len(rootfsMounts) > 0 {
		rootfsPath = rootfsMounts[0].Source
	}

	// ── Prepare OCI bundle directory ──────────────────────────────────────────
	bundlePath := filepath.Join(o.dataRoot, "containers", id, "bundle")
	if mkErr := os.MkdirAll(bundlePath, dirPerm); mkErr != nil {
		return nil, fmt.Errorf("gan: run: mkdir bundle: %w", mkErr)
	}

	// ── Generate OCI Runtime Spec ─────────────────────────────────────────────
	specOpts := specgen.Opts{
		RootFS:      rootfsPath,
		Cmd:         opts.Cmd,
		Entrypoint:  opts.Entrypoint,
		Env:         opts.Env,
		WorkDir:     opts.WorkDir,
		ContainerID: id,
		ReadOnly:    opts.ReadOnly,
		CapAdd:      opts.CapAdd,
		CapDrop:     opts.CapDrop,
		NetworkMode: opts.NetworkMode,
	}
	spec, genErr := specgen.Generate(opts.ImageConfig, specOpts)
	if genErr != nil {
		return nil, fmt.Errorf("gan: run: generate spec: %w", genErr)
	}
	if writeErr := specgen.Write(bundlePath, spec); writeErr != nil {
		return nil, fmt.Errorf("gan: run: write spec: %w", writeErr)
	}

	// ── Persist the container in KaCreated state ──────────────────────────────
	logPath := filepath.Join(o.dataRoot, "containers", id, "container.log")
	pidFile := filepath.Join(o.dataRoot, "containers", id, "container.pid")
	rtInfo := o.runtimeInfo

	ctr := &Container{
		ID:          id,
		Name:        name,
		Image:       opts.Image,
		ImageDigest: opts.ImageDigest,
		Ka:          KaCreated,
		BundlePath:  bundlePath,
		RootFSPath:  rootfsPath,
		LogPath:     logPath,
		PidFile:     pidFile,
		RuntimeName: rtInfo.Name,
		Created:     time.Now().UTC(),
		Labels:      opts.Labels,
	}
	if saveErr := o.Manager.SaveContainer(ctx, ctr); saveErr != nil {
		return nil, fmt.Errorf("gan: run: save container: %w", saveErr)
	}

	// ── Launch via Monitor ────────────────────────────────────────────────────
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second //nolint:mnd // default 10s start timeout
	}

	monCfg := eld.MonitorConfig{
		ContainerID: id,
		BundlePath:  bundlePath,
		LogPath:     logPath,
		PidFile:     pidFile,
		ExitFile:    filepath.Join(o.dataRoot, "containers", id, "exit.code"),
		Detach:      opts.Detach,
		Timeout:     timeout,
	}

	result, monErr := o.monitor.Run(ctx, monCfg)
	if monErr != nil {
		// Mark the container as stopped on launch failure.
		ctr.Ka = KaStopped
		_ = o.Manager.SaveContainer(ctx, ctr)
		return nil, fmt.Errorf("gan: run: monitor: %w", monErr)
	}

	// ── Update state to Running ───────────────────────────────────────────────
	now := time.Now().UTC()
	ctr.Ka = KaRunning
	ctr.Pid = result.Pid
	ctr.Started = &now
	if saveErr := o.Manager.SaveContainer(ctx, ctr); saveErr != nil {
		return nil, fmt.Errorf("gan: run: update running state: %w", saveErr)
	}

	if !opts.Detach {
		// Foreground: process has already exited.
		finished := time.Now().UTC()
		ctr.Ka = KaStopped
		ctr.ExitCode = result.ExitCode
		ctr.Finished = &finished
		_ = o.Manager.SaveContainer(ctx, ctr)
	}

	return ctr, nil
}

// Stop sends a stop signal to a running container and waits for it to exit.
func (o *Ops) Stop(ctx context.Context, id string, opts StopOpts) error {
	ctr, err := o.Manager.LoadContainer(ctx, id)
	if err != nil {
		return err
	}
	if ctr.Ka != KaRunning {
		return fmt.Errorf("gan: stop: container %s is not running (Ka=%s)", id, ctr.Ka)
	}

	sig := opts.Signal
	if sig == 0 || opts.Force {
		sig = syscall.SIGTERM
		if opts.Force {
			sig = syscall.SIGKILL
		}
	}

	if killErr := o.runtime.Kill(ctx, id, sig); killErr != nil {
		if !isAlreadyDead(killErr) {
			return fmt.Errorf("gan: stop: kill: %w", killErr)
		}
	}

	// Poll until container reports stopped.
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second //nolint:mnd // default 10s stop timeout
	}
	if waitErr := o.waitForStop(ctx, id, timeout); waitErr != nil {
		return fmt.Errorf("gan: stop: wait: %w", waitErr)
	}

	// Delete the OCI runtime state.
	if delErr := o.runtime.Delete(ctx, id, nil); delErr != nil {
		if !isAlreadyGone(delErr) {
			return fmt.Errorf("gan: stop: delete runtime: %w", delErr)
		}
	}

	finished := time.Now().UTC()
	ctr.Ka = KaStopped
	ctr.Finished = &finished
	if saveErr := o.Manager.SaveContainer(ctx, ctr); saveErr != nil {
		return fmt.Errorf("gan: stop: save: %w", saveErr)
	}
	return nil
}

// Rm removes a container and its associated storage.
func (o *Ops) Rm(ctx context.Context, id string, opts RmOpts) error {
	ctr, err := o.Manager.LoadContainer(ctx, id)
	if err != nil {
		return err
	}

	if ctr.Ka == KaRunning {
		if !opts.Force {
			return fmt.Errorf("%w: %s", ErrContainerRunning, id)
		}
		if stopErr := o.Stop(ctx, id, StopOpts{Force: true}); stopErr != nil {
			return fmt.Errorf("gan: rm: force stop: %w", stopErr)
		}
	}

	// Remove the container data directory.
	containerDir := filepath.Join(o.dataRoot, "containers", id)
	if rmErr := os.RemoveAll(containerDir); rmErr != nil {
		return fmt.Errorf("gan: rm: remove data dir: %w", rmErr)
	}

	// Remove the state record.
	if delErr := o.Manager.DeleteContainer(ctx, id); delErr != nil {
		return fmt.Errorf("gan: rm: delete state: %w", delErr)
	}

	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// waitForStop polls the runtime state until the container is stopped.
func (o *Ops) waitForStop(ctx context.Context, id string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		state, err := o.runtime.State(ctx, id)
		if err != nil {
			if isAlreadyGone(err) {
				return nil
			}
			return err
		}
		if state.Status == eld.StatusStopped {
			return nil
		}
		time.Sleep(50 * time.Millisecond) //nolint:mnd // standard polling interval
	}
	return fmt.Errorf("timed out waiting for container %s to stop", id)
}

// generateID creates a cryptographically random 64-hex-char container ID.
func generateID() (string, error) {
	b := make([]byte, 32) //nolint:mnd // 32 bytes = 64 hex characters
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// isAlreadyDead reports whether the kill error indicates the process is gone.
func isAlreadyDead(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "no such process") ||
		strings.Contains(s, "not running") ||
		strings.Contains(s, "not found")
}

// isAlreadyGone reports whether a runtime error indicates the container is gone.
func isAlreadyGone(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "not found") ||
		strings.Contains(s, "does not exist") ||
		strings.Contains(s, "no such")
}

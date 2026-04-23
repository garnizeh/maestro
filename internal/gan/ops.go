package gan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	imagespec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/rs/zerolog/log"

	"github.com/garnizeh/maestro/internal/beam"
	"github.com/garnizeh/maestro/internal/eld"
	"github.com/garnizeh/maestro/internal/prim"
	"github.com/garnizeh/maestro/internal/white"
	"github.com/garnizeh/maestro/pkg/specgen"
)

const (
	// dirPerm is the default permission for container data directories.
	dirPerm = 0o700
	// rootfsDirPerm must allow path traversal for non-root container users.
	rootfsDirPerm = 0o755

	shortIDLen     = 12
	minVolumeParts = 2
)

// CreateOpts holds the parameters for creating a container.
type CreateOpts struct {
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
	// Ports contains standard port mapping configurations (-p).
	Ports []string
	// Volumes contains container volume mount configurations (-v).
	Volumes []string
}

// StartOpts holds the parameters for starting an existing container.
type StartOpts struct {
	// Detach runs the container in the background.
	Detach bool
	// Stdout is an optional writer to stream real-time container output.
	Stdout io.Writer
	// Stderr is an optional writer to stream real-time container output.
	Stderr io.Writer
	// Timeout is how long to wait for the container to start (default: 10s).
	Timeout time.Duration
}

// RunOpts holds the parameters for creating and starting a container.
type RunOpts struct {
	CreateOpts
	StartOpts
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

// NetworkManager defines the interface for container networking.
type NetworkManager interface {
	Attach(
		ctx context.Context,
		id string,
		mount *beam.MountRequest,
		portMappings []beam.PortMapping,
	) (*beam.AttachResult, error)
	Detach(ctx context.Context, id string, portMappings []beam.PortMapping) error
}

// ImageStore defines the interface for local image operations.
type ImageStore interface {
	Swell(ctx context.Context, ref string, p prim.Prim) (string, error)
	GetConfig(ctx context.Context, ref string) (imagespec.ImageConfig, string, error)
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
	// networkMgr holds the reference to the CNI Beam manager.
	networkMgr NetworkManager
	// imageStore is the Maturin image store for pulling/swelling layers.
	imageStore ImageStore

	// dataRoot is the root directory for bundle/log/pid data.
	dataRoot string
	// seccompProfile is the default seccomp configuration.
	seccompProfile *white.Seccomp

	// injectable interfaces for testing
	fs      FS
	mounter Mounter
	specGen SpecGenerator
	idGen   IDGenerator
}

// NewOps returns an [Ops] instance.
func NewOps(
	m *Manager,
	runtime eld.Eld,
	rtInfo eld.RuntimeInfo,
	snapshotter prim.Prim,
	networkMgr NetworkManager,
	imageStore ImageStore,
	dataRoot string,
) *Ops {
	o := &Ops{
		Manager:     m,
		runtime:     runtime,
		runtimeInfo: rtInfo,
		snapshotter: snapshotter,
		monitor:     eld.NewMonitor(runtime),
		networkMgr:  networkMgr,
		imageStore:  imageStore,
		dataRoot:    dataRoot,
		fs:          RealFS{},
		mounter:     &RealMounter{},
		specGen:     realSpecGenerator{},
		idGen:       realIDGenerator{},
	}
	return o
}

// WithFS sets a custom filesystem implementation.
func (o *Ops) WithFS(f FS) *Ops {
	o.fs = f
	return o
}

// WithMounter sets a custom mounter implementation.
func (o *Ops) WithMounter(m Mounter) *Ops {
	o.mounter = m
	return o
}

// WithSpecGenerator sets a custom spec generator implementation.
func (o *Ops) WithSpecGenerator(s SpecGenerator) *Ops {
	o.specGen = s
	return o
}

// WithIDGenerator sets a custom ID generator implementation.
func (o *Ops) WithIDGenerator(g IDGenerator) *Ops {
	o.idGen = g
	return o
}

// WithSeccompProfile sets the default seccomp profile.
func (o *Ops) WithSeccompProfile(s *white.Seccomp) *Ops {
	o.seccompProfile = s
	return o
}

// ListContainers returns all containers (delegates to Manager).
func (o *Ops) ListContainers(ctx context.Context) ([]*Container, error) {
	return o.Manager.ListContainers(ctx)
}

// LoadContainer retrieves a container by ID or human-readable name.
func (o *Ops) LoadContainer(ctx context.Context, idOrName string) (*Container, error) {
	// ── 1. Try loading by full container ID ───────────────────────────────────
	ctr, err := o.Manager.LoadContainer(ctx, idOrName)
	if err == nil {
		return ctr, nil
	}

	// Any error other than "not found" is fatal.
	if !errors.Is(err, ErrContainerNotFound) {
		return nil, err
	}

	// ── 2. Fallback: search by human-readable Name ──────────────────────────────
	ctr, findErr := o.Manager.FindByName(ctx, idOrName)
	if findErr != nil {
		return nil, fmt.Errorf("gan: find by name %s: %w", idOrName, findErr)
	}

	if ctr != nil {
		return ctr, nil
	}

	// ── 3. Final Fallback: search by partial ID (unimplemented) ───────────────
	// Future optimization: match by short ID prefix (e.g. first 12 chars).

	return nil, fmt.Errorf("%w: %s", ErrContainerNotFound, idOrName)
}

// Create prepares a container from the given options, up to the KaCreated state.
func (o *Ops) Create(ctx context.Context, opts CreateOpts) (*Container, error) {
	id, err := o.idGen.NewID()
	if err != nil {
		return nil, fmt.Errorf("gan: create: generate id: %w", err)
	}

	defer func() {
		if err != nil {
			if rmErr := o.Rm(ctx, id, RmOpts{Force: true}); rmErr != nil {
				log.Warn().Err(rmErr).Str("id", id).Msg("gan: failed to cleanup partial container")
			}
		}
	}()

	log.Debug().Interface("opts", opts).Msg("gan: create: resolving metadata")
	name, cfg, dgst, err := o.resolveMetadata(ctx, id, opts)
	if err != nil {
		return nil, err
	}

	opts.ImageConfig = cfg
	opts.ImageDigest = dgst

	rootfsPath, bundlePath, mounts, rootlessMount, err := o.prepareFilesystem(ctx, id, opts.Image)
	if err != nil {
		return nil, err
	}

	log.Debug().Str("id", id).Bool("rootless", os.Getuid() != 0).
		Interface("mount", rootlessMount).Msg("gan: creating network namespace")
	netNSPath, launcherPath, err := o.attachNetwork(ctx, id, rootlessMount, opts)
	if err != nil {
		return nil, fmt.Errorf("gan: create: %w", err)
	}

	// ── Perform Host Mount (only if not delegated) ───────────────────────────

	if rootlessMount == nil {
		if mntErr := o.mountRootfs(ctx, mounts, rootfsPath); mntErr != nil {
			return nil, fmt.Errorf("gan: create: mount rootfs: %w", mntErr)
		}
	}

	if resolved, evalErr := o.fs.EvalSymlinks(rootfsPath); evalErr == nil {
		rootfsPath = resolved
	}

	// ── Generate OCI Runtime Spec ─────────────────────────────────────────────
	if genErr := o.writeRuntimeSpec(id, bundlePath, rootfsPath, netNSPath, opts); genErr != nil {
		return nil, fmt.Errorf("gan: create: %w", genErr)
	}

	// ── Persist the container in KaCreated state ──────────────────────────────
	logPath := filepath.Join(o.dataRoot, "containers", id, "container.log")
	pidFile := filepath.Join(o.dataRoot, "containers", id, "container.pid")
	rtInfo := o.runtimeInfo

	ctr := &Container{
		ID:           id,
		Name:         name,
		Image:        opts.Image,
		ImageDigest:  opts.ImageDigest,
		Ka:           KaCreated,
		BundlePath:   bundlePath,
		RootFSPath:   rootfsPath,
		LogPath:      logPath,
		PidFile:      pidFile,
		RuntimeName:  rtInfo.Name,
		Created:      time.Now().UTC(),
		Labels:       opts.Labels,
		Ports:        opts.Ports,
		NetNSPath:    netNSPath,
		LauncherPath: launcherPath,
	}
	if saveErr := o.Manager.SaveContainer(ctx, ctr); saveErr != nil {
		return nil, fmt.Errorf("gan: create: save container: %w", saveErr)
	}

	log.Debug().Str("id", id).Str("name", name).Str("image", opts.Image).
		Msg("gan: create: container persisted")

	return ctr, nil
}

// Start initiates execution of a previously created container.
func (o *Ops) Start(ctx context.Context, id string, opts StartOpts) (*Container, error) {
	ctr, err := o.Manager.LoadContainer(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("gan: start: %w", err)
	}

	if ctr.Ka != KaCreated && ctr.Ka != KaStopped {
		return nil, fmt.Errorf(
			"gan: start: container %s is in state %s; want Created or Stopped",
			id,
			ctr.Ka,
		)
	}

	// ── Launch via Monitor ────────────────────────────────────────────────────
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second //nolint:mnd // default 10s start timeout
	}

	monCfg := eld.MonitorConfig{
		ContainerID:  ctr.ID,
		BundlePath:   ctr.BundlePath,
		LogPath:      ctr.LogPath,
		PidFile:      ctr.PidFile,
		ExitFile:     filepath.Join(o.dataRoot, "containers", ctr.ID, "exit.code"),
		Detach:       opts.Detach,
		Stderr:       opts.Stderr,
		Timeout:      timeout,
		LauncherPath: ctr.LauncherPath,
	}

	result, monErr := o.monitor.Run(ctx, monCfg)
	if monErr != nil {
		// Mark the container as stopped on launch failure.
		ctr.Ka = KaStopped
		if saveErr := o.Manager.SaveContainer(ctx, ctr); saveErr != nil {
			log.Warn().
				Err(saveErr).
				Str("id", ctr.ID).
				Msg("gan: failed to save container state after monitor failure")
		}
		return nil, fmt.Errorf("gan: start: monitor: %w", monErr)
	}

	log.Debug().Str("id", ctr.ID).Int("pid", result.Pid).Int("exitCode", result.ExitCode).
		Msg("gan: start: monitor run completed")

	// ── Update state to Running ───────────────────────────────────────────────
	now := time.Now().UTC()
	ctr.Ka = KaRunning
	ctr.Pid = result.Pid
	ctr.Started = &now
	if saveErr := o.Manager.SaveContainer(ctx, ctr); saveErr != nil {
		return nil, fmt.Errorf("gan: start: update running state: %w", saveErr)
	}

	if !opts.Detach {
		// Foreground: process has already exited.
		finished := time.Now().UTC()
		ctr.Ka = KaStopped
		ctr.ExitCode = result.ExitCode
		ctr.Finished = &finished
		if saveErr := o.Manager.SaveContainer(ctx, ctr); saveErr != nil {
			log.Warn().
				Err(saveErr).
				Str("id", ctr.ID).
				Msg("gan: failed to save terminal state for foreground container")
		}
	}

	return ctr, nil
}

// Run creates and starts a new container.
func (o *Ops) Run(ctx context.Context, opts RunOpts) (*Container, error) {
	ctr, err := o.Create(ctx, opts.CreateOpts)
	if err != nil {
		return nil, err
	}

	return o.Start(ctx, ctr.ID, opts.StartOpts)
}

// Stop sends a stop signal to a running container and waits for it to exit.
func (o *Ops) Stop(ctx context.Context, id string, opts StopOpts) error {
	log.Debug().Str("id", id).Bool("force", opts.Force).Msg("gan: stopping container")
	ctr, err := o.LoadContainer(ctx, id)
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

	if killErr := o.runtime.Kill(ctx, ctr.ID, sig); killErr != nil {
		if !isAlreadyDead(killErr) {
			return fmt.Errorf("gan: stop: kill: %w", killErr)
		}
	}

	// Poll until container reports stopped.
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second //nolint:mnd // default 10s stop timeout
	}
	if waitErr := o.waitForStop(ctx, ctr.ID, timeout); waitErr != nil {
		return fmt.Errorf("gan: stop: wait: %w", waitErr)
	}
	log.Debug().Str("id", id).Msg("gan: stop: container halted")

	// Delete the OCI runtime state.
	if delErr := o.runtime.Delete(ctx, ctr.ID, nil); delErr != nil {
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

// Kill sends a signal to a container's init process.
func (o *Ops) Kill(ctx context.Context, id string, sig syscall.Signal) error {
	ctr, err := o.LoadContainer(ctx, id)
	if err != nil {
		return err
	}
	if ctr.Ka != KaRunning {
		return fmt.Errorf("gan: kill: container %s is not running (Ka=%s)", id, ctr.Ka)
	}

	if killErr := o.runtime.Kill(ctx, ctr.ID, sig); killErr != nil {
		return fmt.Errorf("gan: kill: %w", killErr)
	}
	return nil
}

// InspectResult holds the detailed information about a container.
type InspectResult struct {
	Container *Container      `json:"container"`
	OCIConfig any             `json:"oci_config"`
	Runtime   eld.RuntimeInfo `json:"runtime"`
}

// Inspect retrieves the full state and OCI configuration of a container.
func (o *Ops) Inspect(ctx context.Context, id string) (*InspectResult, error) {
	ctr, err := o.LoadContainer(ctx, id)
	if err != nil {
		return nil, err
	}

	res := &InspectResult{
		Container: ctr,
		Runtime:   o.runtimeInfo,
	}

	configPath := filepath.Join(ctr.BundlePath, "config.json")
	data, err := os.ReadFile(configPath)
	if err == nil {
		var ociConfig any
		if jsonErr := json.Unmarshal(data, &ociConfig); jsonErr == nil {
			res.OCIConfig = ociConfig
		}
	}

	return res, nil
}

// Rm removes a container and its associated storage.
func (o *Ops) Rm(ctx context.Context, id string, opts RmOpts) error {
	log.Debug().Str("id", id).Bool("force", opts.Force).Msg("gan: removing container")
	ctr, err := o.LoadContainer(ctx, id)
	if err != nil {
		return err
	}

	if ctr.Ka == KaRunning {
		if !opts.Force {
			return fmt.Errorf("%w: %s", ErrContainerRunning, ctr.ID)
		}
		if stopErr := o.Stop(ctx, ctr.ID, StopOpts{Force: true}); stopErr != nil {
			return fmt.Errorf("gan: rm: force stop: %w", stopErr)
		}
	}

	// ── Configure Networking via Beam (Cleanup) ───────────────────────────────
	if o.networkMgr != nil && ctr.NetNSPath != "" {
		var portMappings []beam.PortMapping
		for _, p := range ctr.Ports {
			if mappings, pErr := beam.ParsePortMapping(p); pErr == nil {
				portMappings = append(portMappings, mappings...)
			}
		}
		// Ignore detach errors as we're doing a best-effort cleanup
		if detErr := o.networkMgr.Detach(ctx, ctr.ID, portMappings); detErr != nil {
			log.Warn().
				Err(detErr).
				Str("id", ctr.ID).
				Msg("gan: failed to detach network during removal")
		}
	}

	// Unmount the rootfs if it exists to avoid EBUSY during RemoveAll.
	rootfsPath := filepath.Join(o.dataRoot, "containers", ctr.ID, "bundle", "rootfs")
	if unmErr := o.mounter.Unmount(ctx, rootfsPath); unmErr != nil {
		log.Debug().Err(unmErr).Str("id", ctr.ID).
			Msg("gan: failed to unmount rootfs during removal (expected if not mounted)")
	}

	// Remove the container data directory.
	containerDir := filepath.Join(o.dataRoot, "containers", ctr.ID)
	if rmErr := o.fs.RemoveAll(containerDir); rmErr != nil {
		return fmt.Errorf("gan: rm: remove data dir: %w", rmErr)
	}

	// Remove the state record.
	if delErr := o.Manager.DeleteContainer(ctx, ctr.ID); delErr != nil {
		return fmt.Errorf("gan: rm: delete state: %w", delErr)
	}

	return nil
}

func (o *Ops) resolveMetadata(
	ctx context.Context,
	id string,
	opts CreateOpts,
) (string, imagespec.ImageConfig, string, error) {
	name, err := o.resolveContainerName(ctx, id, opts.Name)
	if err != nil {
		return "", imagespec.ImageConfig{}, "", err
	}

	cfg, dgst, err := o.resolveImageConfig(ctx, opts.Image)
	if err != nil {
		return "", imagespec.ImageConfig{}, "", err
	}
	return name, cfg, dgst, nil
}

func (o *Ops) prepareFilesystem(
	ctx context.Context,
	id, image string,
) (string, string, []prim.Mount, *beam.MountRequest, error) {
	rootfsPath := filepath.Join(o.dataRoot, "containers", id, "bundle", "rootfs")
	bundlePath := filepath.Join(o.dataRoot, "containers", id, "bundle")

	parentKey, err := o.imageStore.Swell(ctx, image, o.snapshotter)
	if err != nil {
		return "", "", nil, nil, fmt.Errorf("gan: create: swell image: %w", err)
	}
	log.Debug().Str("image", image).Str("parentKey", parentKey).Msg("gan: prepareFS: image swelled")

	snapshotKey := "rw-" + id
	mounts, err := o.snapshotter.Prepare(ctx, snapshotKey, parentKey)
	if err != nil {
		return "", "", nil, nil, fmt.Errorf("gan: create: prepare snapshot: %w", err)
	}

	rootlessMount := o.buildRootlessMount(id, rootfsPath, mounts)

	if mkErr := o.fs.MkdirAll(bundlePath, dirPerm); mkErr != nil {
		return "", "", nil, nil, fmt.Errorf("gan: create: mkdir bundle: %w", mkErr)
	}
	if mkErr := o.fs.MkdirAll(rootfsPath, rootfsDirPerm); mkErr != nil {
		return "", "", nil, nil, fmt.Errorf("gan: create: mkdir rootfs: %w", mkErr)
	}

	return rootfsPath, bundlePath, mounts, rootlessMount, nil
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

func isAlreadyDead(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "no such process") ||
		strings.Contains(s, "no such file") ||
		strings.Contains(s, "not running") ||
		strings.Contains(s, "not found")
}

// isAlreadyGone reports whether a runtime error indicates the container is gone.
func isAlreadyGone(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "not found") ||
		strings.Contains(s, "does not exist") ||
		strings.Contains(s, "no such")
}

// attachNetwork handles the network namespace setup and port mapping via Beam.
func (o *Ops) attachNetwork(ctx context.Context, id string, mount *beam.MountRequest,
	opts CreateOpts) (string, string, error) {
	if o.networkMgr == nil || opts.NetworkMode == "host" {
		return "", "", nil // No setup needed
	}

	// For rootless, we MUST call Attach even if NetworkMode is "none"
	// if we have a mount request, because the holder manages the mount.
	if opts.NetworkMode == "none" && mount == nil {
		return "", "", nil
	}

	var portMappings []beam.PortMapping
	for _, p := range opts.Ports {
		mappings, err := beam.ParsePortMapping(p)
		if err != nil {
			return "", "", fmt.Errorf("parse port mapping %q: %w", p, err)
		}
		portMappings = append(portMappings, mappings...)
	}

	log.Debug().Str("id", id).Int("ports", len(portMappings)).Msg("gan: invoking beam attach")
	res, err := o.networkMgr.Attach(ctx, id, mount, portMappings)
	if err != nil {
		return "", "", fmt.Errorf("attach network: %w", err)
	}
	return res.NetNSPath, res.LauncherPath, nil
}

// mountRootfs executes the mount instructions from the snapshotter.
func (o *Ops) mountRootfs(ctx context.Context, mounts []prim.Mount, target string) error {
	log.Debug().Int("layers", len(mounts)).Str("target", target).Msg("gan: mounting rootfs")
	for _, m := range mounts {
		if err := o.mountLayer(ctx, m, target); err != nil {
			return err
		}
	}
	return nil
}

func (o *Ops) mountLayer(ctx context.Context, m prim.Mount, target string) error {
	flags := uintptr(0)
	data := ""

	for _, opt := range m.Options {
		switch opt {
		case "rw":
			// Default
		case "ro":
			flags |= msReadOnly
		case "bind":
			flags |= msBind
		default:
			if data != "" {
				data += ","
			}
			data += opt
		}
	}

	err := o.mounter.Mount(ctx, m.Source, target, m.Type, flags, data)
	if err == nil {
		return nil
	}

	return o.handleMountError(err, m, target, flags, data)
}

func (o *Ops) handleMountError(
	err error,
	m prim.Mount,
	target string,
	flags uintptr,
	data string,
) error {
	isPermissionError := errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES)
	if isPermissionError && (flags&msBind != 0) && os.Getuid() != 0 {
		if rmErr := o.fs.Remove(target); rmErr != nil {
			return fmt.Errorf("rootless mount fallback: remove %s: %w", target, rmErr)
		}

		if symErr := o.fs.Symlink(m.Source, target); symErr != nil {
			return fmt.Errorf(
				"rootless mount fallback: symlink %s -> %s: %w",
				m.Source,
				target,
				symErr,
			)
		}
		return nil
	}

	if errors.Is(err, syscall.EPERM) {
		return fmt.Errorf(
			"mount %s on %s: permission denied (Maestro may need root or CAP_SYS_ADMIN): %w",
			m.Source, target, err,
		)
	}

	return fmt.Errorf(
		"mount %s on %s type %s (flags: 0x%x, data: %q): %w",
		m.Source, target, m.Type, flags, data, err,
	)
}

func (o *Ops) writeRuntimeSpec(id, bundle, rootfs, netNS string, opts CreateOpts) error {
	mntNS := ""
	// When netNS points to a holder process (/proc/<pid>/ns/net), crun will be
	// executed from inside the holder, which is already in the holder's user
	// namespace.  Passing userNSPath would cause the kernel to return EINVAL on
	// setns because you cannot re-enter your current user namespace.
	// Instead we set InsideUserNS=true so specgen generates a child user
	// namespace with holder-relative mappings (0→0).
	insideUserNS := strings.HasPrefix(netNS, "/proc/") && strings.HasSuffix(netNS, "/ns/net")
	if insideUserNS {
		mntNS = strings.Replace(netNS, "/ns/net", "/ns/mnt", 1)
	}
	log.Debug().Str("id", id).Str("netNS", netNS).Str("mntNS", mntNS).
		Bool("insideUserNS", insideUserNS).Str("networkMode", opts.NetworkMode).
		Str("imageUser", opts.ImageConfig.User).Msg("gan: writeRuntimeSpec")

	var extraMounts []specgen.SpecMount
	for _, v := range opts.Volumes {
		parts := strings.Split(v, ":")
		if len(parts) < minVolumeParts {
			continue // skip invalid volume format (must at least be src:dst)
		}
		source := parts[0]
		dest := parts[1]
		mountOpts := []string{"bind"}
		if len(parts) > minVolumeParts {
			mountOpts = append(mountOpts, strings.Split(parts[2], ",")...)
		}
		extraMounts = append(extraMounts, specgen.SpecMount{
			Destination: dest,
			Type:        "bind",
			Source:      source,
			Options:     mountOpts,
		})
	}

	specOpts := specgen.Opts{
		RootFS:       rootfs,
		Cmd:          opts.Cmd,
		Entrypoint:   opts.Entrypoint,
		Env:          opts.Env,
		WorkDir:      opts.WorkDir,
		ContainerID:  id,
		ReadOnly:     opts.ReadOnly,
		CapAdd:       opts.CapAdd,
		CapDrop:      opts.CapDrop,
		NetworkMode:  opts.NetworkMode,
		NetNSPath:    netNS,
		MntNSPath:    mntNS,
		Rootless:     os.Getuid() != 0,
		InsideUserNS: insideUserNS,
		Seccomp:      o.seccompProfile,
		Mounts:       extraMounts,
	}
	spec, err := o.specGen.Generate(opts.ImageConfig, specOpts)
	if err != nil {
		return fmt.Errorf("generate spec: %w", err)
	}
	if writeErr := o.specGen.Write(bundle, spec); writeErr != nil {
		return fmt.Errorf("write spec: %w", writeErr)
	}
	return nil
}
func (o *Ops) buildRootlessMount(id, rootfsPath string, mounts []prim.Mount) *beam.MountRequest {
	if os.Getuid() == 0 {
		return nil
	}

	for _, m := range mounts {
		if m.Type != "fuse-overlayfs" {
			continue
		}

		holderOpts := make([]string, 0, len(m.Options)+1)
		hasAllowOther := false
		for _, opt := range m.Options {
			opt = strings.TrimSpace(opt)
			if opt == "" || opt == "lazytime" || opt == "relatime" {
				continue
			}
			if strings.HasPrefix(opt, "uidmapping=") || strings.HasPrefix(opt, "gidmapping=") {
				continue
			}
			if opt == "allow_other" {
				hasAllowOther = true
			}
			holderOpts = append(holderOpts, opt)
		}
		if !hasAllowOther {
			holderOpts = append(holderOpts, "allow_other")
		}

		log.Debug().Str("id", id).Str("source", m.Source).Str("target", rootfsPath).
			Strs("options", holderOpts).
			Msg("gan: delegating FUSE mount to netns holder (uidmapping stripped)")
		return &beam.MountRequest{
			Source:  m.Source,
			Target:  rootfsPath,
			Type:    m.Type,
			Options: holderOpts,
		}
	}
	return nil
}
func (o *Ops) resolveContainerName(ctx context.Context, id, name string) (string, error) {
	if name == "" {
		return id[:shortIDLen], nil
	}
	existing, findErr := o.Manager.FindByName(ctx, name)
	if findErr != nil {
		return "", fmt.Errorf("gan: create: find by name: %w", findErr)
	}
	if existing != nil {
		return "", fmt.Errorf("%w: %s", ErrNameAlreadyInUse, name)
	}
	return name, nil
}

func (o *Ops) resolveImageConfig(
	ctx context.Context,
	image string,
) (imagespec.ImageConfig, string, error) {
	cfg, dgst, err := o.imageStore.GetConfig(ctx, image)
	if err != nil {
		return imagespec.ImageConfig{}, "", fmt.Errorf("gan: create: resolve image config: %w", err)
	}
	return cfg, dgst, nil
}

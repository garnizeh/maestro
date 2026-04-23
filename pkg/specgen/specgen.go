package specgen

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	imagespec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/rodrigo-baliza/maestro/internal/white"
	"github.com/rs/zerolog/log"
)

const (
	// filePerm is the default permission for the config.json file.
	filePerm = 0o600
	// rlimitNofileDefault is the standard file descriptor limit for containers.
	rlimitNofileDefault = 1024
)

// Opts holds user-facing container parameters that override image defaults.
type Opts struct {
	// RootFS is the absolute path to the container rootfs directory.
	RootFS string
	// Cmd overrides the image CMD. If empty, the image CMD is used.
	Cmd []string
	// Entrypoint overrides the image ENTRYPOINT. If empty, the image ENTRYPOINT is used.
	Entrypoint []string
	// Env are additional environment variables in KEY=VALUE format.
	// Image environment variables are used as a base; these override them.
	Env []string
	// WorkDir overrides the image's working directory.
	WorkDir string
	// User overrides the image's user (USER:GROUP format).
	User string
	// Hostname sets the UTS hostname for the container (defaults to the container ID).
	Hostname string
	// ContainerID is used as the default hostname if Hostname is not set.
	ContainerID string
	// ReadOnly makes the root filesystem read-only.
	ReadOnly bool
	// CapAdd is a list of Linux capabilities to add.
	CapAdd []string
	// CapDrop is a list of Linux capabilities to drop.
	CapDrop []string
	// Rootless enables rootless-specific namespace and seccomp adjustments.
	Rootless bool
	// NetworkMode is the desired network namespace ("none", "host", or "private").
	// Defaults to "private" if empty.
	NetworkMode string
	// NetNSPath is the absolute path to a pre-created network namespace.
	NetNSPath string
	// UserNSPath is the absolute path to a pre-created user namespace.
	UserNSPath string
	// MntNSPath is the absolute path to a pre-created mount namespace.
	MntNSPath string
	// InsideUserNS indicates that the OCI runtime will be executed from inside an
	// existing user namespace (e.g. the rootless netns holder process). In this
	// case the runtime must NOT try to setns into the holder's user namespace
	// (which would return EINVAL) but should instead create a new child user
	// namespace, using holder-relative UID/GID mappings (0→0).
	InsideUserNS bool
	// Seccomp is the seccomp configuration to apply.
	Seccomp *white.Seccomp
	// Mounts are additional container mount points.
	Mounts []SpecMount
}

// Spec is a minimal OCI Runtime Spec document sufficient for container execution.
// We define our own types to avoid a hard dependency on the full runtime-spec
// module (which includes platform-specific types), using json-compatible fields.
type Spec struct {
	// OCIVersion is the version of the OCI runtime specification.
	OCIVersion string `json:"ociVersion"`
	// Root defines the container's root filesystem.
	Root SpecRoot `json:"root"`
	// Process defines the container's primary process.
	Process Process `json:"process"`
	// Hostname is the UTS hostname for the container.
	Hostname string `json:"hostname"`
	// Linux holds the Linux-specific portion of the OCI spec.
	Linux LinuxSpec `json:"linux"`
	// Mounts defines the container's mount points.
	Mounts []SpecMount `json:"mounts"`
}

// SpecRoot defines the container's root filesystem.
type SpecRoot struct {
	// Path is the absolute path to the rootfs.
	Path string `json:"path"`
	// Readonly indicates whether the rootfs is read-only.
	Readonly bool `json:"readonly"`
}

// SpecMount defines a mount point for the container.
type SpecMount struct {
	Destination string   `json:"destination"`
	Type        string   `json:"type"`
	Source      string   `json:"source"`
	Options     []string `json:"options,omitempty"`
}

// Process defines the container's primary process.
type Process struct {
	// Args is the list of process arguments.
	Args []string `json:"args"`
	// Env is the list of environment variables.
	Env []string `json:"env"`
	// Cwd is the current working directory.
	Cwd string `json:"cwd"`
	// User is the process user and group IDs.
	User ProcessUser `json:"user"`
	// Capabilities are the Linux capabilities for the process.
	Capabilities *LinuxCaps `json:"capabilities,omitempty"`
	// NoNewPrivileges prevents the process from gaining new privileges.
	NoNewPrivileges bool `json:"noNewPrivileges"`
	// Rlimits is the list of resource limits.
	Rlimits []ProcessRlimit `json:"rlimits,omitempty"`
	// Terminal indicates whether the process has a terminal.
	Terminal bool `json:"terminal"`
	// ConsoleSize is the initial terminal size.
	ConsoleSize *ConsoleSize `json:"consoleSize,omitempty"`
}

// ProcessUser defines the UID/GID for the container process.
type ProcessUser struct {
	// UID is the user ID.
	UID uint32 `json:"uid"`
	// GID is the group ID.
	GID uint32 `json:"gid"`
	// AdditionalGids is the list of supplementary group IDs.
	AdditionalGids []uint32 `json:"additionalGids,omitempty"`
}

// LinuxCaps represents Linux capabilities sets.
type LinuxCaps struct {
	// Bounding is the bounding set of capabilities.
	Bounding []string `json:"bounding,omitempty"`
	// Effective is the effective set of capabilities.
	Effective []string `json:"effective,omitempty"`
	// Inheritable is the inheritable set of capabilities.
	Inheritable []string `json:"inheritable,omitempty"`
	// Permitted is the permitted set of capabilities.
	Permitted []string `json:"permitted,omitempty"`
	// Ambient is the ambient set of capabilities.
	Ambient []string `json:"ambient,omitempty"`
}

// LinuxSeccomp represents the seccomp configuration.
type LinuxSeccomp struct {
	DefaultAction string           `json:"defaultAction"`
	Architectures []string         `json:"architectures,omitempty"`
	Syscalls      []SeccompSyscall `json:"syscalls,omitempty"`
}

// SeccompSyscall represents a syscall and its action in the seccomp filter.
type SeccompSyscall struct {
	Names  []string `json:"names"`
	Action string   `json:"action"`
}

// ProcessRlimit is a resource limit for the container process.
type ProcessRlimit struct {
	// Type is the resource limit type (e.g. RLIMIT_NOFILE).
	Type string `json:"type"`
	// Hard is the hard limit.
	Hard uint64 `json:"hard"`
	// Soft is the soft limit.
	Soft uint64 `json:"soft"`
}

// ConsoleSize is the terminal size.
type ConsoleSize struct {
	// Height is the terminal height in characters.
	Height uint `json:"height"`
	// Width is the terminal width in characters.
	Width uint `json:"width"`
}

// LinuxSpec holds the Linux-specific portion of the OCI spec.
type LinuxSpec struct {
	// Namespaces is the list of namespaces for the container.
	Namespaces []LinuxNamespace `json:"namespaces,omitempty"`
	// UIDMappings is the list of user ID mappings for rootless.
	UIDMappings []LinuxIDMapping `json:"uidMappings,omitempty"`
	// GIDMappings is the list of group ID mappings for rootless.
	GIDMappings []LinuxIDMapping `json:"gidMappings,omitempty"`
	// Seccomp is the seccomp configuration.
	Seccomp *LinuxSeccomp `json:"seccomp,omitempty"`
	// Resources is the cgroup resource limits.
	Resources *LinuxResources `json:"resources,omitempty"`
	// MaskedPaths is the list of paths to mask.
	MaskedPaths []string `json:"maskedPaths,omitempty"`
	// ReadonlyPaths is the list of paths to make read-only.
	ReadonlyPaths []string `json:"readonlyPaths,omitempty"`
	// RootfsPropagation sets the propagation type for the container rootfs mount.
	// Use "slave" for rootless containers to avoid EPERM when crun tries to
	// remount "/" as private inside a user namespace it does not own.
	RootfsPropagation string `json:"rootfsPropagation,omitempty"`
}

// LinuxNamespace is an OCI Linux namespace entry.
type LinuxNamespace struct {
	// Type is the namespace type (e.g. pid, mount).
	Type string `json:"type"`
	// Path is the absolute path to the namespace.
	Path string `json:"path,omitempty"`
}

// LinuxIDMapping is a user/group namespace UID/GID mapping.
type LinuxIDMapping struct {
	// ContainerID is the ID inside the container.
	ContainerID uint32 `json:"containerID"`
	// HostID is the ID on the host.
	HostID uint32 `json:"hostID"`
	// Size is the size of the mapping range.
	Size uint32 `json:"size"`
}

// LinuxResources describes cgroup resource limits.
type LinuxResources struct{}

// defaultCaps is the set of Linux capabilities granted to containers by default.
var defaultCaps = []string{ //nolint:gochecknoglobals // OCI default set
	"CAP_CHOWN",
	"CAP_DAC_OVERRIDE",
	"CAP_FSETID",
	"CAP_FOWNER",
	"CAP_MKNOD",
	"CAP_NET_RAW",
	"CAP_SETGID",
	"CAP_SETUID",
	"CAP_SETFCAP",
	"CAP_NET_BIND_SERVICE",
	"CAP_SYS_CHROOT",
	"CAP_KILL",
	"CAP_AUDIT_WRITE",
}

// Generate produces an OCI Runtime Spec from image configuration and user opts.
//
//nolint:funlen // complex OCI spec generation orchestration
func Generate(imgCfg imagespec.ImageConfig, opts Opts) (*Spec, error) {
	// ── Process args ──────────────────────────────────────────────────────────
	entrypoint := imgCfg.Entrypoint
	if len(opts.Entrypoint) > 0 {
		entrypoint = opts.Entrypoint
	}
	cmd := imgCfg.Cmd
	if len(opts.Cmd) > 0 {
		cmd = opts.Cmd
	}
	args := append(entrypoint, cmd...) //nolint:gocritic // append is safe for small lists
	log.Debug().Strs("args", args).Msg("specgen: resolved container command")

	// ── Environment variables ─────────────────────────────────────────────────
	envMap := make(map[string]string)
	for _, e := range imgCfg.Env {
		k, v, _ := strings.Cut(e, "=")
		envMap[k] = v
	}
	for _, e := range opts.Env {
		k, v, _ := strings.Cut(e, "=")
		envMap[k] = v
	}
	var env []string
	if _, ok := envMap["PATH"]; !ok {
		env = append(env, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	}
	for k, v := range envMap {
		env = append(env, k+"="+v)
	}

	// ── Working directory ─────────────────────────────────────────────────────
	cwd := imgCfg.WorkingDir
	if opts.WorkDir != "" {
		cwd = opts.WorkDir
	}
	if cwd == "" {
		cwd = "/"
	}

	// ── Hostname ──────────────────────────────────────────────────────────────
	hostname := opts.Hostname
	if hostname == "" {
		hostname = opts.ContainerID
		if len(hostname) > 12 { //nolint:mnd // standard short hostname length
			hostname = hostname[:12]
		}
	}

	// ── Capabilities ─────────────────────────────────────────────────────────
	caps := buildCaps(opts.CapAdd, opts.CapDrop)

	// ── Linux namespaces ──────────────────────────────────────────────────────
	namespaces := buildNamespaces(opts.NetworkMode, opts.NetNSPath, opts.UserNSPath,
		opts.MntNSPath, opts.Rootless, opts.InsideUserNS)

	// ── User/group ID mappings (rootless) ─────────────────────────────────────
	uid, uidMappings, gidMappings := buildUserMappings(imgCfg.User, opts.User,
		opts.UserNSPath, opts.Rootless, opts.InsideUserNS)

	// ── Rootfs ────────────────────────────────────────────────────────────────
	rootfs := opts.RootFS
	if rootfs == "" {
		rootfs = "rootfs"
	}

	spec := &Spec{
		OCIVersion: "1.0.2",
		Root: SpecRoot{
			Path:     rootfs,
			Readonly: opts.ReadOnly,
		},
		Process: Process{
			Args: args,
			Env:  env,
			Cwd:  cwd,
			User: ProcessUser{
				UID: uid,
				GID: 0,
			},
			Capabilities:    caps,
			NoNewPrivileges: true,
			Rlimits: []ProcessRlimit{
				{
					Type: "RLIMIT_NOFILE",
					Hard: rlimitNofileDefault,
					Soft: rlimitNofileDefault,
				},
			},
		},
		Hostname: hostname,
		Linux: LinuxSpec{
			Namespaces:        namespaces,
			UIDMappings:       uidMappings,
			GIDMappings:       gidMappings,
			Seccomp:           buildSeccomp(opts.Seccomp),
			RootfsPropagation: buildRootfsPropagation(opts.Rootless),
			MaskedPaths: []string{
				"/proc/acpi", "/proc/kcore", "/proc/keys",
				"/proc/latency_stats", "/proc/timer_list",
				"/proc/timer_stats", "/proc/sched_debug",
				"/proc/scsi", "/sys/firmware",
			},
			ReadonlyPaths: []string{
				"/proc/bus", "/proc/fs", "/proc/irq",
				"/proc/sys", "/proc/sysrq-trigger",
			},
		},
		Mounts: []SpecMount{
			{Destination: "/proc", Type: "proc", Source: "proc"},
			{
				Destination: "/dev",
				Type:        "tmpfs",
				Source:      "tmpfs",
				Options:     []string{"nosuid", "strictatime", "mode=755", "size=65536k"},
			},
			{
				Destination: "/dev/pts",
				Type:        "devpts",
				Source:      "devpts",
				Options: func() []string {
					mo := []string{"nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620"}
					if !opts.Rootless {
						mo = append(mo, "gid=5")
					}
					return mo
				}(),
			},
			{
				Destination: "/dev/shm",
				Type:        "tmpfs",
				Source:      "shm",
				Options:     []string{"nosuid", "noexec", "nodev", "mode=1777", "size=65536k"},
			},
			{
				Destination: "/dev/mqueue",
				Type:        "mqueue",
				Source:      "mqueue",
				Options:     []string{"nosuid", "noexec", "nodev"},
			},
			{
				Destination: "/sys",
				Type:        "sysfs",
				Source:      "sysfs",
				Options:     []string{"nosuid", "noexec", "nodev", "ro"},
			},
			{
				Destination: "/sys/fs/cgroup",
				Type:        "cgroup",
				Source:      "cgroup",
				Options:     []string{"nosuid", "noexec", "nodev", "relatime", "ro"},
			},
		},
	}

	spec.Mounts = append(spec.Mounts, opts.Mounts...)

	return spec, nil
}

// Write serialises spec to config.json inside dir.
func Write(dir string, spec *Spec) error {
	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return fmt.Errorf("specgen: marshal: %w", err)
	}
	path := filepath.Join(dir, "config.json")
	if writeErr := os.WriteFile(path, data, filePerm); writeErr != nil {
		return fmt.Errorf("specgen: write config.json: %w", writeErr)
	}
	return nil
}

// ── internal helpers ──────────────────────────────────────────────────────────

func buildCaps(capAdd, capDrop []string) *LinuxCaps {
	caps := make(map[string]bool)
	for _, c := range defaultCaps {
		caps[c] = true
	}
	for _, c := range capAdd {
		caps[normaliseCapability(c)] = true
	}
	for _, c := range capDrop {
		delete(caps, normaliseCapability(c))
	}

	var list []string
	for c := range caps {
		list = append(list, c)
	}

	return &LinuxCaps{
		Bounding:    list,
		Effective:   list,
		Inheritable: list,
		Permitted:   list,
		Ambient:     list,
	}
}

// normaliseCapability ensures the capability has the CAP_ prefix.
func normaliseCapability(c string) string {
	c = strings.ToUpper(c)
	if !strings.HasPrefix(c, "CAP_") {
		return "CAP_" + c
	}
	return c
}

func buildNamespaces(networkMode, netNSPath, userNSPath, mntNSPath string,
	rootless, insideUserNS bool) []LinuxNamespace {
	ns := []LinuxNamespace{
		{Type: "pid"},
		{Type: "ipc"},
		{Type: "uts"},
	}
	// When crun is launched inside the holder's user namespace (insideUserNS),
	// we must NOT join the holder's mount namespace (mntNSPath) and we stay in
	// the holder's user namespace (no new user NS entry).
	//
	// Correct behaviour: crun creates a brand-new child mount namespace via
	// unshare(CLONE_NEWNS) while it is still running in the holder's user
	// namespace.  The new mnt ns inherits ALL of the holder's mounts (including
	// the FUSE-overlayfs rootfs) and is OWNED by the holder's user namespace,
	// so crun (uid=0 in the holder's user ns) can set rootfs propagation on it.
	// The holder's user NS is kept in setgroups=allow (no deny is written), so
	// container processes can call initgroups/setgroups freely.
	//nolint:nestif // This block is intentionally verbose for clarity in rootless mode
	if insideUserNS {
		// New isolated mount namespace; inherits from holder's mount namespace.
		ns = append(ns, LinuxNamespace{Type: "mount"})
		// No user namespace: stay in holder's user namespace.
	} else {
		if mntNSPath != "" {
			ns = append(ns, LinuxNamespace{Type: "mount", Path: mntNSPath})
		} else {
			ns = append(ns, LinuxNamespace{Type: "mount"})
		}
		if rootless {
			if userNSPath != "" {
				ns = append(ns, LinuxNamespace{Type: "user", Path: userNSPath})
			} else {
				ns = append(ns, LinuxNamespace{Type: "user"})
			}
		}
	}

	switch networkMode {
	case "host":
		// No network namespace — share the host network.
	case "none":
		ns = append(ns, LinuxNamespace{Type: "network"})
	default: // "private" or ""
		ns = append(ns, LinuxNamespace{Type: "network", Path: netNSPath})
	}

	return ns
}

//nolint:unparam // UID is currently always 0, but API allows for future non-root defaults.
func buildUserMappings(
	_ /* imgUser */, _ /* optsUser */, userNSPath string,
	rootless, insideUserNS bool,
) (uint32, []LinuxIDMapping, []LinuxIDMapping) {
	log.Debug().Bool("rootless", rootless).Bool("insideUserNS", insideUserNS).
		Str("userNSPath", userNSPath).Msg("specgen: buildUserMappings")
	if !rootless {
		return 0, nil, nil
	}
	// When crun is already running inside the holder's user namespace we do NOT
	// create a new (nested) user namespace — no UID/GID mappings are needed.
	// The holder's user NS has setgroups=allow (we never write deny to it), so
	// container processes can call initgroups/setgroups freely.
	if insideUserNS {
		log.Debug().
			Msg("specgen: insideUserNS=true → returning no UID/GID mappings (stays in holder user NS)")
		return 0, nil, nil
	}
	// When joining an explicit pre-created user namespace there is nothing to
	// map — the namespace already has its own mappings.
	if userNSPath != "" {
		log.Debug().Str("userNSPath", userNSPath).
			Msg("specgen: joining existing userNS → returning no UID/GID mappings")
		return 0, nil, nil
	}

	// Rootless: map container UID 0 to the host user's UID.
	hostUID := uint32(os.Getuid()) //nolint:gosec // standard UID mapping for rootless
	hostGID := uint32(os.Getgid()) //nolint:gosec // standard GID mapping for rootless

	currentUser, err := userLookup()
	if err != nil {
		log.Debug().Err(err).Uint32("hostUID", hostUID).Uint32("hostGID", hostGID).
			Msg("specgen: userLookup failed, falling back to single-ID mapping")
		// Fallback to single ID mapping if user lookup fails
		return 0,
			[]LinuxIDMapping{{ContainerID: 0, HostID: hostUID, Size: 1}},
			[]LinuxIDMapping{{ContainerID: 0, HostID: hostGID, Size: 1}}
	}

	uidMaps, gidMaps, err := white.BuildIDMappings(currentUser, hostUID, hostGID)
	if err != nil {
		log.Debug().
			Err(err).
			Str("user", currentUser).
			Uint32("hostUID", hostUID).
			Uint32("hostGID", hostGID).
			Msg("specgen: BuildIDMappings failed, falling back to single-ID mapping")
		// Fallback to single ID mapping if subuid/subgid not found or insufficient
		return 0,
			[]LinuxIDMapping{{ContainerID: 0, HostID: hostUID, Size: 1}},
			[]LinuxIDMapping{{ContainerID: 0, HostID: hostGID, Size: 1}}
	}

	resultUID := translateMappings(uidMaps)
	resultGID := translateMappings(gidMaps)
	log.Debug().
		Interface("uidMappings", resultUID).
		Interface("gidMappings", resultGID).
		Msg("specgen: built ID mappings from subuid/subgid")
	return 0, resultUID, resultGID
}

func userLookup() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return u.Username, nil
}

func translateMappings(ms []white.IDMapping) []LinuxIDMapping {
	res := make([]LinuxIDMapping, len(ms))
	for i, m := range ms {
		res[i] = LinuxIDMapping{
			ContainerID: m.ContainerID,
			HostID:      m.HostID,
			Size:        m.Size,
		}
	}
	return res
}

// buildRootfsPropagation returns the rootfs propagation setting for the spec.
// Rootless containers cannot change the propagation of "/" to "rprivate"
// (which is crun's default) because "/" is owned by the initial user namespace.
// Using "slave" is permitted and prevents mount leaks back to the host.
func buildRootfsPropagation(rootless bool) string {
	if rootless {
		return "slave"
	}
	return ""
}

func buildSeccomp(s *white.Seccomp) *LinuxSeccomp {
	if s == nil {
		return nil
	}

	syscalls := make([]SeccompSyscall, len(s.Syscalls))
	for i, sys := range s.Syscalls {
		syscalls[i] = SeccompSyscall{
			Names:  sys.Names,
			Action: sys.Action,
		}
	}

	return &LinuxSeccomp{
		DefaultAction: s.DefaultAction,
		Architectures: s.Architectures,
		Syscalls:      syscalls,
	}
}

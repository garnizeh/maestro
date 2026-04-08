package specgen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	imagespec "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	// filePerm is the default permission for the config.json file.
	filePerm = 0o600
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
}

// SpecRoot defines the container's root filesystem.
type SpecRoot struct {
	// Path is the absolute path to the rootfs.
	Path string `json:"path"`
	// Readonly indicates whether the rootfs is read-only.
	Readonly bool `json:"readonly"`
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

// LinuxSeccomp is a minimal seccomp configuration.
type LinuxSeccomp struct {
	// DefaultAction is the default action for the seccomp filter.
	DefaultAction string `json:"defaultAction"`
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
	namespaces := buildNamespaces(opts.NetworkMode, opts.Rootless)

	// ── User/group ID mappings (rootless) ─────────────────────────────────────
	uid, uidMappings, gidMappings := buildUserMappings(imgCfg.User, opts.User, opts.Rootless)

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
				{Type: "RLIMIT_NOFILE", Hard: 1024, Soft: 1024}, //nolint:mnd // standard file descriptor limits
			},
		},
		Hostname: hostname,
		Linux: LinuxSpec{
			Namespaces:  namespaces,
			UIDMappings: uidMappings,
			GIDMappings: gidMappings,
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
	}

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

func buildNamespaces(networkMode string, rootless bool) []LinuxNamespace {
	ns := []LinuxNamespace{
		{Type: "pid"},
		{Type: "ipc"},
		{Type: "uts"},
		{Type: "mount"},
	}
	if rootless {
		ns = append(ns, LinuxNamespace{Type: "user"})
	}

	switch networkMode {
	case "host":
		// No network namespace — share the host network.
	case "none":
		ns = append(ns, LinuxNamespace{Type: "network"})
	default: // "private" or ""
		ns = append(ns, LinuxNamespace{Type: "network"})
	}

	return ns
}

func buildUserMappings(
	_, _ string,
	rootless bool,
) (uint32, []LinuxIDMapping, []LinuxIDMapping) { //nolint:unparam // currently returns 0, planned for future update
	if !rootless {
		return 0, nil, nil
	}

	// Rootless: map container UID 0 to the host user's UID.
	hostUID := uint32(os.Getuid()) //nolint:gosec // standard UID mapping for rootless
	hostGID := uint32(os.Getgid()) //nolint:gosec // standard GID mapping for rootless

	return 0,
		[]LinuxIDMapping{{ContainerID: 0, HostID: hostUID, Size: 65536}}, //nolint:mnd // standard id mapping size
		[]LinuxIDMapping{{ContainerID: 0, HostID: hostGID, Size: 65536}} //nolint:mnd // standard id mapping size
}

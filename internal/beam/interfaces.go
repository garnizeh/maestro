package beam

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/cni/pkg/types"

	"github.com/garnizeh/maestro/internal/sys"
	"github.com/garnizeh/maestro/pkg/archive"
)

const (
	mappingPerm  = 0o644
	pollInterval = 10 * time.Millisecond
)

// MountRequest represents a storage mount to be performed (often by a rootless holder).
type MountRequest struct {
	Source  string   `json:"source"`
	Target  string   `json:"target"`
	Type    string   `json:"type"`
	Options []string `json:"options"`
}

// ExecRequest is sent to the netns_holder to execute a command inside the NS.
type ExecRequest struct {
	Args []string `json:"args"`
	Wait bool     `json:"wait"`
}

// ExecResponse is returned by the holder after an execution.
type ExecResponse struct {
	Pid      int    `json:"pid"`
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
}

// PortMapping matches the CNI portmap plugin capability schema.
type PortMapping struct {
	HostPort      int    `json:"hostPort"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol"`
	HostIP        string `json:"hostIP,omitempty"`
}

// ── Internal testability interfaces ──────────────────────────────────────────

// FS abstracts several os package functions.
type FS interface {
	MkdirAll(path string, perm os.FileMode) error
	IsExist(err error) bool
	IsNotExist(err error) bool
	Remove(name string) error
	Create(name string) (*os.File, error)
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm os.FileMode) error
	Stat(name string) (os.FileInfo, error)
}

// Mounter abstracts Todash low-level namespace operations.
type Mounter interface {
	NewNS(nsPath string, mount *MountRequest) (netNSPath, launcherPath string, err error)
	DeleteNS(nsPath string) error
}

// SyscallMounter abstracts direct unix syscalls (Thin Shell).
type SyscallMounter interface {
	Unshare(flags int) error
	Mount(source string, target string, fstype string, flags uintptr, data string) error
	Unmount(target string, flags int) error
}

// Commander abstracts os/exec functions for mocking.
type Commander interface {
	CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd
}

// AttachResult carries information about an established container network.
type AttachResult struct {
	NetNSPath    string
	LauncherPath string
	Result       types.Result
}

// NetworkManager defines the high-level interface for Gan.
type NetworkManager interface {
	Attach(
		ctx context.Context,
		id string,
		mount *MountRequest,
		portMappings []PortMapping,
	) (*AttachResult, error)
	Detach(ctx context.Context, id string, portMappings []PortMapping) error
}

// PluginManager abstracts CNI plugin binary management.
type PluginManager interface {
	DownloadCNIPlugins(ctx context.Context, targetDir string) error
}

// namespaceManager abstracts Todash for testing.
type namespaceManager interface {
	NewNS(id string, mount *MountRequest) (netNSPath, launcherPath string, err error)
	DeleteNS(id string) error
	NSPath(id string) string
	WithRootless(bool) namespaceManager
}

// cniExecutor abstracts Guardian for testing.
type cniExecutor interface {
	LoadConfigList(confBytes []byte) (*libcni.NetworkConfigList, error)
	InvokeADD(ctx context.Context, netConfList *libcni.NetworkConfigList,
		containerID, netnsPath, ifName string, portMappings []PortMapping) (types.Result, error)
	InvokeDEL(ctx context.Context, netConfList *libcni.NetworkConfigList,
		containerID, netnsPath, ifName string, portMappings []PortMapping) error
	InvokeCHECK(ctx context.Context, netConfList *libcni.NetworkConfigList,
		containerID, netnsPath, ifName string) error
}

// ── Real implementations (Thin Shells) ───────────────────────────────────────

type RealFS = sys.RealFS

type RealCommander = sys.RealCommander

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Extractor interface {
	Extract(r io.Reader, targetDir string, opts archive.ExtractOptions) error
}

type realExtractor struct{}

func (realExtractor) Extract(r io.Reader, targetDir string, opts archive.ExtractOptions) error {
	return archive.ExtractTarGz(r, targetDir, opts)
}

package gan

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	imagespec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/rodrigo-baliza/maestro/internal/sys"
	"github.com/rodrigo-baliza/maestro/pkg/specgen"
)

// FS abstracts filesystem operations used by the Gan lifecycle manager.
type FS interface {
	MkdirAll(path string, perm os.FileMode) error
	Remove(path string) error
	RemoveAll(path string) error
	EvalSymlinks(path string) (string, error)
	Symlink(oldname, newname string) error
	Stat(name string) (os.FileInfo, error)
}

// Mounter abstracts the mount system call.
type Mounter interface {
	Mount(ctx context.Context, source, target, fstype string, flags uintptr, data string) error
	Unmount(ctx context.Context, target string) error
}

// SpecGenerator abstracts OCI runtime configuration generation and persistence.
type SpecGenerator interface {
	Generate(conf imagespec.ImageConfig, opts specgen.Opts) (*specgen.Spec, error)
	Write(bundlePath string, spec *specgen.Spec) error
}

// IDGenerator abstracts random container ID generation.
type IDGenerator interface {
	NewID() (string, error)
}

// ── Thin Shell Implementations ───────────────────────────────────────────────

type RealFS = sys.RealFS
type RealMounter = sys.RealMounter

const (
	msReadOnly = 0x1    // syscall.MS_RDONLY
	msBind     = 0x1000 // syscall.MS_BIND
)

type realSpecGenerator struct{}

func (realSpecGenerator) Generate(
	conf imagespec.ImageConfig,
	opts specgen.Opts,
) (*specgen.Spec, error) {
	return specgen.Generate(conf, opts)
}

func (realSpecGenerator) Write(bundlePath string, spec *specgen.Spec) error {
	return specgen.Write(bundlePath, spec)
}

const macHasherSize = 32

type realIDGenerator struct{}

func (realIDGenerator) NewID() (string, error) {
	b := make([]byte, macHasherSize)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

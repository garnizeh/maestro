// Package sys provides thin wrappers over standard system calls for dependency injection.
// These implementations are used in production code and as bases for mocks in tests.
package sys

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/rs/zerolog/log"
	"golang.org/x/sys/unix"
)

// ── Filesystem ───────────────────────────────────────────────────────────────

// RealFS is a thin shell over common os and filepath functions.
type RealFS struct{}

func (RealFS) MkdirAll(
	p string,
	m os.FileMode,
) error {
	return os.MkdirAll(p, m)
}

func (RealFS) MkdirTemp(
	d, p string,
) (string, error) {
	return os.MkdirTemp(d, p)
}
func (RealFS) Remove(n string) error                    { return os.Remove(n) }
func (RealFS) RemoveAll(p string) error                 { return os.RemoveAll(p) }
func (RealFS) Rename(o, n string) error                 { return os.Rename(o, n) }
func (RealFS) Stat(n string) (os.FileInfo, error)       { return os.Stat(n) }
func (RealFS) FileStat(f *os.File) (os.FileInfo, error) { return f.Stat() }
func (RealFS) Open(n string) (*os.File, error)          { return os.Open(n) }

func (RealFS) OpenFile(
	n string,
	f int,
	m os.FileMode,
) (*os.File, error) {
	return os.OpenFile(n, f, m)
}
func (RealFS) Create(n string) (*os.File, error) { return os.Create(n) }

func (RealFS) CreateTemp(
	d, p string,
) (*os.File, error) {
	return os.CreateTemp(d, p)
}
func (RealFS) ReadFile(n string) ([]byte, error) { return os.ReadFile(n) }

func (RealFS) WriteFile(
	n string,
	d []byte,
	p os.FileMode,
) error {
	return os.WriteFile(n, d, p)
}
func (RealFS) Readlink(n string) (string, error) { return os.Readlink(n) }
func (RealFS) Symlink(o, n string) error         { return os.Symlink(o, n) }

func (RealFS) EvalSymlinks(
	p string,
) (string, error) {
	return filepath.EvalSymlinks(p)
}
func (RealFS) ReadDir(n string) ([]os.DirEntry, error) { return os.ReadDir(n) }

func (RealFS) Walk(
	r string,
	f filepath.WalkFunc,
) error {
	return filepath.Walk(r, f)
}

func (RealFS) WalkDir(
	r string,
	f fs.WalkDirFunc,
) error {
	return filepath.WalkDir(r, f)
}
func (RealFS) IsNotExist(e error) bool             { return os.IsNotExist(e) }
func (RealFS) IsExist(e error) bool                { return os.IsExist(e) }
func (RealFS) Chmod(n string, m os.FileMode) error { return os.Chmod(n, m) }
func (RealFS) Abs(p string) (string, error)        { return filepath.Abs(p) }

func (RealFS) Flock(
	f int,
	h int,
) error {
	return syscall.Flock(f, h)
}

func (RealFS) Copy(
	dst io.Writer,
	src io.Reader,
) (int64, error) {
	return io.Copy(dst, src)
}
func (RealFS) UserHomeDir() (string, error) { return os.UserHomeDir() }
func (RealFS) Getenv(k string) string       { return os.Getenv(k) }

// ── Commander ────────────────────────────────────────────────────────────────

// RealCommander is a thin shell over common os/exec functions.
type RealCommander struct{}

func (RealCommander) CommandContext(ctx context.Context, n string, a ...string) *exec.Cmd {
	return exec.CommandContext(ctx, n, a...)
}
func (RealCommander) Command(n string, a ...string) *exec.Cmd {
	return exec.Command(n, a...) //nolint:noctx // intentionally used for background monitor re-exec
}
func (RealCommander) LookPath(f string) (string, error) { return exec.LookPath(f) }

// ── Mounter ──────────────────────────────────────────────────────────────────

// RealMounter is a thin shell over platform-specific mount operations.
type RealMounter struct {
	// BinaryPath is the path to the fuse-overlayfs binary.
	BinaryPath string
}

// Mount implements the Mounter interface using the mount system call.
func (rm *RealMounter) Mount(
	ctx context.Context,
	_, target, fstype string,
	_ uintptr,
	data string,
) error {
	log.Debug().
		Str("fstype", fstype).
		Str("target", target).
		Str("binary", rm.BinaryPath).
		Msg("sys: mounting")
	if (fstype == "fuse.fuse-overlayfs" || fstype == "fuse-overlayfs") && rm.BinaryPath != "" {
		// For rootless fuse-overlayfs, we might need to use the binary directly
		// if the mount syscall is not sufficient/available for the FUSE helper.
		bin := rm.BinaryPath
		cmd := exec.CommandContext(ctx, bin, "-o", data, target)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("fuse-overlayfs mount failed: %w (output: %s)", err, string(out))
		}
		return nil
	}
	return unix.Mount("", target, fstype, 0, data)
}

// Unmount detaches the filesystem at the given target.
func (rm *RealMounter) Unmount(ctx context.Context, target string) error {
	log.Debug().Str("target", target).Msg("sys: unmounting")
	// Try standard unmount first. MNT_DETACH (lazy unmount) is used to ensure
	// cleanup even if some processes still have open files, which is common
	// during container teardown.
	err := unix.Unmount(target, unix.MNT_DETACH)
	if err == nil {
		return nil
	}

	// Fallback for FUSE mounts in rootless environments if unix.Unmount fails.
	// We use "fusermount -zu" which is the standard unprivileged lazy unmount tool.
	cmd := exec.CommandContext(ctx, "fusermount", "-zu", target)
	if out, errCmd := cmd.CombinedOutput(); errCmd != nil {
		return fmt.Errorf("unmount %s failed: %w (output: %s)", target, errCmd, string(out))
	}

	return nil
}

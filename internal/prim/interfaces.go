package prim

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/rodrigo-baliza/maestro/internal/sys"
)

// FS abstracts filesystem operations used by the Prim storage drivers.
type FS interface {
	MkdirAll(path string, perm os.FileMode) error
	Remove(path string) error
	RemoveAll(path string) error
	Stat(name string) (os.FileInfo, error)
	Rename(oldpath, newpath string) error
	ReadFile(filename string) ([]byte, error)
	WriteFile(filename string, data []byte, perm os.FileMode) error
	Open(name string) (*os.File, error)
	OpenFile(name string, flag int, perm os.FileMode) (*os.File, error)
	Readlink(name string) (string, error)
	Symlink(oldname, newname string) error
	ReadDir(name string) ([]os.DirEntry, error)
	Walk(root string, fn filepath.WalkFunc) error
	WalkDir(root string, fn fs.WalkDirFunc) error
	MkdirTemp(dir, pattern string) (string, error)
	IsNotExist(err error) bool
	Copy(dst io.Writer, src io.Reader) (int64, error)
	FileStat(f *os.File) (os.FileInfo, error)
}

// Mounter abstracts the mount system call.
type Mounter interface {
	Mount(ctx context.Context, source, target, fstype string, flags uintptr, data string) error
	Unmount(ctx context.Context, target string) error
}

// ── Thin Shell Implementations ───────────────────────────────────────────────

type RealFS = sys.RealFS
type RealMounter = sys.RealMounter

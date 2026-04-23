package testutil

import (
	"context"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/rodrigo-baliza/maestro/internal/sys"
)

// File abstracts [os.File] for testing.
type File interface {
	io.ReadWriteCloser
	Name() string
	Stat() (os.FileInfo, error)
	Sync() error
}

// FS abstracts filesystem operations for both production and testing.
type FS interface {
	MkdirAll(path string, perm os.FileMode) error
	MkdirTemp(dir, pattern string) (string, error)
	Remove(path string) error
	RemoveAll(path string) error
	Rename(oldpath, newpath string) error
	Stat(name string) (os.FileInfo, error)
	FileStat(f *os.File) (os.FileInfo, error)
	Open(name string) (*os.File, error)
	OpenFile(name string, flag int, perm os.FileMode) (*os.File, error)
	Create(name string) (*os.File, error)
	CreateTemp(dir, pattern string) (*os.File, error)
	ReadFile(filename string) ([]byte, error)
	WriteFile(filename string, data []byte, perm os.FileMode) error
	Readlink(name string) (string, error)
	Symlink(oldname, newname string) error
	EvalSymlinks(path string) (string, error)
	ReadDir(name string) ([]os.DirEntry, error)
	Walk(root string, fn filepath.WalkFunc) error
	WalkDir(root string, fn fs.WalkDirFunc) error
	IsNotExist(err error) bool
	IsExist(err error) bool
	Chmod(name string, mode os.FileMode) error
	Abs(path string) (string, error)
	Flock(fd int, how int) error
	Copy(dst io.Writer, src io.Reader) (int64, error)
	UserHomeDir() (string, error)
	Getenv(key string) string
}

// MockFS is a highly customizable mock for the FS interface.
type MockFS struct {
	sys.RealFS

	MkdirAllFn     func(string, os.FileMode) error
	MkdirTempFn    func(string, string) (string, error)
	RemoveFn       func(string) error
	RemoveAllFn    func(string) error
	RenameFn       func(string, string) error
	StatFn         func(string) (os.FileInfo, error)
	FileStatFn     func(*os.File) (os.FileInfo, error)
	OpenFn         func(string) (*os.File, error)
	OpenFileFn     func(string, int, os.FileMode) (*os.File, error)
	CreateFn       func(string) (*os.File, error)
	CreateTempFn   func(string, string) (*os.File, error)
	ReadFileFn     func(string) ([]byte, error)
	WriteFileFn    func(string, []byte, os.FileMode) error
	ReadlinkFn     func(string) (string, error)
	SymlinkFn      func(string, string) error
	EvalSymlinksFn func(string) (string, error)
	ReadDirFn      func(string) ([]os.DirEntry, error)
	WalkFn         func(string, filepath.WalkFunc) error
	WalkDirFn      func(string, fs.WalkDirFunc) error
	IsNotExistFn   func(error) bool
	IsExistFn      func(error) bool
	ChmodFn        func(string, os.FileMode) error
	AbsFn          func(string) (string, error)
	FlockFn        func(int, int) error
	CopyFn         func(io.Writer, io.Reader) (int64, error)
	UserHomeDirFn  func() (string, error)
	GetenvFn       func(string) string
}

func (m *MockFS) MkdirAll(p string, mo os.FileMode) error {
	if m.MkdirAllFn != nil {
		return m.MkdirAllFn(p, mo)
	}
	return m.RealFS.MkdirAll(p, mo)
}
func (m *MockFS) MkdirTemp(d, p string) (string, error) {
	if m.MkdirTempFn != nil {
		return m.MkdirTempFn(d, p)
	}
	return m.RealFS.MkdirTemp(d, p)
}
func (m *MockFS) Remove(p string) error {
	if m.RemoveFn != nil {
		return m.RemoveFn(p)
	}
	return m.RealFS.Remove(p)
}
func (m *MockFS) RemoveAll(p string) error {
	if m.RemoveAllFn != nil {
		return m.RemoveAllFn(p)
	}
	return m.RealFS.RemoveAll(p)
}
func (m *MockFS) Rename(o, n string) error {
	if m.RenameFn != nil {
		return m.RenameFn(o, n)
	}
	return m.RealFS.Rename(o, n)
}
func (m *MockFS) Stat(n string) (os.FileInfo, error) {
	if m.StatFn != nil {
		return m.StatFn(n)
	}
	return m.RealFS.Stat(n)
}
func (m *MockFS) FileStat(f *os.File) (os.FileInfo, error) {
	if m.FileStatFn != nil {
		return m.FileStatFn(f)
	}
	return m.RealFS.FileStat(f)
}
func (m *MockFS) Open(n string) (*os.File, error) {
	if m.OpenFn != nil {
		return m.OpenFn(n)
	}
	return m.RealFS.Open(n)
}
func (m *MockFS) OpenFile(n string, f int, mo os.FileMode) (*os.File, error) {
	if m.OpenFileFn != nil {
		return m.OpenFileFn(n, f, mo)
	}
	return m.RealFS.OpenFile(n, f, mo)
}
func (m *MockFS) Create(n string) (*os.File, error) {
	if m.CreateFn != nil {
		return m.CreateFn(n)
	}
	return m.RealFS.Create(n)
}
func (m *MockFS) CreateTemp(d, p string) (*os.File, error) {
	if m.CreateTempFn != nil {
		return m.CreateTempFn(d, p)
	}
	return m.RealFS.CreateTemp(d, p)
}
func (m *MockFS) ReadFile(n string) ([]byte, error) {
	if m.ReadFileFn != nil {
		return m.ReadFileFn(n)
	}
	return m.RealFS.ReadFile(n)
}
func (m *MockFS) WriteFile(n string, d []byte, mo os.FileMode) error {
	if m.WriteFileFn != nil {
		return m.WriteFileFn(n, d, mo)
	}
	return m.RealFS.WriteFile(n, d, mo)
}
func (m *MockFS) Readlink(n string) (string, error) {
	if m.ReadlinkFn != nil {
		return m.ReadlinkFn(n)
	}
	return m.RealFS.Readlink(n)
}
func (m *MockFS) Symlink(o, n string) error {
	if m.SymlinkFn != nil {
		return m.SymlinkFn(o, n)
	}
	return m.RealFS.Symlink(o, n)
}
func (m *MockFS) EvalSymlinks(p string) (string, error) {
	if m.EvalSymlinksFn != nil {
		return m.EvalSymlinksFn(p)
	}
	return m.RealFS.EvalSymlinks(p)
}
func (m *MockFS) ReadDir(n string) ([]os.DirEntry, error) {
	if m.ReadDirFn != nil {
		return m.ReadDirFn(n)
	}
	return m.RealFS.ReadDir(n)
}
func (m *MockFS) Walk(r string, f filepath.WalkFunc) error {
	if m.WalkFn != nil {
		return m.WalkFn(r, f)
	}
	return m.RealFS.Walk(r, f)
}
func (m *MockFS) WalkDir(r string, f fs.WalkDirFunc) error {
	if m.WalkDirFn != nil {
		return m.WalkDirFn(r, f)
	}
	return m.RealFS.WalkDir(r, f)
}
func (m *MockFS) IsNotExist(e error) bool {
	if m.IsNotExistFn != nil {
		return m.IsNotExistFn(e)
	}
	return m.RealFS.IsNotExist(e)
}
func (m *MockFS) IsExist(e error) bool {
	if m.IsExistFn != nil {
		return m.IsExistFn(e)
	}
	return m.RealFS.IsExist(e)
}
func (m *MockFS) Chmod(n string, mo os.FileMode) error {
	if m.ChmodFn != nil {
		return m.ChmodFn(n, mo)
	}
	return m.RealFS.Chmod(n, mo)
}
func (m *MockFS) Abs(p string) (string, error) {
	if m.AbsFn != nil {
		return m.AbsFn(p)
	}
	return m.RealFS.Abs(p)
}
func (m *MockFS) Flock(f int, h int) error {
	if m.FlockFn != nil {
		return m.FlockFn(f, h)
	}
	return m.RealFS.Flock(f, h)
}
func (m *MockFS) Copy(dst io.Writer, src io.Reader) (int64, error) {
	if m.CopyFn != nil {
		return m.CopyFn(dst, src)
	}
	return m.RealFS.Copy(dst, src)
}
func (m *MockFS) UserHomeDir() (string, error) {
	if m.UserHomeDirFn != nil {
		return m.UserHomeDirFn()
	}
	return m.RealFS.UserHomeDir()
}
func (m *MockFS) Getenv(k string) string {
	if m.GetenvFn != nil {
		return m.GetenvFn(k)
	}
	return m.RealFS.Getenv(k)
}

// ── Commander ────────────────────────────────────────────────────────────────

// Commander abstracts os/exec package functions.
type Commander interface {
	CommandContext(ctx context.Context, name string, arg ...string) *exec.Cmd
	Command(name string, arg ...string) *exec.Cmd
	LookPath(file string) (string, error)
}

// MockCommander is a highly customizable mock for the Commander interface.
type MockCommander struct {
	sys.RealCommander

	CommandContextFn func(context.Context, string, ...string) *exec.Cmd
	CommandFn        func(string, ...string) *exec.Cmd
	LookPathFn       func(string) (string, error)

	// Captured data for assertions
	CapturedArgs []string
}

func (m *MockCommander) CommandContext(ctx context.Context, n string, a ...string) *exec.Cmd {
	m.CapturedArgs = append([]string{n}, a...)
	if m.CommandContextFn != nil {
		return m.CommandContextFn(ctx, n, a...)
	}
	return m.RealCommander.CommandContext(ctx, n, a...)
}

func (m *MockCommander) Command(n string, a ...string) *exec.Cmd {
	m.CapturedArgs = append([]string{n}, a...)
	if m.CommandFn != nil {
		return m.CommandFn(n, a...)
	}
	return m.RealCommander.Command(n, a...)
}

func (m *MockCommander) LookPath(f string) (string, error) {
	if m.LookPathFn != nil {
		return m.LookPathFn(f)
	}
	return m.RealCommander.LookPath(f)
}

// ── Mounter ──────────────────────────────────────────────────────────────────

// Mounter abstracts low-level mount operations.
type Mounter interface {
	Mount(ctx context.Context, source, target, fstype string, flags uintptr, data string) error
	Unmount(ctx context.Context, target string) error
}

// MockMounter is a highly customizable mock for the Mounter interface.
type MockMounter struct {
	MountFn   func(context.Context, string, string, string, uintptr, string) error
	UnmountFn func(context.Context, string) error
}

func (m *MockMounter) Mount(ctx context.Context, s, t, f string, fl uintptr, d string) error {
	if m.MountFn != nil {
		return m.MountFn(ctx, s, t, f, fl, d)
	}
	return nil
}

func (m *MockMounter) Unmount(ctx context.Context, t string) error {
	if m.UnmountFn != nil {
		return m.UnmountFn(ctx, t)
	}
	return nil
}

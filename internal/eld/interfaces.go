package eld

import (
	"context"
	"os"
	"os/exec"

	"github.com/garnizeh/maestro/internal/sys"
)

// ── Internal testability interfaces ──────────────────────────────────────────

// FS abstracts several os package functions for the eld package.
type FS interface {
	MkdirAll(path string, perm os.FileMode) error
	OpenFile(name string, flag int, perm os.FileMode) (*os.File, error)
	Open(name string) (*os.File, error)
	CreateTemp(dir, pattern string) (*os.File, error)
	Chmod(name string, mode os.FileMode) error
	Rename(oldpath, newpath string) error
	Remove(name string) error
	Stat(name string) (os.FileInfo, error)
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, d []byte, p os.FileMode) error
	IsNotExist(err error) bool
	Abs(path string) (string, error)
}

// Commander abstracts os/exec package functions.
type Commander interface {
	CommandContext(ctx context.Context, name string, arg ...string) *exec.Cmd
	Command(name string, arg ...string) *exec.Cmd
	LookPath(file string) (string, error)
}

// ── Real implementations (Thin Shells) ───────────────────────────────────────

type RealFS = sys.RealFS
type RealCommander = sys.RealCommander

package maturin

import (
	"io"
	"io/fs"
	"os"

	"github.com/garnizeh/maestro/internal/sys"
	"github.com/garnizeh/maestro/pkg/archive"
)

// FS abstracts several os package functions.
type FS interface {
	MkdirAll(path string, perm os.FileMode) error
	Remove(path string) error
	Rename(oldpath, newpath string) error
	Stat(name string) (os.FileInfo, error)
	Open(name string) (*os.File, error)
	CreateTemp(dir, pattern string) (*os.File, error)
	Readlink(name string) (string, error)
	Symlink(oldname, newname string) error
	OpenFile(name string, flag int, perm os.FileMode) (*os.File, error)
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm os.FileMode) error
	Flock(fd int, how int) error
	Copy(dst io.Writer, src io.Reader) (int64, error)
	WalkDir(root string, fn fs.WalkDirFunc) error
}

// Extractor abstracts layer extraction.
type Extractor interface {
	Extract(r io.Reader, targetDir string, opts archive.ExtractOptions) error
}

// ── Thin Shell Implementations ───────────────────────────────────────────────

type RealFS = sys.RealFS

type realExtractor struct{}

func (realExtractor) Extract(r io.Reader, targetDir string, opts archive.ExtractOptions) error {
	return archive.ExtractTarGz(r, targetDir, opts)
}

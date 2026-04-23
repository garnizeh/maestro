package prim //nolint:testpackage // shared test helpers for prim package

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type mockFS struct {
	mkdirAllErr  error
	removeErr    error
	removeAllErr error
	statRes      os.FileInfo
	statErr      error
	renameErr    error
	readFileRes  []byte
	readFileErr  error
	writeFileErr error
	openRes      *os.File
	openErr      error
	openFileRes  *os.File
	openFileErr  error
	readlinkRes  string
	readlinkErr  error
	symlinkErr   error
	readDirRes   []os.DirEntry
	readDirErr   error
	walkErr      error
	walkDirErr   error
	mkdirTempErr error

	// Callbacks for dynamic behavior
	MkdirAllFn  func(string, os.FileMode) error
	StatFn      func(string) (os.FileInfo, error)
	readMetaFn  func(string) ([]byte, error)
	WriteFileFn func(string, []byte, os.FileMode) error
	WalkFn      func(string, filepath.WalkFunc) error
	WalkDirFn   func(string, fs.WalkDirFunc) error
	OpenFn      func(string) (*os.File, error)
	OpenFileFn  func(string, int, os.FileMode) (*os.File, error)
	MkdirTempFn func(string, string) (string, error)
	ReadFileFn  func(string) ([]byte, error)
	CopyFn      func(io.Writer, io.Reader) (int64, error)
	FileStatFn  func(*os.File) (os.FileInfo, error)

	fallback FS
}

func (m *mockFS) MkdirAll(p string, mode os.FileMode) error {
	if m.MkdirAllFn != nil {
		return m.MkdirAllFn(p, mode)
	}
	if m.mkdirAllErr != nil {
		return m.mkdirAllErr
	}
	if m.fallback != nil {
		return m.fallback.MkdirAll(p, mode)
	}
	return nil
}
func (m *mockFS) Remove(p string) error {
	if m.removeErr != nil {
		return m.removeErr
	}
	if m.fallback != nil {
		return m.fallback.Remove(p)
	}
	return nil
}
func (m *mockFS) RemoveAll(p string) error {
	if m.removeAllErr != nil {
		return m.removeAllErr
	}
	if m.fallback != nil {
		return m.fallback.RemoveAll(p)
	}
	return nil
}
func (m *mockFS) Stat(p string) (os.FileInfo, error) {
	if m.StatFn != nil {
		return m.StatFn(p)
	}
	if m.statErr != nil || m.statRes != nil {
		return m.statRes, m.statErr
	}
	if m.fallback != nil {
		return m.fallback.Stat(p)
	}
	return nil, os.ErrNotExist
}
func (m *mockFS) Rename(o, n string) error {
	if m.renameErr != nil {
		return m.renameErr
	}
	if m.fallback != nil {
		return m.fallback.Rename(o, n)
	}
	return nil
}
func (m *mockFS) ReadFile(f string) ([]byte, error) {
	if m.readMetaFn != nil && filepath.Base(f) == "meta.json" {
		return m.readMetaFn(f)
	}
	if m.ReadFileFn != nil {
		return m.ReadFileFn(f)
	}
	if m.readFileErr != nil || m.readFileRes != nil {
		return m.readFileRes, m.readFileErr
	}
	if m.fallback != nil {
		return m.fallback.ReadFile(f)
	}
	return nil, nil
}
func (m *mockFS) WriteFile(f string, d []byte, mode os.FileMode) error {
	if m.WriteFileFn != nil {
		return m.WriteFileFn(f, d, mode)
	}
	if m.writeFileErr != nil {
		return m.writeFileErr
	}
	if m.fallback != nil {
		return m.fallback.WriteFile(f, d, mode)
	}
	return nil
}
func (m *mockFS) Open(n string) (*os.File, error) {
	if m.OpenFn != nil {
		return m.OpenFn(n)
	}
	if m.openErr != nil || m.openRes != nil {
		return m.openRes, m.openErr
	}
	if m.fallback != nil {
		return m.fallback.Open(n)
	}
	return nil, os.ErrNotExist
}
func (m *mockFS) OpenFile(n string, f int, mode os.FileMode) (*os.File, error) {
	if m.OpenFileFn != nil {
		return m.OpenFileFn(n, f, mode)
	}
	if m.openFileErr != nil || m.openFileRes != nil {
		return m.openFileRes, m.openFileErr
	}
	if m.fallback != nil {
		return m.fallback.OpenFile(n, f, mode)
	}
	return nil, os.ErrNotExist
}
func (m *mockFS) Readlink(n string) (string, error) {
	if m.readlinkErr != nil || m.readlinkRes != "" {
		return m.readlinkRes, m.readlinkErr
	}
	if m.fallback != nil {
		return m.fallback.Readlink(n)
	}
	return "", nil
}
func (m *mockFS) Symlink(o, n string) error {
	if m.symlinkErr != nil {
		return m.symlinkErr
	}
	if m.fallback != nil {
		return m.fallback.Symlink(o, n)
	}
	return nil
}
func (m *mockFS) ReadDir(n string) ([]os.DirEntry, error) {
	if m.readDirErr != nil || m.readDirRes != nil {
		return m.readDirRes, m.readDirErr
	}
	if m.fallback != nil {
		return m.fallback.ReadDir(n)
	}
	return nil, nil
}
func (m *mockFS) Walk(r string, fn filepath.WalkFunc) error {
	if m.WalkFn != nil {
		return m.WalkFn(r, fn)
	}
	if m.walkErr != nil {
		return m.walkErr
	}
	if m.fallback != nil {
		return m.fallback.Walk(r, fn)
	}
	return nil
}
func (m *mockFS) WalkDir(r string, fn fs.WalkDirFunc) error {
	if m.WalkDirFn != nil {
		return m.WalkDirFn(r, fn)
	}
	if m.walkDirErr != nil {
		return m.walkDirErr
	}
	if m.fallback != nil {
		return m.fallback.WalkDir(r, fn)
	}
	return nil
}
func (m *mockFS) MkdirTemp(d, p string) (string, error) {
	if m.MkdirTempFn != nil {
		return m.MkdirTempFn(d, p)
	}
	if m.mkdirTempErr != nil {
		return "", m.mkdirTempErr
	}
	if m.fallback != nil {
		return m.fallback.MkdirTemp(d, p)
	}
	return d, nil
}
func (m *mockFS) IsNotExist(err error) bool { return os.IsNotExist(err) }
func (m *mockFS) Copy(dst io.Writer, src io.Reader) (int64, error) {
	if m.CopyFn != nil {
		return m.CopyFn(dst, src)
	}
	if m.fallback != nil {
		return m.fallback.Copy(dst, src)
	}
	return 0, nil
}
func (m *mockFS) FileStat(f *os.File) (os.FileInfo, error) {
	if m.FileStatFn != nil {
		return m.FileStatFn(f)
	}
	if m.fallback != nil {
		return m.fallback.FileStat(f)
	}
	return nil, os.ErrNotExist
}

type mockMounter struct {
	mountErr   error
	unmountErr error
}

func (m *mockMounter) Mount(_ context.Context, _, _, _ string, _ uintptr, _ string) error {
	return m.mountErr
}

func (m *mockMounter) Unmount(_ context.Context, _ string) error {
	return m.unmountErr
}

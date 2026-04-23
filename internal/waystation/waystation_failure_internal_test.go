package waystation

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"

	"github.com/garnizeh/maestro/internal/sys"
	"github.com/garnizeh/maestro/internal/testutil"
)

// mockTempFile abstracts [os.File] for testing.
type mockTempFile struct {
	writeErr error
	closeErr error
	name     string
}

func (m *mockTempFile) Write(p []byte) (n int, err error) {
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	return len(p), nil
}

func (m *mockTempFile) Close() error {
	return m.closeErr
}

func (m *mockTempFile) Name() string {
	return m.name
}

type mockFS struct {
	testutil.MockFS

	CreateTempFn func(string, string) (TempFile, error)
}

func (m *mockFS) CreateTemp(d, p string) (TempFile, error) {
	if m.CreateTempFn != nil {
		return m.CreateTempFn(d, p)
	}
	return m.MockFS.CreateTemp(d, p)
}

type mockJSON struct {
	realJSON

	marshalFn   func(any) ([]byte, error)
	unmarshalFn func([]byte, any) error
}

func (m *mockJSON) Marshal(v any) ([]byte, error) {
	if m.marshalFn != nil {
		return m.marshalFn(v)
	}
	return m.realJSON.Marshal(v)
}
func (m *mockJSON) Unmarshal(d []byte, v any) error {
	if m.unmarshalFn != nil {
		return m.unmarshalFn(d, v)
	}
	return m.realJSON.Unmarshal(d, v)
}

type mockLocker struct {
	sys.RealFS

	flockFn func(int, int) error
}

func (m *mockLocker) Flock(fd int, how int) error {
	if m.flockFn != nil {
		return m.flockFn(fd, how)
	}
	return m.RealFS.Flock(fd, how)
}

func TestStore_Init_Error(t *testing.T) {
	t.Parallel()
	fs := &mockFS{}
	fs.MkdirAllFn = func(_ string, _ os.FileMode) error {
		return errors.New("mkdir error")
	}
	s := New("/any").WithFS(fs)
	if err := s.Init(); err == nil || err.Error() != "create dir /any: mkdir error" {
		t.Errorf("got error %v, want mkdir error", err)
	}
}

func TestStore_Put_MarshalError(t *testing.T) {
	t.Parallel()
	m := &mockJSON{
		marshalFn: func(_ any) ([]byte, error) {
			return nil, errors.New("marshal error")
		},
	}
	s := New(t.TempDir()).WithMarshaller(m)
	if err := s.Put("c", "k", "v"); err == nil || err.Error() != "marshal: marshal error" {
		t.Errorf("got error %v, want marshal error", err)
	}
}

func TestStore_Put_MkdirError(t *testing.T) {
	t.Parallel()
	fs := &mockFS{}
	fs.MkdirAllFn = func(_ string, _ os.FileMode) error {
		return errors.New("mkdir error")
	}
	s := New(t.TempDir()).WithFS(fs)
	if err := s.Put("c", "k", "v"); err == nil ||
		err.Error() != "create collection dir: mkdir error" {
		t.Errorf("got error %v, want mkdir error", err)
	}
}

func TestStore_Put_CreateTempError(t *testing.T) {
	t.Parallel()
	fs := &mockFS{
		CreateTempFn: func(_, _ string) (TempFile, error) {
			return nil, errors.New("temp error")
		},
	}
	s := New(t.TempDir()).WithFS(fs)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Put("c", "k", "v"); err == nil || err.Error() != "create temp: temp error" {
		t.Errorf("got error %v, want temp error", err)
	}
}

func TestStore_Put_WriteError(t *testing.T) {
	t.Parallel()
	fs := &mockFS{
		CreateTempFn: func(_, _ string) (TempFile, error) {
			return &mockTempFile{writeErr: errors.New("write error"), name: "tmp"}, nil
		},
	}
	s := New(t.TempDir()).WithFS(fs)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	err := s.Put("c", "k", "v")
	if err == nil || err.Error() != "write temp: write error" {
		t.Errorf("got error %v, want write error", err)
	}
}

func TestStore_Put_CloseError(t *testing.T) {
	t.Parallel()
	fs := &mockFS{
		CreateTempFn: func(_, _ string) (TempFile, error) {
			return &mockTempFile{closeErr: errors.New("close error"), name: "tmp"}, nil
		},
	}
	s := New(t.TempDir()).WithFS(fs)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	err := s.Put("c", "k", "v")
	if err == nil || err.Error() != "close temp: close error" {
		t.Errorf("got error %v, want close error", err)
	}
}

func TestStore_Put_RenameError(t *testing.T) {
	t.Parallel()
	fs := &mockFS{}
	fs.RenameFn = func(_, _ string) error {
		return errors.New("rename error")
	}
	s := New(t.TempDir()).WithFS(fs)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := s.Put("c", "k", "v"); err == nil || err.Error() != "rename: rename error" {
		t.Errorf("got error %v, want rename error", err)
	}
}

func TestStore_Get_ReadError(t *testing.T) {
	t.Parallel()
	fs := &mockFS{}
	fs.ReadFileFn = func(_ string) ([]byte, error) {
		return nil, errors.New("read error")
	}
	s := New(t.TempDir()).WithFS(fs)
	err := s.Get("c", "k", nil)
	if err == nil || err.Error() != "read: read error" {
		t.Errorf("got error %v, want read error", err)
	}
}

func TestStore_Get_UnmarshalError(t *testing.T) {
	t.Parallel()
	fs := &mockFS{}
	fs.ReadFileFn = func(_ string) ([]byte, error) {
		return []byte("invalid json"), nil
	}
	m := &mockJSON{
		unmarshalFn: func(_ []byte, _ any) error {
			return errors.New("unmarshal error")
		},
	}
	s := New(t.TempDir()).WithFS(fs).WithMarshaller(m)
	err := s.Get("c", "k", nil)
	if err == nil || err.Error() != "unmarshal: unmarshal error" {
		t.Errorf("got error %v, want unmarshal error", err)
	}
}

func TestStore_Delete_RemoveError(t *testing.T) {
	t.Parallel()
	fs := &mockFS{}
	fs.RemoveFn = func(_ string) error {
		return errors.New("remove error")
	}
	s := New(t.TempDir()).WithFS(fs)
	err := s.Delete("c", "k")
	if err == nil || err.Error() != "delete: remove error" {
		t.Errorf("got error %v, want remove error", err)
	}
}

func TestStore_List_ReadDirError(t *testing.T) {
	t.Parallel()
	fs := &mockFS{}
	fs.ReadDirFn = func(_ string) ([]os.DirEntry, error) {
		return nil, errors.New("readdir error")
	}
	s := New(t.TempDir()).WithFS(fs)
	_, err := s.List("c")
	if err == nil || err.Error() != "list c: readdir error" {
		t.Errorf("got error %v, want readdir error", err)
	}
}

func TestLock_Release_Error(t *testing.T) {
	t.Parallel()
	lck := &mockLocker{
		flockFn: func(_ int, _ int) error {
			return errors.New("flock error")
		},
	}

	l := &Lock{
		f:      os.NewFile(uintptr(syscall.Stdin), "test"),
		locker: lck,
	}
	if err := l.Release(); err == nil || err.Error() != "unlock: flock error" {
		t.Errorf("got error %v, want flock error", err)
	}
}

func TestStore_AcquireLock_FlockError(t *testing.T) {
	t.Parallel()
	lck := &mockLocker{
		flockFn: func(_ int, _ int) error {
			return errors.New("flock error")
		},
	}

	s := New(t.TempDir()).WithLocker(lck)
	_, err := s.AcquireLock(context.Background(), "test")
	if err == nil || err.Error() != "acquire write lock test: flock: flock error" {
		t.Errorf("got error %v, want flock error", err)
	}
}

func TestStore_AcquireReadLock_FlockError(t *testing.T) {
	t.Parallel()
	lck := &mockLocker{
		flockFn: func(_ int, _ int) error {
			return errors.New("flock error")
		},
	}

	s := New(t.TempDir()).WithLocker(lck)
	_, err := s.AcquireReadLock(context.Background(), "test")
	if err == nil || err.Error() != "acquire read lock test: flock: flock error" {
		t.Errorf("got error %v, want flock error", err)
	}
}

func TestStore_CheckAndMigrate_Failures(t *testing.T) {
	t.Parallel()
	s := New(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Newer version
	if err := s.Put("meta", "schema", map[string]int{"version": 99}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	err := s.CheckAndMigrate()
	if err == nil ||
		err.Error() != "state store schema version 99 is newer than this binary (1); upgrade maestro" {
		t.Errorf("got %v, want newer version error", err)
	}

	// Invalid version (negative) triggers migrate default case
	if putErr := s.Put("meta", "schema", map[string]int{"version": -1}); putErr != nil {
		t.Fatalf("Put: %v", putErr)
	}
	err = s.CheckAndMigrate()
	if err == nil || err.Error() != "migrate schema v-1→v0: no migration defined from v-1 to v0" {
		t.Errorf("got %v, want migration error", err)
	}
}

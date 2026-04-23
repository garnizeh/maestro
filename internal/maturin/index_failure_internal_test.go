package maturin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/garnizeh/maestro/internal/testutil"
)

func TestIndex_Lock_Failures(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()

	t.Run("MkdirAllFail", func(t *testing.T) {
		s := New(root)
		s.WithFS(&testutil.MockFS{
			MkdirAllFn: func(_ string, _ os.FileMode) error {
				return errors.New("mkdir-fail")
			},
		})
		_, err := s.lockIndex(ctx)
		if err == nil || err.Error() != "create maturin dir: mkdir-fail" {
			t.Errorf("got error %v, want mkdir-fail", err)
		}
	})

	t.Run("OpenFileFail", func(t *testing.T) {
		s := New(root)
		s.WithFS(&testutil.MockFS{
			OpenFileFn: func(_ string, _ int, _ os.FileMode) (*os.File, error) {
				return nil, errors.New("open-fail")
			},
		})
		_, err := s.lockIndex(ctx)
		if err == nil || err.Error() != "open index lock: open-fail" {
			t.Errorf("got error %v, want open-fail", err)
		}
	})

	t.Run("LockTimeout", func(t *testing.T) {
		s := New(root)
		// Mock flock to always fail with EWOULDBLOCK
		s.WithFS(&testutil.MockFS{
			FlockFn: func(_ int, _ int) error {
				return os.NewSyscallError("flock", errors.New("resource temporarily unavailable"))
			},
		})
		// Fast timeout for test
		shortCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()

		_, err := s.lockIndex(shortCtx)
		if err == nil {
			t.Error("expected timeout error")
		}
	})
}

func TestIndex_ReadWrite_Failures(t *testing.T) {
	root := t.TempDir()

	t.Run("ReadIndexFail", func(t *testing.T) {
		s := New(root)
		s.WithFS(&testutil.MockFS{
			ReadFileFn: func(_ string) ([]byte, error) {
				return nil, errors.New("read-fail")
			},
		})
		_, err := s.readIndex()
		if err == nil || err.Error() != "read index: read-fail" {
			t.Errorf("got error %v, want read-fail", err)
		}
	})

	t.Run("UnmarshalIndexFail", func(t *testing.T) {
		s := New(root)
		s.WithFS(&testutil.MockFS{
			ReadFileFn: func(_ string) ([]byte, error) {
				return []byte("invalid json"), nil
			},
		})
		_, err := s.readIndex()
		if err == nil {
			t.Error("expected unmarshal error")
		}
	})

	t.Run("WriteIndexTempFail", func(t *testing.T) {
		s := New(root)
		s.WithFS(&testutil.MockFS{
			WriteFileFn: func(_ string, _ []byte, _ os.FileMode) error {
				return errors.New("write-fail")
			},
		})
		err := s.writeIndex(v1.Index{})
		if err == nil || err.Error() != "write index temp: write-fail" {
			t.Errorf("got error %v, want write-fail", err)
		}
	})

	t.Run("WriteIndexRenameFail", func(t *testing.T) {
		s := New(root)
		// Ensure maturin dir exists so WriteFileFn doesn't fail early
		if err := os.MkdirAll(filepath.Join(root, "maturin"), 0o700); err != nil {
			t.Fatalf("failed to create maturin dir: %v", err)
		}

		s.WithFS(&testutil.MockFS{
			RenameFn: func(_, _ string) error {
				return errors.New("rename-fail")
			},
		})
		err := s.writeIndex(v1.Index{})
		if err == nil || err.Error() != "commit index: rename-fail" {
			t.Errorf("got error %v, want rename-fail", err)
		}
	})
}

func TestIndex_PublicAPI_Failures(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()

	t.Run("AddToIndex_ReadFail", func(t *testing.T) {
		s := New(root)
		s.WithFS(&testutil.MockFS{
			ReadFileFn: func(_ string) ([]byte, error) {
				return nil, errors.New("read-fail")
			},
		})
		err := s.AddToIndex(ctx, v1.Descriptor{})
		if err == nil || err.Error() != "read index: read-fail" {
			t.Errorf("got %v, want read-fail", err)
		}
	})

	t.Run("RemoveFromIndex_ReadFail", func(t *testing.T) {
		s := New(root)
		s.WithFS(&testutil.MockFS{
			ReadFileFn: func(_ string) ([]byte, error) {
				return nil, errors.New("read-fail")
			},
		})
		err := s.RemoveFromIndex(ctx, digest.FromString("test"))
		if err == nil || err.Error() != "read index: read-fail" {
			t.Errorf("got %v, want read-fail", err)
		}
	})

	t.Run("ListIndex_ReadFail", func(t *testing.T) {
		s := New(root)
		s.WithFS(&testutil.MockFS{
			ReadFileFn: func(_ string) ([]byte, error) {
				return nil, errors.New("read-fail")
			},
		})
		_, err := s.ListIndex(ctx)
		if err == nil || err.Error() != "read index: read-fail" {
			t.Errorf("got %v, want read-fail", err)
		}
	})
}

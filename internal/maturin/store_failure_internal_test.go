package maturin

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/opencontainers/go-digest"

	"github.com/rodrigo-baliza/maestro/internal/testutil"
)

func TestStore_Put_Failures(t *testing.T) {
	content := []byte("test content")
	dgst := digest.FromBytes(content)

	t.Run("MkdirAllFail", func(t *testing.T) {
		s := New(t.TempDir())
		s.WithFS(&testutil.MockFS{
			MkdirAllFn: func(_ string, _ os.FileMode) error {
				return errors.New("mkdir-all-fail")
			},
		})
		err := s.Put(dgst, bytes.NewReader(content))
		if err == nil || err.Error() != "create blob dir: mkdir-all-fail" {
			t.Errorf("got error %v, want mkdir-all-fail", err)
		}
	})

	t.Run("CreateTempFail", func(t *testing.T) {
		s := New(t.TempDir())
		s.WithFS(&testutil.MockFS{
			CreateTempFn: func(_, _ string) (*os.File, error) {
				return nil, errors.New("create-temp-fail")
			},
		})
		err := s.Put(dgst, bytes.NewReader(content))
		if err == nil || err.Error() != "create temp blob: create-temp-fail" {
			t.Errorf("got error %v, want create-temp-fail", err)
		}
	})

	t.Run("CopyIOFail", func(t *testing.T) {
		s := New(t.TempDir())
		s.WithFS(&testutil.MockFS{
			CopyFn: func(_ io.Writer, _ io.Reader) (int64, error) {
				return 0, errors.New("copy-io-fail")
			},
		})
		err := s.Put(dgst, bytes.NewReader(content))
		if err == nil || err.Error() != "write blob: copy-io-fail" {
			t.Errorf("got error %v, want copy-io-fail", err)
		}
	})

	t.Run("RenameFail", func(t *testing.T) {
		s := New(t.TempDir())
		s.WithFS(&testutil.MockFS{
			RenameFn: func(_, _ string) error {
				return errors.New("rename-fail")
			},
		})
		err := s.Put(dgst, bytes.NewReader(content))
		if err == nil || err.Error() != "commit blob "+string(dgst)+": rename-fail" {
			t.Errorf("got error %v, want rename-fail", err)
		}
	})
}

func TestStore_Get_Failures(t *testing.T) {
	dgst := digest.FromString("test")

	t.Run("OpenFail", func(t *testing.T) {
		s := New(t.TempDir())
		s.WithFS(&testutil.MockFS{
			OpenFn: func(_ string) (*os.File, error) {
				return nil, errors.New("open-fail")
			},
		})
		_, err := s.Get(dgst)
		if err == nil || err.Error() != "open blob "+string(dgst)+": open-fail" {
			t.Errorf("got error %v, want open-fail", err)
		}
	})
}

func TestStore_Delete_Failures(t *testing.T) {
	dgst := digest.FromString("test")

	t.Run("RemoveFail", func(t *testing.T) {
		s := New(t.TempDir())
		s.WithFS(&testutil.MockFS{
			RemoveFn: func(_ string) error {
				return errors.New("remove-fail")
			},
		})
		err := s.Delete(dgst)
		if err == nil || err.Error() != "delete blob "+string(dgst)+": remove-fail" {
			t.Errorf("got error %v, want remove-fail", err)
		}
	})
}

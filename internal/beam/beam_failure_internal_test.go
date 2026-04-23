package beam

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/rodrigo-baliza/maestro/internal/testutil"
)

type mockFS = testutil.MockFS

type mockDownloader struct {
	downloadErr error
}

func (m *mockDownloader) DownloadCNIPlugins(_ context.Context, _ string) error {
	return m.downloadErr
}

func TestBeam_LoadDefaultConfig_Failures(t *testing.T) {
	confDir := t.TempDir()
	binDir := t.TempDir()
	netnsDir := t.TempDir()

	t.Run("ReadFileFail", func(t *testing.T) {
		fs := &mockFS{
			ReadFileFn: func(_ string) ([]byte, error) {
				return nil, errors.New("read-fail")
			},
		}
		b := NewBeam(confDir, binDir, netnsDir).WithFS(fs)

		_, err := b.LoadDefaultConfig()
		if err == nil ||
			err.Error() != "failed to read network config "+confDir+"/cni-beam0.conflist: read-fail" {
			t.Errorf("got error %v, want read-fail", err)
		}
	})

	t.Run("MkdirAllFail", func(t *testing.T) {
		fs := &mockFS{
			ReadFileFn: func(_ string) ([]byte, error) {
				return nil, os.ErrNotExist
			},
			MkdirAllFn: func(_ string, _ os.FileMode) error {
				return errors.New("mkdir-fail")
			},
			IsNotExistFn: func(_ error) bool { return true },
		}
		b := NewBeam(confDir, binDir, netnsDir).WithFS(fs)

		_, err := b.LoadDefaultConfig()
		if err == nil || err.Error() != "failed to create network config directory: mkdir-fail" {
			t.Errorf("got error %v, want mkdir-fail", err)
		}
	})

	t.Run("WriteFileFail", func(t *testing.T) {
		fs := &mockFS{
			ReadFileFn: func(_ string) ([]byte, error) {
				return nil, os.ErrNotExist
			},
			WriteFileFn: func(_ string, _ []byte, _ os.FileMode) error {
				return errors.New("write-fail")
			},
			MkdirAllFn:   func(_ string, _ os.FileMode) error { return nil },
			IsNotExistFn: func(_ error) bool { return true },
		}
		b := NewBeam(confDir, binDir, netnsDir).WithFS(fs)

		_, err := b.LoadDefaultConfig()
		if err == nil || err.Error() != "failed to write default CNI config: write-fail" {
			t.Errorf("got error %v, want write-fail", err)
		}
	})
}

func TestBeam_Attach_Failures(t *testing.T) {
	ctx := context.Background()
	b := NewBeam(t.TempDir(), t.TempDir(), t.TempDir())

	t.Run("DownloadFail", func(t *testing.T) {
		dl := &mockDownloader{downloadErr: errors.New("download-fail")}
		b.WithDownloader(dl)

		_, err := b.Attach(ctx, "id", nil, nil)
		if err == nil || err.Error() != "failed to ensure CNI plugins: download-fail" {
			t.Errorf("got error %v, want download-fail", err)
		}
	})
}

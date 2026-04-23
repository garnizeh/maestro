package maturin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"

	"github.com/garnizeh/maestro/internal/testutil"
)

func TestImageInfo_ListImages_Failures(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()

	t.Run("WalkDirFail", func(t *testing.T) {
		s := New(root)
		s.WithFS(&testutil.MockFS{
			WalkDirFn: func(_ string, _ fs.WalkDirFunc) error {
				return errors.New("walk-fail")
			},
		})
		_, err := s.ListImages(ctx)
		if err == nil || err.Error() != "list manifests: walk-fail" {
			t.Errorf("got error %v, want walk-fail", err)
		}
	})

	t.Run("ImageInfoFromTagFail_SilentSkip", func(t *testing.T) {
		// ListImages should skip malformed entries silently (nil error from walk func)
		s := New(root)
		// Provide a WalkDirFn that simulates finding a tag but ResolveTag fails
		s.WithFS(&testutil.MockFS{
			WalkDirFn: func(_ string, fn fs.WalkDirFunc) error {
				// manifestRoot/registry/repo/tag
				tagPath := filepath.Join(s.Root(), "maturin", "manifests", "reg", "repo", "tag")
				return fn(tagPath, &mockDirEntry{name: "tag"}, nil)
			},
			ReadlinkFn: func(_ string) (string, error) {
				return "", errors.New("resolve-fail")
			},
		})
		imgs, err := s.ListImages(ctx)
		if err != nil {
			t.Fatalf("ListImages failed: %v", err)
		}
		if len(imgs) != 0 {
			t.Errorf("expected 0 images, got %d", len(imgs))
		}
	})

	t.Run("WalkDirEntryError", func(t *testing.T) {
		s := New(root)
		s.WithFS(&testutil.MockFS{
			WalkDirFn: func(_ string, fn fs.WalkDirFunc) error {
				return fn("/some/path", nil, errors.New("entry-error"))
			},
		})
		_, err := s.ListImages(ctx)
		if err == nil || !strings.Contains(err.Error(), "entry-error") {
			t.Errorf("got %v, want entry-error", err)
		}
	})

	t.Run("MalformedPath", func(t *testing.T) {
		s := New(root)
		s.WithFS(&testutil.MockFS{
			WalkDirFn: func(path string, fn fs.WalkDirFunc) error {
				// If path is same as manifestsRoot, rel is "." (1 part)
				return fn(path, &mockDirEntry{name: filepath.Base(path)}, nil)
			},
		})
		imgs, err := s.ListImages(ctx)
		if err != nil {
			t.Fatalf("ListImages failed: %v", err)
		}
		if len(imgs) != 0 {
			t.Errorf("expected 0 images for root path, got %d", len(imgs))
		}
	})
}

type mockDirEntry struct {
	fs.DirEntry

	name string
}

func (m *mockDirEntry) Name() string { return m.name }
func (m *mockDirEntry) IsDir() bool  { return false }

func TestImageInfo_InspectImage_Failures(t *testing.T) {
	root := t.TempDir()

	t.Run("ParseRefFail", func(t *testing.T) {
		s := New(root)
		_, err := s.InspectImage("!!invalid!!")
		if err == nil {
			t.Error("expected error on invalid ref")
		}
	})

	t.Run("ResolveRefFail", func(t *testing.T) {
		s := New(root)
		_, err := s.InspectImage("docker.io/library/notfound:latest")
		if err == nil {
			t.Error("expected error on resolve tag fail")
		}
	})

	t.Run("ParseManifestFail", func(t *testing.T) {
		s := New(root)
		dgst := digest.FromString("missing")
		s.WithFS(&testutil.MockFS{
			ReadlinkFn: func(_ string) (string, error) { return string(dgst), nil },
		})
		_, err := s.InspectImage("docker.io/library/test:latest")
		if err == nil {
			t.Error("expected error on InspectImage parse fail")
		}
	})
}

func TestImageInfo_ImageHistory_Failures(t *testing.T) {
	root := t.TempDir()

	t.Run("ParseRefFail", func(t *testing.T) {
		s := New(root)
		_, err := s.ImageHistory("!!invalid!!")
		if err == nil {
			t.Error("expected error on invalid ref")
		}
	})

	t.Run("ResolveRefFail", func(t *testing.T) {
		s := New(root)
		_, err := s.ImageHistory("docker.io/library/notfound:latest")
		if err == nil {
			t.Error("expected error on resolve tag fail")
		}
	})

	t.Run("ParseManifestFail", func(t *testing.T) {
		s := New(root)
		dgst := digest.FromString("missing")
		s.WithFS(&testutil.MockFS{
			ReadlinkFn: func(_ string) (string, error) { return string(dgst), nil },
		})
		_, err := s.ImageHistory("docker.io/library/test:latest")
		if err == nil {
			t.Error("expected error on ImageHistory parse fail")
		}
	})
}

func TestImageInfo_RemoveImage_Failures(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()

	t.Run("ParseRefFail", func(t *testing.T) {
		s := New(root)
		err := s.RemoveImage(ctx, "!!invalid!!")
		if err == nil {
			t.Error("expected error on invalid ref")
		}
	})

	t.Run("NoTagFail", func(t *testing.T) {
		s := New(root)
		err := s.RemoveImage(
			ctx,
			"docker.io/library/test@sha256:6ae8a75555209fd6c44157c0aed8016e763ff435a19cf186f76863140143ff72",
		)
		if err == nil || !strings.Contains(err.Error(), "requires a tag reference") {
			t.Errorf("got error %v", err)
		}
	})

	t.Run("ResolveTagFail", func(t *testing.T) {
		s := New(root)
		err := s.RemoveImage(ctx, "docker.io/library/notfound:latest")
		if err == nil {
			t.Error("expected error on resolve tag fail")
		}
	})

	t.Run("RemoveSymlinkFail", func(t *testing.T) {
		s := New(root)
		dgst := digest.FromString("test")
		s.WithFS(&testutil.MockFS{
			ReadlinkFn: func(_ string) (string, error) { return string(dgst), nil },
			RemoveFn:   func(_ string) error { return errors.New("remove-fail") },
		})
		err := s.RemoveImage(ctx, "docker.io/library/test:latest")
		if err == nil || !strings.Contains(err.Error(), "remove-fail") {
			t.Errorf("got %v, want remove-fail", err)
		}
	})

	t.Run("RemoveFromIndexFail", func(t *testing.T) {
		s := New(root)
		dgst := digest.FromString("test")
		s.WithFS(&testutil.MockFS{
			ReadlinkFn: func(_ string) (string, error) { return string(dgst), nil },
			RemoveFn:   func(_ string) error { return nil },
			ReadFileFn: func(_ string) ([]byte, error) { return nil, errors.New("index-read-fail") },
		})
		err := s.RemoveImage(ctx, "docker.io/library/test:latest")
		if err == nil || !strings.Contains(err.Error(), "index-read-fail") {
			t.Errorf("got %v", err)
		}
	})
}

func TestImageInfo_ReadBlob_Failures(t *testing.T) {
	root := t.TempDir()
	dgst := digest.FromString("test")

	t.Run("ReadBlobFail", func(t *testing.T) {
		s := New(root)
		s.WithFS(&testutil.MockFS{
			OpenFn: func(_ string) (*os.File, error) {
				return nil, errors.New("open-fail")
			},
		})
		_, err := s.readBlob(dgst)
		if err == nil || !strings.Contains(err.Error(), "open-fail") {
			t.Errorf("got error %v, want open-fail", err)
		}
	})
}

func TestImageInfo_ParseManifestAndConfig_ReadManifestFail(t *testing.T) {
	root := t.TempDir()
	dgst := digest.FromString("test")
	s := New(root)
	s.WithFS(&testutil.MockFS{
		OpenFn: func(_ string) (*os.File, error) {
			return nil, errors.New("read-fail")
		},
	})
	_, _, _, _, err := s.parseManifestAndConfig(dgst)
	if err == nil || !strings.Contains(err.Error(), "read manifest") {
		t.Errorf("got error %v, want read manifest error", err)
	}
}

func TestImageInfo_ParseManifestAndConfig_UnmarshalManifestFail(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	content := []byte("invalid json")
	d := digest.FromBytes(content)
	if err := s.Put(d, bytes.NewReader(content)); err != nil {
		t.Fatalf("failed to put manifest: %v", err)
	}
	_, _, _, _, err := s.parseManifestAndConfig(d)
	if err == nil {
		t.Error("expected unmarshal manifest failure")
	}
}

func TestImageInfo_ParseManifestAndConfig_ReadConfigFail(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	cfgDgst := digest.FromString("config")
	mf := ociManifest{}
	mf.Config.Digest = string(cfgDgst)
	mfBytes, err := json.Marshal(mf)
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}
	mfDgst := digest.FromBytes(mfBytes)
	if putErr := s.Put(mfDgst, bytes.NewReader(mfBytes)); putErr != nil {
		t.Fatalf("failed to put manifest: %v", putErr)
	}
	_, _, _, _, err = s.parseManifestAndConfig(mfDgst)
	if err == nil || !strings.Contains(err.Error(), "read config") {
		t.Errorf("got error %v", err)
	}
}

func TestImageInfo_ParseManifestAndConfig_UnmarshalConfigFail(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	badCfgContent := []byte("not json")
	cfgDgst := digest.FromBytes(badCfgContent)
	if putErr := s.Put(cfgDgst, bytes.NewReader(badCfgContent)); putErr != nil {
		t.Fatalf("failed to put config: %v", putErr)
	}
	mf := ociManifest{}
	mf.Config.Digest = string(cfgDgst)
	mfBytes, err := json.Marshal(mf)
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}
	mfDgst := digest.FromBytes(mfBytes)
	if putErr := s.Put(mfDgst, bytes.NewReader(mfBytes)); putErr != nil {
		t.Fatalf("failed to put manifest: %v", putErr)
	}
	_, _, _, _, err = s.parseManifestAndConfig(mfDgst)
	if err == nil {
		t.Error("expected unmarshal config failure")
	}
}

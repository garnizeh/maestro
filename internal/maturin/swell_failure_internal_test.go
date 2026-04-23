package maturin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"

	"github.com/rodrigo-baliza/maestro/internal/prim"
	"github.com/rodrigo-baliza/maestro/pkg/archive"
)

type mockPrim struct {
	prim.Prim

	prepareFn   func(ctx context.Context, key, parent string) ([]prim.Mount, error)
	commitFn    func(ctx context.Context, name, key string) error
	RemoveFn    func(ctx context.Context, key string) error
	walkFn      func(ctx context.Context, fn func(prim.Info) error) error
	writableDir string
	format      archive.WhiteoutFormat
}

func (m *mockPrim) Prepare(ctx context.Context, key, parent string) ([]prim.Mount, error) {
	if m.prepareFn != nil {
		return m.prepareFn(ctx, key, parent)
	}
	return nil, nil
}
func (m *mockPrim) Commit(ctx context.Context, name, key string) error {
	if m.commitFn != nil {
		return m.commitFn(ctx, name, key)
	}
	return nil
}
func (m *mockPrim) Remove(ctx context.Context, key string) error {
	if m.RemoveFn != nil {
		return m.RemoveFn(ctx, key)
	}
	return nil
}
func (m *mockPrim) Walk(ctx context.Context, fn func(prim.Info) error) error {
	if m.walkFn != nil {
		return m.walkFn(ctx, fn)
	}
	return nil
}
func (m *mockPrim) WritableDir(_ string) string            { return m.writableDir }
func (m *mockPrim) WhiteoutFormat() archive.WhiteoutFormat { return m.format }

func TestSwell_Failures(t *testing.T) {
	root := t.TempDir()

	t.Run("ParseReferenceFail", func(t *testing.T) {
		testSwellParseReferenceFail(t, root)
	})

	t.Run("ResolveRefFail", func(t *testing.T) {
		testSwellResolveRefFail(t, root)
	})

	t.Run("ParseManifestFail", func(t *testing.T) {
		testSwellParseManifestFail(t, root)
	})

	t.Run("CheckLayerFail", func(t *testing.T) {
		testSwellCheckLayerFail(t, root)
	})

	t.Run("PrepareFail", func(t *testing.T) {
		testSwellPrepareFail(t, root)
	})

	t.Run("NoMountPoint", func(t *testing.T) {
		testSwellNoMountPoint(t, root)
	})

	t.Run("ExtractLayerFail_GetBlob", func(t *testing.T) {
		testSwellExtractLayerFailGetBlob(t, root)
	})

	t.Run("ExtractLayerFail_Archive", func(t *testing.T) {
		testSwellExtractLayerFailArchive(t, root)
	})

	t.Run("CommitLayerFail", func(t *testing.T) {
		testSwellCommitLayerFail(t, root)
	})
}

func testSwellParseReferenceFail(t *testing.T, root string) {
	ctx := context.Background()
	s := New(root)
	_, err := s.Swell(ctx, "invalid ref!!", nil)
	if err == nil {
		t.Error("expected error on invalid ref")
	}
}

func testSwellResolveRefFail(t *testing.T, root string) {
	ctx := context.Background()
	s := New(root)
	_, err := s.Swell(ctx, "docker.io/library/notfound:latest", nil)
	if err == nil {
		t.Error("expected error on resolve tag fail")
	}
}

func testSwellParseManifestFail(t *testing.T, root string) {
	ctx := context.Background()
	s := New(root)
	reg, repo, tag := "index.docker.io", "library/badman", "latest"
	badContent := []byte("not json")
	badDgst := digest.FromBytes(badContent)
	// Put garbage in the manifest CAS
	if err := s.Put(badDgst, bytes.NewReader(badContent)); err != nil {
		t.Fatalf("failed to put manifest: %v", err)
	}
	if err := s.PutManifest(reg, repo, tag, badDgst, bytes.NewReader(badContent)); err != nil {
		t.Fatalf("failed to put manifest: %v", err)
	}

	_, err := s.Swell(ctx, reg+"/"+repo+":"+tag, nil)
	if err == nil {
		t.Error("expected error on parse manifest fail")
	} else {
		t.Logf("ParseManifestFail got error: %v", err)
	}
}

func testSwellCheckLayerFail(t *testing.T, root string) {
	ctx := context.Background()
	s, _ := setupValidMaturinStore(t, root)
	p := &mockPrim{
		walkFn: func(_ context.Context, _ func(prim.Info) error) error {
			return errors.New("walk-fail")
		},
	}
	ref := "docker.io/library/test:latest"
	_, err := s.Swell(ctx, ref, p)
	if err == nil || err.Error() != "swell: check layer 0: walk-fail" {
		t.Errorf("got error %v, want walk-fail", err)
	}
}

func testSwellPrepareFail(t *testing.T, root string) {
	ctx := context.Background()
	s, _ := setupValidMaturinStore(t, root)
	p := &mockPrim{
		prepareFn: func(_ context.Context, _, _ string) ([]prim.Mount, error) {
			return nil, errors.New("prepare-fail")
		},
	}
	ref := "docker.io/library/test:latest"
	_, err := s.Swell(ctx, ref, p)
	if err == nil || err.Error() != "swell: prepare layer 0: prepare-fail" {
		t.Errorf("got error %v, want prepare-fail", err)
	}
}

func testSwellNoMountPoint(t *testing.T, root string) {
	ctx := context.Background()
	s, _ := setupValidMaturinStore(t, root)
	p := &mockPrim{
		prepareFn: func(_ context.Context, _, _ string) ([]prim.Mount, error) {
			return []prim.Mount{}, nil
		},
	}
	ref := "docker.io/library/test:latest"
	_, err := s.Swell(ctx, ref, p)
	if err == nil || err.Error() != "swell: no mount point for layer 0" {
		t.Errorf("got error %v, want no mount point", err)
	}
}

func testSwellExtractLayerFailGetBlob(t *testing.T, root string) {
	ctx := context.Background()
	s, layerDgst := setupValidMaturinStore(t, root)
	// Corrupt the blob CAS so Get fails or Resolve fails
	if err := s.Delete(layerDgst); err != nil {
		t.Fatalf("failed to delete layer: %v", err)
	}

	p := &mockPrim{
		prepareFn: func(_ context.Context, _, _ string) ([]prim.Mount, error) {
			return []prim.Mount{{Type: "vfs", Source: "/tmp"}}, nil
		},
	}
	ref := "docker.io/library/test:latest"
	_, err := s.Swell(ctx, ref, p)
	if err == nil {
		t.Error("expected error on extract layer (missing blob)")
	}
}

func testSwellExtractLayerFailArchive(t *testing.T, root string) {
	ctx := context.Background()
	s, _ := setupValidMaturinStore(t, root)
	p := &mockPrim{
		prepareFn: func(_ context.Context, _, _ string) ([]prim.Mount, error) {
			return []prim.Mount{{Type: "vfs", Source: "/tmp"}}, nil
		},
	}
	s.WithExtractor(&mockExtractor{
		extractFn: func(_ io.Reader, _ string, _ archive.ExtractOptions) error {
			return errors.New("extract-fail")
		},
	})
	ref := "docker.io/library/test:latest"
	_, err := s.Swell(ctx, ref, p)
	if err == nil || err.Error() != "swell: extract layer 0: extract-fail" {
		t.Errorf("got error %v, want extract-fail", err)
	}
}

func testSwellCommitLayerFail(t *testing.T, root string) {
	ctx := context.Background()
	s, _ := setupValidMaturinStore(t, root)
	s.WithExtractor(&mockExtractor{
		extractFn: func(_ io.Reader, _ string, _ archive.ExtractOptions) error { return nil },
	})
	p := &mockPrim{
		prepareFn: func(_ context.Context, _, _ string) ([]prim.Mount, error) {
			return []prim.Mount{{Type: "vfs", Source: "/tmp"}}, nil
		},
		commitFn: func(_ context.Context, _, _ string) error {
			return errors.New("commit-fail")
		},
	}
	ref := "docker.io/library/test:latest"
	_, err := s.Swell(ctx, ref, p)
	if err == nil || err.Error() != "swell: commit layer 0: commit-fail" {
		t.Errorf("got error %v, want commit-fail", err)
	}
}

func setupValidMaturinStore(t *testing.T, root string) (*Store, digest.Digest) {
	s := New(root)
	layerContent := []byte("dummy layer content")
	layerDgst := digest.FromBytes(layerContent)
	if err := s.Put(layerDgst, bytes.NewReader(layerContent)); err != nil {
		t.Fatalf("s.Put: %v", err)
	}

	cfg := ociConfig{Created: time.Now().UTC()}
	cfgBytes, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	cfgDgst := digest.FromBytes(cfgBytes)
	if putErr := s.Put(cfgDgst, bytes.NewReader(cfgBytes)); putErr != nil {
		t.Fatalf("s.Put: %v", putErr)
	}

	mf := ociManifest{}
	mf.Config.Digest = string(cfgDgst)
	mf.Layers = append(mf.Layers, struct {
		Size   int64  `json:"size"`
		Digest string `json:"digest"`
	}{Size: int64(len(layerContent)), Digest: string(layerDgst)})
	mfBytes, err := json.Marshal(mf)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	mfDgst := digest.FromBytes(mfBytes)
	if putErr := s.Put(mfDgst, bytes.NewReader(mfBytes)); putErr != nil {
		t.Fatalf("s.Put: %v", putErr)
	}

	if putErr := s.PutManifest(
		"index.docker.io", "library/test", "latest", mfDgst, bytes.NewReader(mfBytes),
	); putErr != nil {
		t.Fatalf("s.PutManifest: %v", putErr)
	}
	return s, layerDgst
}

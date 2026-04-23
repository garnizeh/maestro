package maturin //nolint:testpackage // needs internal access for swell testing

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"

	"github.com/garnizeh/maestro/internal/prim"
	"github.com/garnizeh/maestro/pkg/archive"
)

func TestSwell_Success(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s := New(root)
	s.WithExtractor(&mockExtractor{
		extractFn: func(_ io.Reader, _ string, _ archive.ExtractOptions) error {
			return nil
		},
	})
	p, err := prim.NewVFS(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create VFS: %v", err)
	}

	// 1. Create a dummy layer blob
	layerContent := []byte("dummy layer content")
	layerDgst := digest.FromBytes(layerContent)
	if putErr := s.Put(layerDgst, bytes.NewReader(layerContent)); putErr != nil {
		t.Fatalf("Put layer: %v", putErr)
	}

	// 2. Create OCI config
	cfg := ociConfig{
		Created:      time.Now().UTC(),
		Architecture: "amd64",
		Os:           "linux",
	}
	cfgBytes, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	cfgDgst := digest.FromBytes(cfgBytes)
	if putErr := s.Put(cfgDgst, bytes.NewReader(cfgBytes)); putErr != nil {
		t.Fatalf("Put config: %v", putErr)
	}

	// 3. Create OCI manifest
	mf := ociManifest{}
	mf.Config.Digest = string(cfgDgst)
	mf.Config.Size = int64(len(cfgBytes))
	mf.Layers = append(mf.Layers, struct {
		Size   int64  `json:"size"`
		Digest string `json:"digest"`
	}{
		Size:   int64(len(layerContent)),
		Digest: string(layerDgst),
	})
	mfBytes, err := json.Marshal(mf)
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}
	mfDgst := digest.FromBytes(mfBytes)
	if putErr := s.Put(mfDgst, bytes.NewReader(mfBytes)); putErr != nil {
		t.Fatalf("Put manifest: %v", putErr)
	}

	// 4. Create tag symlink
	reg, repo, tag := "index.docker.io", "library/test", "latest"
	if putErr := s.PutManifest(reg, repo, tag, mfDgst, bytes.NewReader(mfBytes)); putErr != nil {
		t.Fatalf("PutManifest: %v", putErr)
	}

	// 5. Run Swell
	// The library might normalize docker.io to index.docker.io
	ref := "docker.io/library/test:latest"
	topKey, err := s.Swell(ctx, ref, p)
	if err != nil {
		t.Fatalf("Swell: %v", err)
	}

	expectedKey := "layer-" + layerDgst.Encoded()
	if topKey != expectedKey {
		t.Errorf("got topKey %q, want %q", topKey, expectedKey)
	}

	// Verify the layer exists in prim via Walk
	found := false
	if walkErr := p.Walk(ctx, func(info prim.Info) error {
		if info.Key == expectedKey {
			found = true
		}
		return nil
	}); walkErr != nil {
		t.Fatalf("Walk: %v", walkErr)
	}
	if !found {
		t.Errorf("layer %q not found in prim after Swell", expectedKey)
	}

	// 6. Run Swell again to test existing layer path (layerExists body)
	topKey2, err := s.Swell(ctx, ref, p)
	if err != nil {
		t.Fatalf("Swell again: %v", err)
	}
	if topKey != topKey2 {
		t.Errorf("expected same topKey on second Swell, got %q vs %q", topKey, topKey2)
	}
}

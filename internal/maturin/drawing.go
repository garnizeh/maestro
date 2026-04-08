package maturin

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	ggcr "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

const defaultParallelism = 4

// RegistryClient is the interface Drawing uses to fetch remote image data.
// [github.com/rodrigo-baliza/maestro/internal/shardik.Client] satisfies this interface.
type RegistryClient interface {
	GetManifest(ctx context.Context, refStr string) (ggcr.Descriptor, error)
	GetImage(ctx context.Context, refStr string) (ggcr.Image, error)
	GetIndex(ctx context.Context, refStr string) (ggcr.ImageIndex, error)
}

// DrawOptions configures a [Store.Draw] pull operation.
type DrawOptions struct {
	Platform    string       // explicit platform override, e.g. "linux/arm64"
	Parallelism int          // max concurrent blob downloads; 0 → 4
	Progress    io.Writer    // receives plain-text download progress lines; nil → [io.Discard]
	OnLayerDone ProgressFunc // optional structured progress callback; nil = no-op
}

// Draw pulls the image identified by refStr from the registry, stores all
// blobs in the local CAS, creates a tag symlink (for tag references), and
// updates the OCI image index.
//
// refStr may be a tag reference ("nginx:latest") or a digest reference
// ("nginx@sha256:…").
func (s *Store) Draw(ctx context.Context, client RegistryClient, refStr string, opts DrawOptions) error {
	if opts.Parallelism <= 0 {
		opts.Parallelism = defaultParallelism
	}
	if opts.Progress == nil {
		opts.Progress = io.Discard
	}

	// Parse the reference first to fail fast on invalid input.
	ref, parseErr := name.ParseReference(refStr)
	if parseErr != nil {
		return fmt.Errorf("parse reference: %w", parseErr)
	}

	// Resolve the top-level manifest descriptor to detect index vs. image.
	topDesc, descErr := client.GetManifest(ctx, refStr)
	if descErr != nil {
		return fmt.Errorf("get manifest: %w", descErr)
	}

	// If the top-level manifest is an image index, run Keystone platform selection.
	imageRef := refStr
	if isIndexMediaType(topDesc.MediaType) {
		resolved, resolveErr := resolveIndexPlatform(ctx, client, refStr, ref.Context(), opts.Platform)
		if resolveErr != nil {
			return resolveErr
		}
		imageRef = resolved
	}

	// Fetch the full image (ggcr is lazy; blobs are not yet downloaded here).
	img, imgErr := client.GetImage(ctx, imageRef)
	if imgErr != nil {
		return fmt.Errorf("get image: %w", imgErr)
	}

	// Download and store all layer blobs in parallel.
	if layerErr := s.storeImageLayers(img, opts); layerErr != nil {
		return layerErr
	}

	// Store config blob, manifest blob, tag symlink, and update the index.
	return s.finalizeImage(ctx, img, ref)
}

// resolveIndexPlatform fetches an image index and runs Keystone platform
// selection. It returns the digest reference string for the selected image.
func resolveIndexPlatform(
	ctx context.Context,
	client RegistryClient,
	refStr string,
	repo name.Repository,
	platformStr string,
) (string, error) {
	idx, idxErr := client.GetIndex(ctx, refStr)
	if idxErr != nil {
		return "", fmt.Errorf("get image index: %w", idxErr)
	}

	want, platErr := parsePlatform(platformStr)
	if platErr != nil {
		return "", platErr
	}

	selected, selectErr := keystoneSelect(idx, want)
	if selectErr != nil {
		return "", selectErr
	}

	return repo.String() + "@" + selected.Digest.String(), nil
}

// isIndexMediaType reports whether mt is an OCI image index or Docker manifest
// list media type.
func isIndexMediaType(mt types.MediaType) bool {
	return mt == types.OCIImageIndex || mt == types.DockerManifestList
}

// syncWriter wraps an [io.Writer] with a mutex so concurrent goroutines can
// safely write progress lines without a data race.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (sw *syncWriter) Write(p []byte) (int, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.w.Write(p)
}

// storeImageLayers downloads and stores all layer blobs for img with
// configurable parallelism.
func (s *Store) storeImageLayers(img ggcr.Image, opts DrawOptions) error {
	layers, layersErr := img.Layers()
	if layersErr != nil {
		return fmt.Errorf("list layers: %w", layersErr)
	}

	sem := make(chan struct{}, opts.Parallelism)
	errs := make([]error, len(layers))
	var wg sync.WaitGroup
	sw := &syncWriter{w: opts.Progress}

	for i, layer := range layers {
		wg.Add(1)
		go func(i int, layer ggcr.Layer) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			errs[i] = s.downloadLayer(layer, sw, opts.OnLayerDone)
		}(i, layer)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			return fmt.Errorf("layer %d: %w", i, err)
		}
	}
	return nil
}

// downloadLayer stores a single layer blob in the CAS. If the blob already
// exists it is skipped (deduplication). onDone is called once the layer is
// processed (nil is a no-op).
func (s *Store) downloadLayer(layer ggcr.Layer, progress io.Writer, onDone ProgressFunc) error {
	start := time.Now()

	h, digestErr := layer.Digest()
	if digestErr != nil {
		return fmt.Errorf("layer digest: %w", digestErr)
	}

	dgst := digest.Digest(h.String())
	shortHex := dgst.Hex()[:12]

	if s.Exists(dgst) {
		_, _ = fmt.Fprintf(progress, "layer %s: already present\n", shortHex)
		if onDone != nil {
			onDone(LayerEvent{Digest: shortHex, Skipped: true, Duration: time.Since(start)})
		}
		return nil
	}

	size, sizeErr := layer.Size()
	if sizeErr != nil {
		size = 0 //coverage:ignore Size() errors are non-fatal; real layers always return a valid size
	}

	rc, compErr := layer.Compressed()
	if compErr != nil {
		return fmt.Errorf("open layer %s: %w", dgst, compErr)
	}
	defer rc.Close()

	if putErr := s.Put(dgst, rc); putErr != nil {
		return fmt.Errorf("store layer %s: %w", dgst, putErr)
	}

	_, _ = fmt.Fprintf(progress, "layer %s: pulled\n", shortHex)
	if onDone != nil {
		onDone(LayerEvent{Digest: shortHex, Skipped: false, Size: size, Duration: time.Since(start)})
	}
	return nil
}

// finalizeImage stores the config blob, stores the manifest and creates a tag
// symlink (for tag references), then updates the OCI image index.
func (s *Store) finalizeImage(ctx context.Context, img ggcr.Image, ref name.Reference) error {
	// Config blob.
	rawConfig, configErr := img.RawConfigFile()
	if configErr != nil {
		return fmt.Errorf("raw config: %w", configErr)
	}

	configName, nameErr := img.ConfigName()
	if nameErr != nil {
		return fmt.Errorf("config name: %w", nameErr)
	}
	configDgst := digest.Digest(configName.String())
	if !s.Exists(configDgst) {
		if putErr := s.Put(configDgst, bytes.NewReader(rawConfig)); putErr != nil {
			return fmt.Errorf("store config: %w", putErr)
		}
	}

	// Manifest blob and tag symlink.
	rawManifest, manErr := img.RawManifest()
	if manErr != nil {
		return fmt.Errorf("raw manifest: %w", manErr)
	}

	imgDigest, dgstErr := img.Digest()
	if dgstErr != nil {
		return fmt.Errorf("image digest: %w", dgstErr)
	}
	manifestDgst := digest.Digest(imgDigest.String())

	registry := ref.Context().RegistryStr()
	repository := ref.Context().RepositoryStr()

	tag, isTag := ref.(name.Tag)
	if isTag {
		if putErr := s.PutManifest(
			registry, repository, tag.TagStr(), manifestDgst, bytes.NewReader(rawManifest),
		); putErr != nil {
			return fmt.Errorf("store manifest: %w", putErr)
		}
	} else if !s.Exists(manifestDgst) {
		if putErr := s.Put(manifestDgst, bytes.NewReader(rawManifest)); putErr != nil {
			return fmt.Errorf("store manifest (digest ref): %w", putErr)
		}
	}

	// Add to local OCI image index.
	mt, mtErr := img.MediaType()
	if mtErr != nil {
		return fmt.Errorf("media type: %w", mtErr)
	}

	desc := v1.Descriptor{
		MediaType: string(mt),
		Digest:    manifestDgst,
		Size:      int64(len(rawManifest)),
	}
	return s.AddToIndex(ctx, desc)
}

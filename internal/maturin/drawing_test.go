package maturin_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	ggcr "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/opencontainers/go-digest"

	"github.com/rodrigo-baliza/maestro/internal/maturin"
)

// ── test constants ────────────────────────────────────────────────────────────

const (
	defaultConfigContent         = `{}`
	defaultManifestContent       = `{"schemaVersion":2}`
	testLayerSize          int64 = 64
)

// hashOf computes the SHA256 ggcr.Hash of content.
func hashOf(content []byte) ggcr.Hash {
	d := digest.SHA256.FromBytes(content)
	return ggcr.Hash{Algorithm: d.Algorithm().String(), Hex: d.Hex()}
}

// ── fakeClient ───────────────────────────────────────────────────────────────

type fakeClient struct {
	manifestDesc ggcr.Descriptor
	manifestErr  error
	image        ggcr.Image
	imageErr     error
	index        ggcr.ImageIndex
	indexErr     error
}

func (fc *fakeClient) GetManifest(_ context.Context, _ string) (ggcr.Descriptor, error) {
	return fc.manifestDesc, fc.manifestErr
}

func (fc *fakeClient) GetImage(_ context.Context, _ string) (ggcr.Image, error) {
	return fc.image, fc.imageErr
}

func (fc *fakeClient) GetIndex(_ context.Context, _ string) (ggcr.ImageIndex, error) {
	return fc.index, fc.indexErr
}

// imageClient returns a fakeClient that reports a single-platform image
// manifest and returns img from GetImage.
func imageClient(img ggcr.Image) *fakeClient {
	return &fakeClient{
		manifestDesc: ggcr.Descriptor{MediaType: types.DockerManifestSchema2},
		image:        img,
	}
}

// ── fakeImage ────────────────────────────────────────────────────────────────

// fakeImage implements ggcr.Image with overridable function fields.
// Default implementations produce self-consistent digest/content pairs so that
// CAS Put calls succeed unless the test explicitly overrides a field to inject
// an error.
type fakeImage struct {
	layersFn      func() ([]ggcr.Layer, error)
	mediaTypeFn   func() (types.MediaType, error)
	rawManifestFn func() ([]byte, error)
	rawConfigFn   func() ([]byte, error)
	digestFn      func() (ggcr.Hash, error)
	configNameFn  func() (ggcr.Hash, error)
}

func (fi *fakeImage) Layers() ([]ggcr.Layer, error) {
	if fi.layersFn != nil {
		return fi.layersFn()
	}
	return nil, nil
}

func (fi *fakeImage) MediaType() (types.MediaType, error) {
	if fi.mediaTypeFn != nil {
		return fi.mediaTypeFn()
	}
	return types.DockerManifestSchema2, nil
}

func (fi *fakeImage) RawManifest() ([]byte, error) {
	if fi.rawManifestFn != nil {
		return fi.rawManifestFn()
	}
	return []byte(defaultManifestContent), nil
}

func (fi *fakeImage) RawConfigFile() ([]byte, error) {
	if fi.rawConfigFn != nil {
		return fi.rawConfigFn()
	}
	return []byte(defaultConfigContent), nil
}

// Digest returns the SHA256 of the manifest content so that Put succeeds.
func (fi *fakeImage) Digest() (ggcr.Hash, error) {
	if fi.digestFn != nil {
		return fi.digestFn()
	}
	return hashOf([]byte(defaultManifestContent)), nil
}

// ConfigName returns the SHA256 of the config content so that Put succeeds.
func (fi *fakeImage) ConfigName() (ggcr.Hash, error) {
	if fi.configNameFn != nil {
		return fi.configNameFn()
	}
	return hashOf([]byte(defaultConfigContent)), nil
}

// Stub methods not used by Drawing.
func (fi *fakeImage) Size() (int64, error) { return 0, nil }
func (fi *fakeImage) ConfigFile() (*ggcr.ConfigFile, error) {
	return nil, nil //nolint:nilnil // stub: never called
}
func (fi *fakeImage) Manifest() (*ggcr.Manifest, error) {
	return nil, nil //nolint:nilnil // stub: never called
}
func (fi *fakeImage) LayerByDigest(ggcr.Hash) (ggcr.Layer, error) {
	return nil, nil //nolint:nilnil // stub: never called
}
func (fi *fakeImage) LayerByDiffID(ggcr.Hash) (ggcr.Layer, error) {
	return nil, nil //nolint:nilnil // stub: never called
}

// ── fakeLayer ────────────────────────────────────────────────────────────────

type fakeLayer struct {
	digestFn     func() (ggcr.Hash, error)
	compressedFn func() (io.ReadCloser, error)
	sizeFn       func() (int64, error)
}

func (fl *fakeLayer) Digest() (ggcr.Hash, error) {
	if fl.digestFn != nil {
		return fl.digestFn()
	}
	return ggcr.Hash{Algorithm: "sha256", Hex: strings.Repeat("c", 64)}, nil
}

func (fl *fakeLayer) Compressed() (io.ReadCloser, error) {
	if fl.compressedFn != nil {
		return fl.compressedFn()
	}
	return io.NopCloser(strings.NewReader("")), nil
}

// Stub methods not used by Drawing.
func (fl *fakeLayer) DiffID() (ggcr.Hash, error) { return ggcr.Hash{}, nil }
func (fl *fakeLayer) Uncompressed() (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (fl *fakeLayer) Size() (int64, error) {
	if fl.sizeFn != nil {
		return fl.sizeFn()
	}
	return 0, nil
}
func (fl *fakeLayer) MediaType() (types.MediaType, error) { return types.DockerLayer, nil }

// ── fakeDrawIndex ─────────────────────────────────────────────────────────────

type fakeDrawIndex struct {
	manifest    *ggcr.IndexManifest
	manifestErr error
}

func (fi *fakeDrawIndex) MediaType() (types.MediaType, error) { return types.OCIImageIndex, nil }
func (fi *fakeDrawIndex) Digest() (ggcr.Hash, error)          { return ggcr.Hash{}, nil }
func (fi *fakeDrawIndex) Size() (int64, error)                { return 0, nil }
func (fi *fakeDrawIndex) RawManifest() ([]byte, error)        { return nil, nil }
func (fi *fakeDrawIndex) Image(ggcr.Hash) (ggcr.Image, error) {
	return nil, nil //nolint:nilnil // stub: never called
}
func (fi *fakeDrawIndex) ImageIndex(ggcr.Hash) (ggcr.ImageIndex, error) {
	return nil, nil //nolint:nilnil // stub: never called
}
func (fi *fakeDrawIndex) IndexManifest() (*ggcr.IndexManifest, error) {
	if fi.manifestErr != nil {
		return nil, fi.manifestErr
	}
	return fi.manifest, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func randomImage(t *testing.T, numLayers int64) ggcr.Image {
	t.Helper()
	img, err := random.Image(testLayerSize, numLayers)
	if err != nil {
		t.Fatalf("random.Image: %v", err)
	}
	return img
}

// ── happy path ────────────────────────────────────────────────────────────────

func TestDraw_SinglePlatformImage_Success(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	img := randomImage(t, 2)

	if err := s.Draw(context.Background(), imageClient(img), "nginx:latest", maturin.DrawOptions{}); err != nil {
		t.Fatalf("Draw: %v", err)
	}

	// Verify all layers are in the CAS.
	layers, _ := img.Layers()
	for _, layer := range layers {
		h, _ := layer.Digest()
		if !s.Exists(digest.Digest(h.String())) {
			t.Errorf("layer %s not in CAS", h)
		}
	}

	// Verify index updated.
	descs, _ := s.ListIndex(context.Background())
	if len(descs) != 1 {
		t.Errorf("expected 1 index entry, got %d", len(descs))
	}
}

func TestDraw_WithProgress(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	img := randomImage(t, 1)
	var buf bytes.Buffer

	if err := s.Draw(
		context.Background(), imageClient(img), "nginx:latest", maturin.DrawOptions{Progress: &buf},
	); err != nil {
		t.Fatalf("Draw: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected progress output, got none")
	}
}

func TestDraw_DefaultParallelism(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	img := randomImage(t, 2)

	if err := s.Draw(
		context.Background(), imageClient(img), "nginx:latest", maturin.DrawOptions{Parallelism: 0},
	); err != nil {
		t.Fatalf("Draw with zero parallelism: %v", err)
	}
}

func TestDraw_Deduplication_SkipsExistingLayers(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	img := randomImage(t, 2)
	client := imageClient(img)

	// First pull stores all blobs.
	if err := s.Draw(context.Background(), client, "nginx:latest", maturin.DrawOptions{}); err != nil {
		t.Fatalf("first Draw: %v", err)
	}

	// Second pull should skip all already-present layers.
	var buf bytes.Buffer
	if err := s.Draw(context.Background(), client, "nginx:latest", maturin.DrawOptions{Progress: &buf}); err != nil {
		t.Fatalf("second Draw: %v", err)
	}
	if !strings.Contains(buf.String(), "already present") {
		t.Error("expected 'already present' in progress on second pull")
	}
}

func TestDraw_DeduplicatesConfig(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	img := randomImage(t, 1)
	client := imageClient(img)

	// First pull stores config.
	if err := s.Draw(context.Background(), client, "nginx:latest", maturin.DrawOptions{}); err != nil {
		t.Fatalf("first Draw: %v", err)
	}

	// Second pull should skip config (already present); no error expected.
	if err := s.Draw(context.Background(), client, "nginx:latest", maturin.DrawOptions{}); err != nil {
		t.Fatalf("second Draw: %v", err)
	}
}

func TestDraw_DigestReference_NoTagSymlink(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	img := randomImage(t, 1)

	imgDigest, _ := img.Digest()
	refStr := "nginx@" + imgDigest.String()

	if err := s.Draw(context.Background(), imageClient(img), refStr, maturin.DrawOptions{}); err != nil {
		t.Fatalf("Draw: %v", err)
	}

	// Manifest blob should be in the CAS.
	if !s.Exists(digest.Digest(imgDigest.String())) {
		t.Error("manifest blob should exist in CAS for digest reference")
	}

	// Verify index updated.
	descs, _ := s.ListIndex(context.Background())
	if len(descs) != 1 {
		t.Errorf("expected 1 index entry, got %d", len(descs))
	}
}

func TestDraw_DigestReference_AlreadyStoredManifest(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	img := randomImage(t, 1)

	imgDigest, _ := img.Digest()
	refStr := "nginx@" + imgDigest.String()

	// First pull stores the manifest.
	if err := s.Draw(context.Background(), imageClient(img), refStr, maturin.DrawOptions{}); err != nil {
		t.Fatalf("first Draw: %v", err)
	}
	// Second pull finds manifest already stored — should succeed without error.
	if err := s.Draw(context.Background(), imageClient(img), refStr, maturin.DrawOptions{}); err != nil {
		t.Fatalf("second Draw: %v", err)
	}
}

func TestDraw_IndexDispatch_OCIImageIndex(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	img := randomImage(t, 1)
	imgDigest, _ := img.Digest()

	idx := &fakeDrawIndex{
		manifest: &ggcr.IndexManifest{
			Manifests: []ggcr.Descriptor{
				{
					MediaType: types.OCIManifestSchema1,
					Digest:    imgDigest,
					Platform:  &ggcr.Platform{OS: "linux", Architecture: "amd64"},
				},
			},
		},
	}
	client := &fakeClient{
		manifestDesc: ggcr.Descriptor{MediaType: types.OCIImageIndex},
		index:        idx,
		image:        img,
	}

	if err := s.Draw(
		context.Background(), client, "nginx:latest", maturin.DrawOptions{Platform: "linux/amd64"},
	); err != nil {
		t.Fatalf("Draw with OCIImageIndex: %v", err)
	}

	descs, _ := s.ListIndex(context.Background())
	if len(descs) != 1 {
		t.Errorf("expected 1 index entry, got %d", len(descs))
	}
}

func TestDraw_IndexDispatch_DockerManifestList(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	img := randomImage(t, 1)
	imgDigest, _ := img.Digest()

	idx := &fakeDrawIndex{
		manifest: &ggcr.IndexManifest{
			Manifests: []ggcr.Descriptor{
				{
					MediaType: types.DockerManifestSchema2,
					Digest:    imgDigest,
					Platform:  &ggcr.Platform{OS: "linux", Architecture: "amd64"},
				},
			},
		},
	}
	client := &fakeClient{
		manifestDesc: ggcr.Descriptor{MediaType: types.DockerManifestList},
		index:        idx,
		image:        img,
	}

	if err := s.Draw(
		context.Background(), client, "nginx:latest", maturin.DrawOptions{Platform: "linux/amd64"},
	); err != nil {
		t.Fatalf("Draw with DockerManifestList: %v", err)
	}
}

// ── error paths ───────────────────────────────────────────────────────────────

func TestDraw_InvalidReference(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	err := s.Draw(context.Background(), &fakeClient{}, "invalid ref!!!", maturin.DrawOptions{})
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestDraw_GetManifestError(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	client := &fakeClient{manifestErr: errors.New("registry down")}

	if err := s.Draw(context.Background(), client, "nginx:latest", maturin.DrawOptions{}); err == nil {
		t.Fatal("expected GetManifest error, got nil")
	}
}

func TestDraw_GetIndexError(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	client := &fakeClient{
		manifestDesc: ggcr.Descriptor{MediaType: types.OCIImageIndex},
		indexErr:     errors.New("index unreachable"),
	}

	if err := s.Draw(context.Background(), client, "nginx:latest", maturin.DrawOptions{}); err == nil {
		t.Fatal("expected GetIndex error, got nil")
	}
}

func TestDraw_PlatformParseError(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	client := &fakeClient{
		manifestDesc: ggcr.Descriptor{MediaType: types.OCIImageIndex},
		index:        &fakeDrawIndex{manifest: &ggcr.IndexManifest{}},
	}

	// "linux" is missing the required arch component.
	if err := s.Draw(context.Background(), client, "nginx:latest", maturin.DrawOptions{Platform: "linux"}); err == nil {
		t.Fatal("expected platform parse error, got nil")
	}
}

func TestDraw_KeystoneNoMatch(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	idx := &fakeDrawIndex{
		manifest: &ggcr.IndexManifest{
			Manifests: []ggcr.Descriptor{
				{
					MediaType: types.OCIManifestSchema1,
					Digest:    ggcr.Hash{Algorithm: "sha256", Hex: strings.Repeat("f", 64)},
					Platform:  &ggcr.Platform{OS: "windows", Architecture: "amd64"},
				},
			},
		},
	}
	client := &fakeClient{
		manifestDesc: ggcr.Descriptor{MediaType: types.OCIImageIndex},
		index:        idx,
	}

	err := s.Draw(context.Background(), client, "nginx:latest", maturin.DrawOptions{Platform: "linux/amd64"})
	if !errors.Is(err, maturin.ErrNoPlatformMatch) {
		t.Fatalf("expected ErrNoPlatformMatch, got %v", err)
	}
}

func TestDraw_KeystoneIndexManifestError(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	client := &fakeClient{
		manifestDesc: ggcr.Descriptor{MediaType: types.OCIImageIndex},
		index:        &fakeDrawIndex{manifestErr: errors.New("manifest error")},
	}

	if err := s.Draw(
		context.Background(), client, "nginx:latest", maturin.DrawOptions{Platform: "linux/amd64"},
	); err == nil {
		t.Fatal("expected IndexManifest error, got nil")
	}
}

func TestDraw_GetImageError(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	client := &fakeClient{
		manifestDesc: ggcr.Descriptor{MediaType: types.DockerManifestSchema2},
		imageErr:     errors.New("image fetch failed"),
	}

	if err := s.Draw(context.Background(), client, "nginx:latest", maturin.DrawOptions{}); err == nil {
		t.Fatal("expected GetImage error, got nil")
	}
}

func TestDraw_LayerListError(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	img := &fakeImage{
		layersFn: func() ([]ggcr.Layer, error) { return nil, errors.New("layers error") },
	}

	if err := s.Draw(context.Background(), imageClient(img), "nginx:latest", maturin.DrawOptions{}); err == nil {
		t.Fatal("expected layer list error, got nil")
	}
}

func TestDraw_LayerDigestError(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	layer := &fakeLayer{
		digestFn: func() (ggcr.Hash, error) { return ggcr.Hash{}, errors.New("digest error") },
	}
	img := &fakeImage{
		layersFn: func() ([]ggcr.Layer, error) { return []ggcr.Layer{layer}, nil },
	}

	if err := s.Draw(context.Background(), imageClient(img), "nginx:latest", maturin.DrawOptions{}); err == nil {
		t.Fatal("expected layer digest error, got nil")
	}
}

func TestDraw_LayerCompressedError(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	layer := &fakeLayer{
		compressedFn: func() (io.ReadCloser, error) { return nil, errors.New("compressed error") },
	}
	img := &fakeImage{
		layersFn: func() ([]ggcr.Layer, error) { return []ggcr.Layer{layer}, nil },
	}

	if err := s.Draw(context.Background(), imageClient(img), "nginx:latest", maturin.DrawOptions{}); err == nil {
		t.Fatal("expected Compressed() error, got nil")
	}
}

func TestDraw_LayerPutError_ReadFails(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	// Reader fails immediately → io.Copy error → Put fails.
	layer := &fakeLayer{
		compressedFn: func() (io.ReadCloser, error) {
			return io.NopCloser(&failReader{err: errors.New("read failed")}), nil
		},
	}
	img := &fakeImage{
		layersFn: func() ([]ggcr.Layer, error) { return []ggcr.Layer{layer}, nil },
	}

	if err := s.Draw(context.Background(), imageClient(img), "nginx:latest", maturin.DrawOptions{}); err == nil {
		t.Fatal("expected layer Put error, got nil")
	}
}

func TestDraw_RawConfigError(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	img := &fakeImage{
		layersFn:    func() ([]ggcr.Layer, error) { return nil, nil },
		rawConfigFn: func() ([]byte, error) { return nil, errors.New("config read error") },
	}

	if err := s.Draw(context.Background(), imageClient(img), "nginx:latest", maturin.DrawOptions{}); err == nil {
		t.Fatal("expected raw config error, got nil")
	}
}

func TestDraw_ConfigNameError(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	img := &fakeImage{
		layersFn:     func() ([]ggcr.Layer, error) { return nil, nil },
		configNameFn: func() (ggcr.Hash, error) { return ggcr.Hash{}, errors.New("config name error") },
	}

	if err := s.Draw(context.Background(), imageClient(img), "nginx:latest", maturin.DrawOptions{}); err == nil {
		t.Fatal("expected config name error, got nil")
	}
}

func TestDraw_ConfigPutError_DigestMismatch(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	// ConfigName returns a digest that doesn't match the default config content `{}`.
	img := &fakeImage{
		layersFn: func() ([]ggcr.Layer, error) { return nil, nil },
		configNameFn: func() (ggcr.Hash, error) {
			return ggcr.Hash{Algorithm: "sha256", Hex: strings.Repeat("9", 64)}, nil
		},
	}

	err := s.Draw(context.Background(), imageClient(img), "nginx:latest", maturin.DrawOptions{})
	if err == nil {
		t.Fatal("expected config Put (digest mismatch) error, got nil")
	}
}

func TestDraw_RawManifestError(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	img := &fakeImage{
		layersFn:      func() ([]ggcr.Layer, error) { return nil, nil },
		rawManifestFn: func() ([]byte, error) { return nil, errors.New("manifest read error") },
	}

	if err := s.Draw(context.Background(), imageClient(img), "nginx:latest", maturin.DrawOptions{}); err == nil {
		t.Fatal("expected raw manifest error, got nil")
	}
}

func TestDraw_ImageDigestError(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	img := &fakeImage{
		layersFn: func() ([]ggcr.Layer, error) { return nil, nil },
		digestFn: func() (ggcr.Hash, error) { return ggcr.Hash{}, errors.New("digest error") },
	}

	if err := s.Draw(context.Background(), imageClient(img), "nginx:latest", maturin.DrawOptions{}); err == nil {
		t.Fatal("expected image digest error, got nil")
	}
}

func TestDraw_PutManifestError_SymlinkBlocked(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	s := maturin.New(root)

	// Block symlink dir creation for index.docker.io by placing a file there.
	if err := os.MkdirAll(filepath.Join(root, "maturin", "manifests"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "maturin", "manifests", "index.docker.io"), []byte("block"), 0o600,
	); err != nil {
		t.Fatal(err)
	}

	img := &fakeImage{layersFn: func() ([]ggcr.Layer, error) { return nil, nil }}
	if err := s.Draw(context.Background(), imageClient(img), "nginx:latest", maturin.DrawOptions{}); err == nil {
		t.Fatal("expected PutManifest symlink error, got nil")
	}
}

func TestDraw_DigestRef_PutError_DigestMismatch(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	// digestFn returns a hash that does NOT match defaultManifestContent.
	// For a digest ref the manifest is stored via s.Put, which checks the hash.
	img := &fakeImage{
		layersFn: func() ([]ggcr.Layer, error) { return nil, nil },
		digestFn: func() (ggcr.Hash, error) {
			return ggcr.Hash{Algorithm: "sha256", Hex: strings.Repeat("e", 64)}, nil
		},
	}
	client := imageClient(img)

	refStr := "nginx@sha256:" + strings.Repeat("e", 64)
	err := s.Draw(context.Background(), client, refStr, maturin.DrawOptions{})
	if err == nil {
		t.Fatal("expected Put error (digest mismatch) for digest ref, got nil")
	}
}

func TestDraw_MediaTypeError(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	img := &fakeImage{
		layersFn:    func() ([]ggcr.Layer, error) { return nil, nil },
		mediaTypeFn: func() (types.MediaType, error) { return "", errors.New("media type error") },
	}

	if err := s.Draw(context.Background(), imageClient(img), "nginx:latest", maturin.DrawOptions{}); err == nil {
		t.Fatal("expected media type error, got nil")
	}
}

func TestDraw_AddToIndexError_LockBlocked(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	s := maturin.New(root)

	// Make .index.lock a directory so OpenFile for the lock fails with EISDIR.
	if err := os.MkdirAll(filepath.Join(root, "maturin", ".index.lock"), 0o700); err != nil {
		t.Fatal(err)
	}

	img := &fakeImage{layersFn: func() ([]ggcr.Layer, error) { return nil, nil }}
	if err := s.Draw(context.Background(), imageClient(img), "nginx:latest", maturin.DrawOptions{}); err == nil {
		t.Fatal("expected AddToIndex error from blocked lock, got nil")
	}
}

func TestDraw_OnLayerDone_CalledForEachLayer(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	img := randomImage(t, 3)

	var mu sync.Mutex
	var events []maturin.LayerEvent
	onDone := func(ev maturin.LayerEvent) {
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	}

	if err := s.Draw(
		context.Background(), imageClient(img), "nginx:latest",
		maturin.DrawOptions{OnLayerDone: onDone},
	); err != nil {
		t.Fatalf("Draw: %v", err)
	}

	mu.Lock()
	got := len(events)
	mu.Unlock()

	if got != 3 {
		t.Errorf("OnLayerDone called %d times, want 3", got)
	}
	for _, ev := range events {
		if ev.Skipped {
			t.Errorf("layer %s: unexpected Skipped=true on first pull", ev.Digest)
		}
		if len(ev.Digest) != 12 {
			t.Errorf("Digest %q: expected 12-char short hex", ev.Digest)
		}
		if ev.Duration < 0 {
			t.Errorf("layer %s: negative Duration %v", ev.Digest, ev.Duration)
		}
	}
}

func TestDraw_OnLayerDone_SkippedOnSecondPull(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	img := randomImage(t, 2)
	client := imageClient(img)

	// First pull — all layers new.
	if err := s.Draw(context.Background(), client, "nginx:latest", maturin.DrawOptions{}); err != nil {
		t.Fatalf("first Draw: %v", err)
	}

	// Second pull — all layers should be skipped.
	var skipped atomic.Int32
	onDone := func(ev maturin.LayerEvent) {
		if ev.Skipped {
			skipped.Add(1)
		}
	}
	if err := s.Draw(
		context.Background(), client, "nginx:latest",
		maturin.DrawOptions{OnLayerDone: onDone},
	); err != nil {
		t.Fatalf("second Draw: %v", err)
	}

	if got := skipped.Load(); got != 2 {
		t.Errorf("skipped = %d, want 2", got)
	}
}

func TestDraw_OnLayerDone_NilCallback_NoPanic(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	img := randomImage(t, 1)

	// nil OnLayerDone must not panic.
	if err := s.Draw(
		context.Background(), imageClient(img), "nginx:latest",
		maturin.DrawOptions{OnLayerDone: nil},
	); err != nil {
		t.Fatalf("Draw: %v", err)
	}
}

func TestDraw_OnLayerDone_SizeError_DefaultsToZero(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	// Layer whose Size() returns an error — download should succeed and
	// LayerEvent.Size must be 0 (the fallback value).
	// Digest must match the actual compressed content (empty → sha256:e3b0c4…).
	emptyDigest := ggcr.Hash{
		Algorithm: "sha256",
		Hex:       "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	}
	layer := &fakeLayer{
		digestFn:     func() (ggcr.Hash, error) { return emptyDigest, nil },
		compressedFn: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("")), nil },
		sizeFn:       func() (int64, error) { return 0, errors.New("size unavailable") },
	}
	img := &fakeImage{
		layersFn: func() ([]ggcr.Layer, error) { return []ggcr.Layer{layer}, nil },
	}

	var events []maturin.LayerEvent
	onDone := func(ev maturin.LayerEvent) { events = append(events, ev) }

	if err := s.Draw(
		context.Background(), imageClient(img), "nginx:latest",
		maturin.DrawOptions{OnLayerDone: onDone},
	); err != nil {
		t.Fatalf("Draw: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 LayerEvent, got %d", len(events))
	}
	if events[0].Size != 0 {
		t.Errorf("Size = %d, want 0 when Size() errors", events[0].Size)
	}
}

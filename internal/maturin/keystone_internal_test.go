package maturin

import (
	"errors"
	"runtime"
	"testing"

	ggcr "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// ── fakeIndex (internal) ─────────────────────────────────────────────────────

type fakeIndexInternal struct {
	manifest    *ggcr.IndexManifest
	manifestErr error
}

func (fi *fakeIndexInternal) MediaType() (types.MediaType, error) { return types.OCIImageIndex, nil }
func (fi *fakeIndexInternal) Digest() (ggcr.Hash, error)          { return ggcr.Hash{}, nil }
func (fi *fakeIndexInternal) Size() (int64, error)                { return 0, nil }
func (fi *fakeIndexInternal) RawManifest() ([]byte, error)        { return nil, nil }
func (fi *fakeIndexInternal) Image(ggcr.Hash) (ggcr.Image, error) {
	return nil, nil //nolint:nilnil // stub: never called
}
func (fi *fakeIndexInternal) ImageIndex(ggcr.Hash) (ggcr.ImageIndex, error) {
	return nil, nil //nolint:nilnil // stub: never called
}
func (fi *fakeIndexInternal) IndexManifest() (*ggcr.IndexManifest, error) {
	if fi.manifestErr != nil {
		return nil, fi.manifestErr
	}
	return fi.manifest, nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func makeInternalDesc(plat *ggcr.Platform, hexID string) ggcr.Descriptor {
	return ggcr.Descriptor{
		MediaType: types.OCIManifestSchema1,
		Digest:    ggcr.Hash{Algorithm: "sha256", Hex: hexID},
		Platform:  plat,
	}
}

func makeInternalIndex(descs []ggcr.Descriptor) *fakeIndexInternal {
	return &fakeIndexInternal{manifest: &ggcr.IndexManifest{Manifests: descs}}
}

// ── parsePlatform ─────────────────────────────────────────────────────────────

func TestParsePlatform_Empty_ReturnsHostPlatform(t *testing.T) {
	t.Parallel()
	p, err := parsePlatform("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.OS != runtime.GOOS {
		t.Errorf("OS = %q, want %q", p.OS, runtime.GOOS)
	}
	if p.Architecture != runtime.GOARCH {
		t.Errorf("Architecture = %q, want %q", p.Architecture, runtime.GOARCH)
	}
}

func TestParsePlatform_OsArch(t *testing.T) {
	t.Parallel()
	p, err := parsePlatform("linux/amd64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.OS != "linux" || p.Architecture != "amd64" || p.Variant != "" {
		t.Errorf("got %+v, want linux/amd64", p)
	}
}

func TestParsePlatform_OsArchVariant(t *testing.T) {
	t.Parallel()
	p, err := parsePlatform("linux/arm/v7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.OS != "linux" || p.Architecture != "arm" || p.Variant != "v7" {
		t.Errorf("got %+v, want linux/arm/v7", p)
	}
}

func TestParsePlatform_Invalid_OnlyOS(t *testing.T) {
	t.Parallel()
	_, err := parsePlatform("linux")
	if err == nil {
		t.Fatal("expected error for os-only platform string")
	}
}

// ── variantSuffix ─────────────────────────────────────────────────────────────

func TestVariantSuffix_Empty(t *testing.T) {
	t.Parallel()
	if got := variantSuffix(""); got != "" {
		t.Errorf("variantSuffix(%q) = %q, want %q", "", got, "")
	}
}

func TestVariantSuffix_NonEmpty(t *testing.T) {
	t.Parallel()
	if got := variantSuffix("v7"); got != "/v7" {
		t.Errorf("variantSuffix(%q) = %q, want %q", "v7", got, "/v7")
	}
}

// ── keystoneSelect ────────────────────────────────────────────────────────────

func TestKeystoneSelect_ExactMatch_OsArchNoVariant(t *testing.T) {
	t.Parallel()
	amd64Desc := makeInternalDesc(&ggcr.Platform{OS: "linux", Architecture: "amd64"}, "aaaa")
	arm64Desc := makeInternalDesc(&ggcr.Platform{OS: "linux", Architecture: "arm64"}, "bbbb")
	idx := makeInternalIndex([]ggcr.Descriptor{amd64Desc, arm64Desc})

	got, err := keystoneSelect(idx, ggcr.Platform{OS: "linux", Architecture: "amd64"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Digest.Hex != "aaaa" {
		t.Errorf("selected %q, want amd64 entry", got.Digest.Hex)
	}
}

func TestKeystoneSelect_ExactMatch_WithVariant(t *testing.T) {
	t.Parallel()
	v6Desc := makeInternalDesc(&ggcr.Platform{OS: "linux", Architecture: "arm", Variant: "v6"}, "v6hex")
	v7Desc := makeInternalDesc(&ggcr.Platform{OS: "linux", Architecture: "arm", Variant: "v7"}, "v7hex")
	idx := makeInternalIndex([]ggcr.Descriptor{v6Desc, v7Desc})

	got, err := keystoneSelect(idx, ggcr.Platform{OS: "linux", Architecture: "arm", Variant: "v7"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Digest.Hex != "v7hex" {
		t.Errorf("selected %q, want v7hex", got.Digest.Hex)
	}
}

func TestKeystoneSelect_FallbackWhenWantHasNoVariant(t *testing.T) {
	t.Parallel()
	// Index only has arm64/v8; want has no variant → fallback accepts any variant.
	idx := makeInternalIndex([]ggcr.Descriptor{
		makeInternalDesc(&ggcr.Platform{OS: "linux", Architecture: "arm64", Variant: "v8"}, "v8hex"),
	})

	got, err := keystoneSelect(idx, ggcr.Platform{OS: "linux", Architecture: "arm64"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Digest.Hex != "v8hex" {
		t.Errorf("selected %q, want v8hex", got.Digest.Hex)
	}
}

func TestKeystoneSelect_FallbackWantVariantNotInIndex(t *testing.T) {
	t.Parallel()
	// Want arm/v9, only arm/v7 available → fallback returns v7.
	idx := makeInternalIndex([]ggcr.Descriptor{
		makeInternalDesc(&ggcr.Platform{OS: "linux", Architecture: "arm", Variant: "v7"}, "v7hex"),
	})

	got, err := keystoneSelect(idx, ggcr.Platform{OS: "linux", Architecture: "arm", Variant: "v9"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Digest.Hex != "v7hex" {
		t.Errorf("selected %q, want v7hex (fallback)", got.Digest.Hex)
	}
}

func TestKeystoneSelect_NoMatch_ReturnsErrNoPlatformMatch(t *testing.T) {
	t.Parallel()
	idx := makeInternalIndex([]ggcr.Descriptor{
		makeInternalDesc(&ggcr.Platform{OS: "windows", Architecture: "amd64"}, "winhex"),
	})

	_, err := keystoneSelect(idx, ggcr.Platform{OS: "linux", Architecture: "amd64"})
	if !errors.Is(err, ErrNoPlatformMatch) {
		t.Fatalf("expected ErrNoPlatformMatch, got %v", err)
	}
}

func TestKeystoneSelect_IndexManifestError(t *testing.T) {
	t.Parallel()
	idx := &fakeIndexInternal{manifestErr: errors.New("fetch failed")}

	_, err := keystoneSelect(idx, ggcr.Platform{OS: "linux", Architecture: "amd64"})
	if err == nil {
		t.Fatal("expected error from IndexManifest")
	}
}

func TestKeystoneSelect_NilPlatformEntriesIgnored(t *testing.T) {
	t.Parallel()
	// Manifests with nil Platform must be skipped.
	nilPlatDesc := ggcr.Descriptor{
		MediaType: types.OCIManifestSchema1,
		Digest:    ggcr.Hash{Algorithm: "sha256", Hex: "nil0"},
	}
	idx := makeInternalIndex([]ggcr.Descriptor{nilPlatDesc})

	_, err := keystoneSelect(idx, ggcr.Platform{OS: "linux", Architecture: "amd64"})
	if !errors.Is(err, ErrNoPlatformMatch) {
		t.Fatalf("expected ErrNoPlatformMatch when all platforms are nil, got %v", err)
	}
}

func TestKeystoneSelect_EmptyIndex(t *testing.T) {
	t.Parallel()
	idx := makeInternalIndex([]ggcr.Descriptor{})

	_, err := keystoneSelect(idx, ggcr.Platform{OS: "linux", Architecture: "amd64"})
	if !errors.Is(err, ErrNoPlatformMatch) {
		t.Fatalf("expected ErrNoPlatformMatch for empty index, got %v", err)
	}
}

func TestKeystoneSelect_FirstExactMatchWins(t *testing.T) {
	t.Parallel()
	first := makeInternalDesc(&ggcr.Platform{OS: "linux", Architecture: "amd64"}, "first")
	second := makeInternalDesc(&ggcr.Platform{OS: "linux", Architecture: "amd64"}, "second")
	idx := makeInternalIndex([]ggcr.Descriptor{first, second})

	got, err := keystoneSelect(idx, ggcr.Platform{OS: "linux", Architecture: "amd64"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Digest.Hex != "first" {
		t.Errorf("selected %q, want first", got.Digest.Hex)
	}
}

func TestKeystoneSelect_ErrorListsAvailablePlatforms(t *testing.T) {
	t.Parallel()
	// arm/v7 and arm (no variant) are available; want linux/amd64 → error lists them.
	idx := makeInternalIndex([]ggcr.Descriptor{
		makeInternalDesc(&ggcr.Platform{OS: "linux", Architecture: "arm", Variant: "v7"}, "v7hex"),
		makeInternalDesc(&ggcr.Platform{OS: "linux", Architecture: "arm"}, "armhex"),
	})

	_, err := keystoneSelect(idx, ggcr.Platform{OS: "linux", Architecture: "amd64"})
	if !errors.Is(err, ErrNoPlatformMatch) {
		t.Fatalf("expected ErrNoPlatformMatch, got %v", err)
	}
	if len(err.Error()) == 0 {
		t.Error("expected non-empty error message")
	}
}

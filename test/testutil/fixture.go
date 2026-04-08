package testutil

import (
	"net/http"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// PushRandomImage pushes a single-layer random image to the registry.
// repo and tag are combined with the registry host, e.g. repo="library/nginx" tag="latest".
// Returns the image digest ("sha256:...").
func PushRandomImage(t *testing.T, reg *Registry, repo, tag string) string {
	t.Helper()

	img, err := random.Image(1024, 2) //nolint:mnd // 1 KB × 2 layers — minimal test fixture
	if err != nil {
		t.Fatalf("random.Image: %v", err)
	}

	refStr := reg.URL + "/" + repo + ":" + tag
	ref, parseErr := name.ParseReference(refStr, name.Insecure)
	if parseErr != nil {
		t.Fatalf("name.ParseReference(%q): %v", refStr, parseErr)
	}

	if writeErr := remote.Write(ref, img, remote.WithTransport(InsecureTransport())); writeErr != nil {
		t.Fatalf("remote.Write: %v", writeErr)
	}

	d, digestErr := img.Digest()
	if digestErr != nil {
		t.Fatalf("img.Digest: %v", digestErr)
	}
	return d.String()
}

// PushMultiPlatformImage pushes an OCI image index containing linux/amd64 and
// linux/arm64 variants to the registry. Returns the index digest ("sha256:...").
func PushMultiPlatformImage(t *testing.T, reg *Registry, repo, tag string) string {
	t.Helper()

	amd64Img, err := random.Image(512, 1) //nolint:mnd // minimal layer
	if err != nil {
		t.Fatalf("random.Image(amd64): %v", err)
	}
	arm64Img, err := random.Image(512, 1) //nolint:mnd // minimal layer
	if err != nil {
		t.Fatalf("random.Image(arm64): %v", err)
	}

	idx := mutate.AppendManifests(empty.Index,
		mutate.IndexAddendum{
			Add: amd64Img,
			Descriptor: v1.Descriptor{
				Platform: &v1.Platform{OS: "linux", Architecture: "amd64"},
			},
		},
		mutate.IndexAddendum{
			Add: arm64Img,
			Descriptor: v1.Descriptor{
				Platform: &v1.Platform{OS: "linux", Architecture: "arm64"},
			},
		},
	)

	refStr := reg.URL + "/" + repo + ":" + tag
	ref, parseErr := name.ParseReference(refStr, name.Insecure)
	if parseErr != nil {
		t.Fatalf("name.ParseReference(%q): %v", refStr, parseErr)
	}

	if writeErr := remote.WriteIndex(ref, idx, remote.WithTransport(InsecureTransport())); writeErr != nil {
		t.Fatalf("remote.WriteIndex: %v", writeErr)
	}

	d, digestErr := idx.Digest()
	if digestErr != nil {
		t.Fatalf("idx.Digest: %v", digestErr)
	}
	return d.String()
}

// InsecureTransport returns an HTTP transport suitable for plain-HTTP test registries.
func InsecureTransport() *http.Transport {
	return &http.Transport{} //nolint:exhaustruct // defaults are fine for plain-HTTP test servers
}

// FakeDigest returns a syntactically valid but nonexistent SHA-256 digest for
// use in tests that need a hash value without pushing real content.
func FakeDigest() v1.Hash {
	h, _ := v1.NewHash("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	return h
}

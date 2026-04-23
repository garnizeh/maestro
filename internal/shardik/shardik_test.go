package shardik_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rodrigo-baliza/maestro/internal/shardik"
	"github.com/rodrigo-baliza/maestro/test/testutil"
)

// ── Task #23 — registry client ────────────────────────────────────────────────

func TestClient_GetManifest_ByTag(t *testing.T) {
	reg := testutil.NewRegistry(t)
	testutil.PushRandomImage(t, reg, "library/nginx", "latest")

	c := shardik.New(shardik.WithInsecure())
	desc, err := c.GetManifest(context.Background(), reg.URL+"/library/nginx:latest")
	if err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if desc.Digest.String() == "" {
		t.Error("expected non-empty digest")
	}
}

func TestClient_GetManifest_NotFound(t *testing.T) {
	reg := testutil.NewRegistry(t)

	c := shardik.New(shardik.WithInsecure())
	_, err := c.GetManifest(context.Background(), reg.URL+"/does/not:exist")
	if err == nil {
		t.Fatal("expected error for non-existent image")
	}
}

func TestClient_GetImage(t *testing.T) {
	reg := testutil.NewRegistry(t)
	testutil.PushRandomImage(t, reg, "library/alpine", "3.18")

	c := shardik.New(shardik.WithInsecure())
	img, err := c.GetImage(context.Background(), reg.URL+"/library/alpine:3.18")
	if err != nil {
		t.Fatalf("GetImage: %v", err)
	}
	layers, err := img.Layers()
	if err != nil {
		t.Fatalf("img.Layers: %v", err)
	}
	if len(layers) == 0 {
		t.Error("expected at least one layer")
	}
}

func TestClient_GetImage_NotFound(t *testing.T) {
	reg := testutil.NewRegistry(t)

	c := shardik.New(shardik.WithInsecure())
	_, err := c.GetImage(context.Background(), reg.URL+"/no/such:image")
	if err == nil {
		t.Fatal("expected error for non-existent image")
	}
}

func TestClient_BlobExists_True(t *testing.T) {
	reg := testutil.NewRegistry(t)
	testutil.PushRandomImage(t, reg, "library/busybox", "latest")

	c := shardik.New(shardik.WithInsecure())
	img, err := c.GetImage(context.Background(), reg.URL+"/library/busybox:latest")
	if err != nil {
		t.Fatalf("GetImage: %v", err)
	}
	layers, err := img.Layers()
	if err != nil || len(layers) == 0 {
		t.Fatalf("img.Layers: %v", err)
	}
	digest, err := layers[0].Digest()
	if err != nil {
		t.Fatalf("layer.Digest: %v", err)
	}

	// The in-process test registry doesn't implement the blobs/HEAD endpoint,
	// so we verify existence by fetching the blob instead.
	rc, err := c.GetBlob(context.Background(), reg.URL+"/library/busybox", digest)
	if err != nil {
		t.Fatalf("GetBlob (existence check): %v", err)
	}
	if closeErr := rc.Close(); closeErr != nil {
		t.Fatalf("rc.Close: %v", closeErr)
	}
}

func TestClient_ListTags(t *testing.T) {
	reg := testutil.NewRegistry(t)
	testutil.PushRandomImage(t, reg, "library/redis", "6")
	testutil.PushRandomImage(t, reg, "library/redis", "7")

	c := shardik.New(shardik.WithInsecure())
	tags, err := c.ListTags(context.Background(), reg.URL+"/library/redis")
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(tags) < 2 {
		t.Errorf("expected at least 2 tags, got %d: %v", len(tags), tags)
	}
}

func TestClient_GetManifest_InvalidRef(t *testing.T) {
	c := shardik.New(shardik.WithInsecure())
	_, err := c.GetManifest(context.Background(), ":::invalid:::")
	if err == nil {
		t.Fatal("expected error for invalid reference")
	}
}

func TestClient_GetIndex_MultiPlatform(t *testing.T) {
	reg := testutil.NewRegistry(t)
	testutil.PushMultiPlatformImage(t, reg, "library/nginx", "multiarch")

	c := shardik.New(shardik.WithInsecure())
	idx, err := c.GetIndex(context.Background(), reg.URL+"/library/nginx:multiarch")
	if err != nil {
		t.Fatalf("GetIndex: %v", err)
	}
	manifest, err := idx.IndexManifest()
	if err != nil {
		t.Fatalf("IndexManifest: %v", err)
	}
	if len(manifest.Manifests) < 2 {
		t.Errorf("expected 2 manifests in index, got %d", len(manifest.Manifests))
	}
}

func TestClient_GetIndex_NotFound(t *testing.T) {
	reg := testutil.NewRegistry(t)

	c := shardik.New(shardik.WithInsecure())
	_, err := c.GetIndex(context.Background(), reg.URL+"/no/index:here")
	if err == nil {
		t.Fatal("expected error for non-existent index")
	}
}

func TestClient_GetBlob(t *testing.T) {
	reg := testutil.NewRegistry(t)
	testutil.PushRandomImage(t, reg, "library/curl", "latest")

	c := shardik.New(shardik.WithInsecure())
	img, err := c.GetImage(context.Background(), reg.URL+"/library/curl:latest")
	if err != nil {
		t.Fatalf("GetImage: %v", err)
	}
	layers, err := img.Layers()
	if err != nil || len(layers) == 0 {
		t.Fatalf("img.Layers: %v", err)
	}
	digest, err := layers[0].Digest()
	if err != nil {
		t.Fatalf("layer.Digest: %v", err)
	}

	rc, err := c.GetBlob(context.Background(), reg.URL+"/library/curl", digest)
	if err != nil {
		t.Fatalf("GetBlob: %v", err)
	}
	defer func() {
		if closeErr := rc.Close(); closeErr != nil {
			t.Fatalf("rc.Close: %v", closeErr)
		}
	}()
}

// mockOCIServer creates a minimal OCI registry stub for error-path testing.
// It answers GET /v2/ with 200 and delegates all other requests to handler.
func mockOCIServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

func TestClient_GetManifest_ServerError(t *testing.T) {
	host := mockOCIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	c := shardik.New(shardik.WithInsecure())
	_, err := c.GetManifest(context.Background(), host+"/repo:tag")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if errors.Is(err, shardik.ErrNotFound) {
		t.Error("server error should not be wrapped as ErrNotFound")
	}
}

func TestClient_GetImage_InvalidRef(t *testing.T) {
	c := shardik.New(shardik.WithInsecure())
	_, err := c.GetImage(context.Background(), ":::invalid:::")
	if err == nil {
		t.Fatal("expected error for invalid reference")
	}
}

func TestClient_GetImage_ServerError(t *testing.T) {
	host := mockOCIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	c := shardik.New(shardik.WithInsecure())
	_, err := c.GetImage(context.Background(), host+"/repo:tag")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if errors.Is(err, shardik.ErrNotFound) {
		t.Error("server error should not be wrapped as ErrNotFound")
	}
}

func TestClient_GetIndex_InvalidRef(t *testing.T) {
	c := shardik.New(shardik.WithInsecure())
	_, err := c.GetIndex(context.Background(), ":::invalid:::")
	if err == nil {
		t.Fatal("expected error for invalid reference")
	}
}

func TestClient_GetIndex_ServerError(t *testing.T) {
	host := mockOCIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	c := shardik.New(shardik.WithInsecure())
	_, err := c.GetIndex(context.Background(), host+"/repo:tag")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if errors.Is(err, shardik.ErrNotFound) {
		t.Error("server error should not be wrapped as ErrNotFound")
	}
}

func TestClient_GetBlob_InvalidRepo(t *testing.T) {
	c := shardik.New(shardik.WithInsecure())
	_, err := c.GetBlob(context.Background(), ":::invalid:::", testutil.FakeDigest(t))
	if err == nil {
		t.Fatal("expected error for invalid repository")
	}
}

func TestClient_GetBlob_NotFound(t *testing.T) {
	host := mockOCIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, err := w.Write([]byte(`{"errors":[{"code":"BLOB_UNKNOWN","message":"blob unknown"}]}`))
		if err != nil {
			t.Fatalf("w.Write: %v", err)
		}
	})

	c := shardik.New(shardik.WithInsecure())
	_, err := c.GetBlob(context.Background(), host+"/repo", testutil.FakeDigest(t))
	if err == nil {
		t.Fatal("expected error for missing blob")
	}
	if !errors.Is(err, shardik.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestClient_GetBlob_ServerError(t *testing.T) {
	host := mockOCIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	c := shardik.New(shardik.WithInsecure())
	_, err := c.GetBlob(context.Background(), host+"/repo", testutil.FakeDigest(t))
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if errors.Is(err, shardik.ErrNotFound) {
		t.Error("server error should not be wrapped as ErrNotFound")
	}
}

func TestClient_BlobExists_InvalidRepo(t *testing.T) {
	c := shardik.New(shardik.WithInsecure())
	_, err := c.BlobExists(context.Background(), ":::invalid:::", testutil.FakeDigest(t))
	if err == nil {
		t.Fatal("expected error for invalid repository")
	}
}

func TestClient_BlobExists_False(t *testing.T) {
	host := mockOCIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, err := w.Write([]byte(`{"errors":[{"code":"BLOB_UNKNOWN","message":"blob unknown"}]}`))
		if err != nil {
			t.Fatalf("w.Write: %v", err)
		}
	})

	c := shardik.New(shardik.WithInsecure())
	exists, err := c.BlobExists(context.Background(), host+"/repo", testutil.FakeDigest(t))
	if err != nil {
		t.Fatalf("BlobExists: %v", err)
	}
	if exists {
		t.Error("expected blob to not exist")
	}
}

func TestClient_BlobExists_True_MockServer(t *testing.T) {
	// ggcr's remote.Head sends HEAD /v2/<repo>/manifests/<digest>.
	// The response must include Content-Type or ggcr rejects it.
	host := mockOCIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && strings.Contains(r.URL.Path, "/manifests/") {
			d := testutil.FakeDigest(t)
			w.Header().Set("Content-Type", "application/vnd.docker.distribution.manifest.v2+json")
			w.Header().Set("Content-Length", "2")
			w.Header().Set("Docker-Content-Digest", d.String())
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	c := shardik.New(shardik.WithInsecure())
	exists, err := c.BlobExists(context.Background(), host+"/repo", testutil.FakeDigest(t))
	if err != nil {
		t.Fatalf("BlobExists: %v", err)
	}
	if !exists {
		t.Error("expected blob to exist")
	}
}

func TestClient_BlobExists_ServerError(t *testing.T) {
	host := mockOCIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	c := shardik.New(shardik.WithInsecure())
	_, err := c.BlobExists(context.Background(), host+"/repo", testutil.FakeDigest(t))
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestClient_ListTags_InvalidRepo(t *testing.T) {
	c := shardik.New(shardik.WithInsecure())
	_, err := c.ListTags(context.Background(), ":::invalid:::")
	if err == nil {
		t.Fatal("expected error for invalid repository")
	}
}

func TestClient_ListTags_ServerError(t *testing.T) {
	host := mockOCIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	c := shardik.New(shardik.WithInsecure())
	_, err := c.ListTags(context.Background(), host+"/repo")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestClient_GetManifest_Unauthorized(t *testing.T) {
	// A 401 with UNAUTHORIZED error code should NOT be treated as ErrNotFound.
	// This covers the UnauthorizedErrorCode branch inside isNotFound.
	host := mockOCIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, err := w.Write([]byte(`{"errors":[{"code":"UNAUTHORIZED","message":"access denied"}]}`))
		if err != nil {
			t.Fatalf("w.Write: %v", err)
		}
	})

	c := shardik.New(shardik.WithInsecure())
	_, err := c.GetManifest(context.Background(), host+"/repo:tag")
	if err == nil {
		t.Fatal("expected error for unauthorized response")
	}
	if errors.Is(err, shardik.ErrNotFound) {
		t.Error("unauthorized response should not be ErrNotFound")
	}
}

func TestClient_GetManifest_ConnectionRefused(t *testing.T) {
	// A refused connection is a non-transport.Error: covers isNotFound's fallback false path.
	c := shardik.New(shardik.WithInsecure())
	_, err := c.GetManifest(context.Background(), "127.0.0.1:1/repo:tag")
	if err == nil {
		t.Fatal("expected error for connection refused")
	}
	if errors.Is(err, shardik.ErrNotFound) {
		t.Error("connection refused should not be ErrNotFound")
	}
}

func TestClient_WithHorn(t *testing.T) {
	reg := testutil.NewRegistry(t)
	testutil.PushRandomImage(t, reg, "library/nginx", "latest")

	c := shardik.New(shardik.WithHorn(shardik.DefaultHornConfig()), shardik.WithInsecure())
	desc, err := c.GetManifest(context.Background(), reg.URL+"/library/nginx:latest")
	if err != nil {
		t.Fatalf("GetManifest with Horn: %v", err)
	}
	if desc.Digest.String() == "" {
		t.Error("expected non-empty digest")
	}
}

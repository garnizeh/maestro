// Package testutil provides shared test helpers for Maestro integration tests.
package testutil

import (
	"net/http/httptest"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
)

// Registry is an in-process OCI registry for use in tests.
type Registry struct {
	// URL is the base URL of the registry (e.g., "127.0.0.1:PORT").
	URL    string
	server *httptest.Server
}

// NewRegistry starts an in-process OCI registry and registers a cleanup
// function to stop it when the test finishes.
func NewRegistry(t *testing.T) *Registry {
	t.Helper()

	srv := httptest.NewServer(registry.New())
	t.Cleanup(srv.Close)

	// Strip the http:// prefix — ggcr name.NewRegistry expects host:port.
	url := srv.URL[len("http://"):]
	return &Registry{URL: url, server: srv}
}

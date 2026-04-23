// Package shardik is Maestro's registry client.
//
// Named after Shardik the Bear from The Dark Tower — the guardian at the Beam's
// endpoint, connecting worlds. Shardik connects Maestro to OCI container registries
// using the OCI Distribution Specification v1.1.0.
//
// Sub-components:
//   - Sigul   — credential resolution chain (auth.json → Docker config → helpers → anonymous)
//   - Horn    — retry with exponential backoff + circuit breaker
//   - Thinny  — mirror/proxy resolution from katet.toml
package shardik

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
)

// Client is a Shardik registry client.
type Client struct {
	keychain authn.Keychain
	opts     []remote.Option
}

// Option configures a [Client].
type Option func(*Client)

// WithKeychain overrides the credential keychain used for authentication.
func WithKeychain(kc authn.Keychain) Option {
	return func(c *Client) { c.keychain = kc }
}

// WithInsecure allows plain-HTTP (non-TLS) registries.
// Use only in tests.
func WithInsecure() Option {
	return func(c *Client) {
		c.opts = append(c.opts, remote.WithTransport(insecureTransport()))
	}
}

// New creates a Shardik client using the Sigul keychain for authentication.
func New(opts ...Option) *Client {
	c := &Client{
		keychain: NewSigulKeychain(SigulConfig{}),
		opts:     nil,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// remoteOptions builds the full option list for a given context.
func (c *Client) remoteOptions(ctx context.Context) []remote.Option {
	opts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(c.keychain),
	}
	return append(opts, c.opts...)
}

// GetManifest fetches the manifest for ref (tag or digest). Returns the
// parsed [v1.Manifest] together with the image/index descriptor.
func (c *Client) GetManifest(ctx context.Context, refStr string) (v1.Descriptor, error) {
	ref, err := name.ParseReference(refStr)
	if err != nil {
		return v1.Descriptor{}, fmt.Errorf("parse reference %q: %w", refStr, err)
	}

	desc, err := remote.Get(ref, c.remoteOptions(ctx)...)
	if err != nil {
		if isNotFound(err) {
			return v1.Descriptor{}, fmt.Errorf("manifest not found: %s: %w", refStr, ErrNotFound)
		}
		return v1.Descriptor{}, fmt.Errorf("get manifest %q: %w", refStr, err)
	}

	return desc.Descriptor, nil
}

// GetImage fetches the full image (manifest + config + layer metadata) for ref.
func (c *Client) GetImage(ctx context.Context, refStr string) (v1.Image, error) {
	ref, err := name.ParseReference(refStr)
	if err != nil {
		return nil, fmt.Errorf("parse reference %q: %w", refStr, err)
	}

	img, err := remote.Image(ref, c.remoteOptions(ctx)...)
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("image not found: %s: %w", refStr, ErrNotFound)
		}
		return nil, fmt.Errorf("get image %q: %w", refStr, err)
	}
	return img, nil
}

// GetIndex fetches an OCI image index (fat manifest) for ref.
func (c *Client) GetIndex(ctx context.Context, refStr string) (v1.ImageIndex, error) {
	ref, err := name.ParseReference(refStr)
	if err != nil {
		return nil, fmt.Errorf("parse reference %q: %w", refStr, err)
	}

	idx, err := remote.Index(ref, c.remoteOptions(ctx)...)
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("index not found: %s: %w", refStr, ErrNotFound)
		}
		return nil, fmt.Errorf("get index %q: %w", refStr, err)
	}
	return idx, nil
}

// GetBlob fetches a blob by its digest from the given repository.
// The caller is responsible for closing the returned [io.ReadCloser].
func (c *Client) GetBlob(
	ctx context.Context,
	repoStr string,
	digest v1.Hash,
) (io.ReadCloser, error) {
	repo, err := name.NewRepository(repoStr)
	if err != nil {
		return nil, fmt.Errorf("parse repository %q: %w", repoStr, err)
	}

	// remote.Layer is lazy — it never makes HTTP requests here; errors surface in Compressed().
	layer, errLayer := remote.Layer(repo.Digest(digest.String()), c.remoteOptions(ctx)...)
	if errLayer != nil {
		return nil, fmt.Errorf("prepare blob %s: %w", digest, errLayer)
	}

	rc, err := layer.Compressed()
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("blob not found: %s: %w", digest, ErrNotFound)
		}
		return nil, fmt.Errorf("open blob %s: %w", digest, err)
	}
	return rc, nil
}

// BlobExists reports whether a blob with the given digest exists in the registry.
func (c *Client) BlobExists(ctx context.Context, repoStr string, digest v1.Hash) (bool, error) {
	repo, err := name.NewRepository(repoStr)
	if err != nil {
		return false, fmt.Errorf("parse repository %q: %w", repoStr, err)
	}

	_, err = remote.Head(repo.Digest(digest.String()), c.remoteOptions(ctx)...)
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("head blob %s: %w", digest, err)
	}
	return true, nil
}

// ListTags lists all tags for the given repository, following pagination.
func (c *Client) ListTags(ctx context.Context, repoStr string) ([]string, error) {
	repo, err := name.NewRepository(repoStr)
	if err != nil {
		return nil, fmt.Errorf("parse repository %q: %w", repoStr, err)
	}

	tags, err := remote.List(repo, c.remoteOptions(ctx)...)
	if err != nil {
		return nil, fmt.Errorf("list tags for %q: %w", repoStr, err)
	}
	return tags, nil
}

// httpNotFound is the HTTP status code for "not found".
const httpNotFound = 404

// ErrNotFound is returned when a registry resource does not exist.
var ErrNotFound = errors.New("not found")

// isNotFound checks whether a registry error represents a 404.
func isNotFound(err error) bool {
	var terr *transport.Error
	if errors.As(err, &terr) {
		for _, e := range terr.Errors {
			if e.Code == transport.UnauthorizedErrorCode {
				return false
			}
		}
		return terr.StatusCode == httpNotFound
	}
	return false
}

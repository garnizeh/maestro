package maturin

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/opencontainers/go-digest"
	"github.com/rs/zerolog/log"
)

// manifestDir returns the symlink directory for a registry/repository pair.
func (s *Store) manifestDir(registry, repository string) string {
	return filepath.Join(s.root, "maturin", "manifests", registry, repository)
}

// tagLinkPath returns the filesystem path of the symlink for a specific tag.
func (s *Store) tagLinkPath(registry, repository, tag string) string {
	return filepath.Join(s.manifestDir(registry, repository), tag)
}

// PutManifest stores a manifest blob in the CAS and creates (or atomically
// replaces) a tag symlink at maturin/manifests/<registry>/<repository>/<tag>.
// The symlink target is the digest string (e.g., "sha256:<hex>").
func (s *Store) PutManifest(
	registry, repository, tag string,
	dgst digest.Digest,
	r io.Reader,
) error {
	if putErr := s.Put(dgst, r); putErr != nil {
		return putErr
	}

	dir := s.manifestDir(registry, repository)
	if mkdirErr := s.fs.MkdirAll(dir, dirPerm); mkdirErr != nil {
		return fmt.Errorf("create manifest dir: %w", mkdirErr)
	}

	return s.atomicSymlink(s.tagLinkPath(registry, repository, tag), string(dgst))
}

// atomicSymlink creates a symlink at path with the given target, atomically
// replacing any existing entry via a temporary name and [os.Rename].
func (s *Store) atomicSymlink(path, target string) error {
	tmp := path + ".tmp"
	if innerErr := s.fs.Remove(tmp); innerErr != nil && !os.IsNotExist(innerErr) {
		log.Debug().Err(innerErr).Str("path", tmp).
			Msg("maturin: failed to remove stale manifest temp file before symlinking")
	}

	if symlinkErr := s.fs.Symlink(target, tmp); symlinkErr != nil {
		return fmt.Errorf("create temp symlink: %w", symlinkErr)
	}

	if renameErr := s.fs.Rename(tmp, path); renameErr != nil {
		if innerErr := s.fs.Remove(tmp); innerErr != nil && !os.IsNotExist(innerErr) {
			log.Debug().Err(innerErr).Str("path", tmp).
				Msg("maturin: failed to remove stale manifest temp file after rename failure")
		}
		return fmt.Errorf("atomically install symlink: %w", renameErr)
	}

	return nil
}

// ResolveTag returns the manifest digest that tag points to in the manifest
// store. Returns [ErrTagNotFound] if no symlink exists for the tag.
func (s *Store) ResolveTag(registry, repository, tag string) (digest.Digest, error) {
	linkPath := s.tagLinkPath(registry, repository, tag)

	target, readErr := s.fs.Readlink(linkPath)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return "", fmt.Errorf("%w: %s/%s:%s",
				ErrTagNotFound, registry, repository, tag)
		}
		return "", fmt.Errorf("resolve tag %s/%s:%s: %w",
			registry, repository, tag, readErr)
	}

	dgst := digest.Digest(target)
	if validateErr := dgst.Validate(); validateErr != nil {
		return "", fmt.Errorf("invalid digest in tag symlink %s/%s:%s: %w",
			registry, repository, tag, validateErr)
	}

	return dgst, nil
}

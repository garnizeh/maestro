package maturin_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"

	"github.com/rodrigo-baliza/maestro/internal/maturin"
)

func TestStore_PutManifest_Success(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	content := []byte(`{"schemaVersion":2}`)
	dgst := mustDigest(content)

	err := s.PutManifest("docker.io", "library/nginx", "latest", dgst, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PutManifest: %v", err)
	}

	// Blob must be in CAS.
	if !s.Exists(dgst) {
		t.Fatal("manifest blob not found in CAS after PutManifest")
	}

	// Symlink must resolve to the digest string.
	linkPath := filepath.Join(
		s.Root(),
		"maturin",
		"manifests",
		"docker.io",
		"library/nginx",
		"latest",
	)
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if target != string(dgst) {
		t.Errorf("symlink target = %q, want %q", target, dgst)
	}
}

func TestStore_PutManifest_MultipleTagsSameDigest(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	content := []byte(`{"schemaVersion":2}`)
	dgst := mustDigest(content)

	if err := s.PutManifest("docker.io", "library/nginx", "latest", dgst, bytes.NewReader(content)); err != nil {
		t.Fatalf("PutManifest latest: %v", err)
	}
	// Second tag: blob already in CAS — Put should still succeed (overwrite is idempotent).
	if err := s.PutManifest("docker.io", "library/nginx", "1.25", dgst, bytes.NewReader(content)); err != nil {
		t.Fatalf("PutManifest 1.25: %v", err)
	}

	dgstLatest, err := s.ResolveTag("docker.io", "library/nginx", "latest")
	if err != nil {
		t.Fatalf("ResolveTag latest: %v", err)
	}
	dgst125, err := s.ResolveTag("docker.io", "library/nginx", "1.25")
	if err != nil {
		t.Fatalf("ResolveTag 1.25: %v", err)
	}
	if dgstLatest != dgst125 {
		t.Errorf("digests differ: latest=%s, 1.25=%s", dgstLatest, dgst125)
	}
}

func TestStore_PutManifest_OverwriteTag(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	contentA := []byte(`{"schemaVersion":2,"tag":"A"}`)
	contentB := []byte(`{"schemaVersion":2,"tag":"B"}`)
	dgstA := mustDigest(contentA)
	dgstB := mustDigest(contentB)

	if err := s.PutManifest("docker.io", "library/nginx", "latest", dgstA, bytes.NewReader(contentA)); err != nil {
		t.Fatalf("PutManifest A: %v", err)
	}
	if err := s.PutManifest("docker.io", "library/nginx", "latest", dgstB, bytes.NewReader(contentB)); err != nil {
		t.Fatalf("PutManifest B: %v", err)
	}

	resolved, err := s.ResolveTag("docker.io", "library/nginx", "latest")
	if err != nil {
		t.Fatalf("ResolveTag: %v", err)
	}
	if resolved != dgstB {
		t.Errorf("resolved = %s, want %s", resolved, dgstB)
	}
}

func TestStore_ResolveTag_Success(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	content := []byte("manifest content")
	dgst := mustDigest(content)

	if err := s.PutManifest("ghcr.io", "myorg/myapp", "v1.0", dgst, bytes.NewReader(content)); err != nil {
		t.Fatalf("PutManifest: %v", err)
	}

	got, err := s.ResolveTag("ghcr.io", "myorg/myapp", "v1.0")
	if err != nil {
		t.Fatalf("ResolveTag: %v", err)
	}
	if got != dgst {
		t.Errorf("resolved digest = %s, want %s", got, dgst)
	}
}

func TestStore_ResolveTag_NotFound(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	_, err := s.ResolveTag("docker.io", "library/alpine", "beta")
	if !errors.Is(err, maturin.ErrTagNotFound) {
		t.Fatalf("expected ErrTagNotFound, got %v", err)
	}
}

func TestStore_ResolveTag_InvalidDigestInSymlink(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	// Manually create a symlink with a garbage target.
	dir := filepath.Join(s.Root(), "maturin", "manifests", "docker.io", "library/nginx")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink("not-a-valid-digest", filepath.Join(dir, "bad")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	_, err := s.ResolveTag("docker.io", "library/nginx", "bad")
	if err == nil {
		t.Fatal("expected error for invalid digest in symlink")
	}
	if errors.Is(err, maturin.ErrTagNotFound) {
		t.Fatal("should not return ErrTagNotFound for present-but-invalid symlink")
	}
}

func TestStore_PutManifest_DigestMismatch(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	content := []byte("real content")
	wrongDigest := mustDigest([]byte("different"))

	err := s.PutManifest(
		"docker.io",
		"library/nginx",
		"latest",
		wrongDigest,
		bytes.NewReader(content),
	)
	if !errors.Is(err, maturin.ErrDigestMismatch) {
		t.Fatalf("expected ErrDigestMismatch, got %v", err)
	}
	// Tag symlink must not have been created.
	_, resolveErr := s.ResolveTag("docker.io", "library/nginx", "latest")
	if !errors.Is(resolveErr, maturin.ErrTagNotFound) {
		t.Fatalf("tag should not exist after failed PutManifest, got: %v", resolveErr)
	}
}

func TestStore_PutManifest_InvalidDigest(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	err := s.PutManifest(
		"docker.io",
		"library/nginx",
		"latest",
		digest.Digest("bad"),
		bytes.NewReader([]byte("x")),
	)
	if err == nil {
		t.Fatal("expected error for invalid digest")
	}
}

// OS error-path tests for manifests.

func TestStore_PutManifest_ManifestDirError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	s := maturin.New(root)

	// Put the blob successfully (uses blobs/sha256 tree, not manifests/).
	content := []byte("manifest content for dir-error")
	dgst := mustDigest(content)
	if err := s.Put(dgst, bytes.NewReader(content)); err != nil {
		t.Fatalf("setup Put: %v", err)
	}

	// Create a FILE at "maturin/manifests" so MkdirAll under it fails.
	if err := os.MkdirAll(filepath.Join(root, "maturin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "maturin", "manifests"), []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := s.PutManifest("docker.io", "library/nginx", "latest", dgst, bytes.NewReader(content))
	if err == nil {
		t.Fatal("expected MkdirAll error, got nil")
	}
}

func TestStore_AtomicSymlink_RenameError(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	content := []byte("symlink rename fail")
	dgst := mustDigest(content)

	// Pre-create a DIRECTORY at the tag symlink path so os.Rename(tmp, dir) fails.
	tagDir := filepath.Join(
		s.Root(),
		"maturin",
		"manifests",
		"docker.io",
		"library/nginx",
		"latest",
	)
	if err := os.MkdirAll(tagDir, 0o700); err != nil {
		t.Fatal(err)
	}

	err := s.PutManifest("docker.io", "library/nginx", "latest", dgst, bytes.NewReader(content))
	if err == nil {
		t.Fatal("expected rename error from atomicSymlink, got nil")
	}
}

func TestStore_AtomicSymlink_SymlinkError(t *testing.T) {
	t.Parallel()
	if os.Getuid() == 0 {
		t.Skip("requires non-root: chmod(0555) does not restrict root")
	}

	s := newTestStore(t)
	content := []byte("symlink create fail")
	dgst := mustDigest(content)

	// Create the manifests dir, then make it read-only so os.Symlink fails.
	tagParent := filepath.Join(s.Root(), "maturin", "manifests", "docker.io", "library/nginx")
	if err := os.MkdirAll(tagParent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(tagParent, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(tagParent, 0o700); err != nil {
			t.Fatalf("failed to restore permissions: %v", err)
		}
	})

	err := s.PutManifest("docker.io", "library/nginx", "latest", dgst, bytes.NewReader(content))
	if err == nil {
		t.Fatal("expected symlink error, got nil")
	}
}

func TestStore_ResolveTag_ReadlinkError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	s := maturin.New(root)

	// Create a FILE at "maturin/manifests" so reading a link under it fails with ENOTDIR.
	if err := os.MkdirAll(filepath.Join(root, "maturin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "maturin", "manifests"), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := s.ResolveTag("docker.io", "library/nginx", "latest")
	if err == nil {
		t.Fatal("expected Readlink error, got nil")
	}
	if errors.Is(err, maturin.ErrTagNotFound) {
		t.Fatal("should not be ErrTagNotFound (path error is not ENOENT)")
	}
}

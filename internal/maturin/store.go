// Package maturin manages local OCI image storage for Maestro.
//
// Named after Maturin the Turtle from The Dark Tower — the ancient world-bearing
// turtle who holds the earth on his shell. Maturin holds every image blob,
// manifest, and tag reference that Maestro stores locally.
//
// Storage layout (under the Waystation root):
//
//	maturin/
//	├── blobs/sha256/  — content-addressable blob store (one file per digest)
//	├── manifests/     — tag symlinks: <registry>/<repo>/<tag> → "sha256:<hex>"
//	└── index.json     — OCI image index tracking all local images
package maturin

import (
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"

	"github.com/opencontainers/go-digest"
	"github.com/rs/zerolog/log"
)

const (
	dirPerm  = 0o700
	filePerm = 0o600
)

// ErrBlobNotFound is returned when a blob digest is absent from the CAS.
var ErrBlobNotFound = errors.New("blob not found")

// ErrDigestMismatch is returned when a blob's SHA256 does not match the
// declared digest — on write (integrity check) or on read (corruption detection).
var ErrDigestMismatch = errors.New("digest mismatch")

// ErrTagNotFound is returned when a tag has no symlink in the manifest store.
var ErrTagNotFound = errors.New("tag not found")

// Store is the Maturin content-addressable image store.
// All operations are rooted under root (the Waystation root directory,
// e.g., ~/.local/share/maestro).
type Store struct {
	root string

	fs        FS
	extractor Extractor
}

// New returns a [Store] rooted at root.
func New(root string) *Store {
	return &Store{
		root:      root,
		fs:        RealFS{},
		extractor: realExtractor{},
	}
}

// WithFS sets a custom filesystem implementation.
func (s *Store) WithFS(f FS) *Store {
	s.fs = f
	return s
}

// WithExtractor sets a custom layer extractor implementation.
func (s *Store) WithExtractor(e Extractor) *Store {
	s.extractor = e
	return s
}

// Root returns the store root directory path.
func (s *Store) Root() string { return s.root }

// blobDir returns the CAS directory for SHA256 blobs.
func (s *Store) blobDir() string {
	return filepath.Join(s.root, "maturin", "blobs", "sha256")
}

// blobPath returns the filesystem path for the blob identified by hexDigest.
func (s *Store) blobPath(hexDigest string) string {
	return filepath.Join(s.blobDir(), hexDigest)
}

// Put stores the content of r in the CAS keyed by dgst.
// The SHA256 of the content is verified against dgst before the file is committed.
// If the digests do not match, no file is persisted and [ErrDigestMismatch] is returned.
// Parent directories are created on demand.
func (s *Store) Put(dgst digest.Digest, r io.Reader) error {
	if validateErr := dgst.Validate(); validateErr != nil {
		return fmt.Errorf("invalid digest: %w", validateErr)
	}

	dir := s.blobDir()
	if mkdirErr := s.fs.MkdirAll(dir, dirPerm); mkdirErr != nil {
		return fmt.Errorf("create blob dir: %w", mkdirErr)
	}

	tmp, openErr := s.fs.CreateTemp(dir, ".tmp-blob-")
	if openErr != nil {
		//coverage:ignore non-writable dir requires root check; covered by TestStore_Put_CreateTempError when run as non-root
		return fmt.Errorf("create temp blob: %w", openErr)
	}

	tmpPath, writeErr := s.writeAndVerify(tmp, dgst, r)
	if writeErr != nil {
		if errRem := s.fs.Remove(tmpPath); errRem != nil && !os.IsNotExist(errRem) {
			log.Debug().
				Err(errRem).
				Str("path", tmpPath).
				Msg("maturin: failed to remove temp blob after write error")
		}
		return writeErr
	}

	dest := s.blobPath(dgst.Hex())
	if renameErr := s.fs.Rename(tmpPath, dest); renameErr != nil {
		if errRem := s.fs.Remove(tmpPath); errRem != nil && !os.IsNotExist(errRem) {
			log.Debug().
				Err(errRem).
				Str("path", tmpPath).
				Msg("maturin: failed to remove temp blob after rename failure")
		}
		return fmt.Errorf("commit blob %s: %w", dgst, renameErr)
	}

	log.Debug().Str("dgst", dgst.String()).Msg("maturin: blob stored")

	return nil
}

// Get returns a reader for the blob at dgst. The reader verifies the SHA256
// digest on [io.EOF]; if the blob is corrupted, it returns [ErrDigestMismatch].
// Returns [ErrBlobNotFound] if no blob with that digest is present.
func (s *Store) Get(dgst digest.Digest) (io.ReadCloser, error) {
	if validateErr := dgst.Validate(); validateErr != nil {
		return nil, fmt.Errorf("invalid digest: %w", validateErr)
	}

	f, openErr := s.fs.Open(s.blobPath(dgst.Hex()))
	if openErr != nil {
		if os.IsNotExist(openErr) {
			return nil, fmt.Errorf("%w: %s", ErrBlobNotFound, dgst)
		}
		return nil, fmt.Errorf("open blob %s: %w", dgst, openErr)
	}

	log.Debug().Str("dgst", dgst.String()).Msg("maturin: blob opened for reading")

	return &verifyingReader{r: f, h: dgst.Algorithm().Hash(), expected: dgst}, nil
}

// Exists reports whether the CAS contains a blob with the given digest.
func (s *Store) Exists(dgst digest.Digest) bool {
	_, err := s.fs.Stat(s.blobPath(dgst.Hex()))
	return err == nil
}

// Delete removes the blob with the given digest from the CAS.
// Returns [ErrBlobNotFound] if no such blob exists.
func (s *Store) Delete(dgst digest.Digest) error {
	if removeErr := s.fs.Remove(s.blobPath(dgst.Hex())); removeErr != nil {
		if os.IsNotExist(removeErr) {
			return fmt.Errorf("%w: %s", ErrBlobNotFound, dgst)
		}
		return fmt.Errorf("delete blob %s: %w", dgst, removeErr)
	}
	return nil
}

// writeAndVerify writes r to f while hashing, verifies the digest, closes f,
// and returns the path of the closed temp file. On error the file is not
// removed — the caller is responsible for cleanup.
func (s *Store) writeAndVerify(f *os.File, dgst digest.Digest, r io.Reader) (string, error) {
	h := dgst.Algorithm().Hash()
	_, copyErr := s.fs.Copy(io.MultiWriter(f, h), r)
	closeErr := f.Close()

	if copyErr != nil {
		return f.Name(), fmt.Errorf("write blob: %w", copyErr)
	}
	if closeErr != nil {
		return f.Name(), fmt.Errorf(
			"close blob: %w",
			closeErr,
		) //coverage:ignore unreachable after successful Write
	}

	actual := digest.Digest(string(dgst.Algorithm()) + ":" + hex.EncodeToString(h.Sum(nil)))
	if actual != dgst {
		return f.Name(), fmt.Errorf("%w: expected %s, got %s", ErrDigestMismatch, dgst, actual)
	}

	return f.Name(), nil
}

// verifyingReader wraps an [io.ReadCloser] and verifies the SHA256 digest on
// [io.EOF]. If the blob is corrupted, Read returns [ErrDigestMismatch].
type verifyingReader struct {
	r        io.ReadCloser
	h        hash.Hash
	expected digest.Digest
}

func (v *verifyingReader) Read(p []byte) (int, error) {
	n, readErr := v.r.Read(p)
	if n > 0 {
		_, err := v.h.Write(p[:n])
		if err != nil {
			return n, fmt.Errorf("hash write: %w", err)
		}
	}
	if errors.Is(readErr, io.EOF) {
		actual := digest.Digest(
			string(v.expected.Algorithm()) + ":" + hex.EncodeToString(v.h.Sum(nil)),
		)
		if actual != v.expected {
			return n, fmt.Errorf("%w: expected %s, got %s", ErrDigestMismatch, v.expected, actual)
		}
	}
	return n, readErr
}

func (v *verifyingReader) Close() error {
	return v.r.Close()
}

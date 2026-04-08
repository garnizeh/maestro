package maturin_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"

	"github.com/rodrigo-baliza/maestro/internal/maturin"
)

// newTestStore returns a Store backed by a temporary directory.
func newTestStore(t *testing.T) *maturin.Store {
	t.Helper()
	return maturin.New(t.TempDir())
}

// mustDigest computes the SHA256 digest of content.
func mustDigest(content []byte) digest.Digest {
	return digest.SHA256.FromBytes(content)
}

func TestNew(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	s := maturin.New(root)
	if s.Root() != root {
		t.Errorf("Root() = %q, want %q", s.Root(), root)
	}
}

func TestStore_Put_Success(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	content := []byte("hello maturin")
	dgst := mustDigest(content)

	if err := s.Put(dgst, bytes.NewReader(content)); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if !s.Exists(dgst) {
		t.Fatal("Exists returned false after Put")
	}
}

func TestStore_Put_CreatesParentDirs(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "newroot") // does not exist yet
	s := maturin.New(root)
	content := []byte("bootstrap")
	dgst := mustDigest(content)

	if err := s.Put(dgst, bytes.NewReader(content)); err != nil {
		t.Fatalf("Put in non-existent root: %v", err)
	}
	if !s.Exists(dgst) {
		t.Fatal("blob not found after Put with new dirs")
	}
}

func TestStore_Put_DigestMismatch(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	content := []byte("actual content")
	wrongDigest := mustDigest([]byte("different content"))

	err := s.Put(wrongDigest, bytes.NewReader(content))
	if !errors.Is(err, maturin.ErrDigestMismatch) {
		t.Fatalf("expected ErrDigestMismatch, got %v", err)
	}
	if s.Exists(wrongDigest) {
		t.Fatal("blob must not be persisted on digest mismatch")
	}
}

func TestStore_Put_InvalidDigest(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	err := s.Put(digest.Digest("not-a-digest"), strings.NewReader("x"))
	if err == nil {
		t.Fatal("expected error for invalid digest")
	}
}

func TestStore_Put_CopyError(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	content := []byte("copy-error content")
	dgst := mustDigest(content)

	err := s.Put(dgst, &failReader{err: errors.New("forced read failure")})
	if err == nil {
		t.Fatal("expected copy error, got nil")
	}
}

// failReader is a reader that always returns an error on Read.
type failReader struct{ err error }

func (r *failReader) Read(_ []byte) (int, error) { return 0, r.err }

func TestStore_Get_Success(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	content := []byte("get me back")
	dgst := mustDigest(content)

	if err := s.Put(dgst, bytes.NewReader(content)); err != nil {
		t.Fatalf("Put: %v", err)
	}

	rc, err := s.Get(dgst)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()

	got, readErr := io.ReadAll(rc)
	if readErr != nil {
		t.Fatalf("ReadAll: %v", readErr)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}
}

func TestStore_Get_NotFound(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	dgst := mustDigest([]byte("absent"))

	_, err := s.Get(dgst)
	if !errors.Is(err, maturin.ErrBlobNotFound) {
		t.Fatalf("expected ErrBlobNotFound, got %v", err)
	}
}

func TestStore_Get_InvalidDigest(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	_, err := s.Get(digest.Digest("sha256:"))
	if err == nil {
		t.Fatal("expected error for invalid digest")
	}
}

func TestStore_Get_CorruptedBlob(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	content := []byte("original content")
	dgst := mustDigest(content)

	if err := s.Put(dgst, bytes.NewReader(content)); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Corrupt the blob on disk.
	blobFile := filepath.Join(s.Root(), "maturin", "blobs", "sha256", dgst.Hex())
	if err := os.WriteFile(blobFile, []byte("corrupted!"), 0o600); err != nil {
		t.Fatalf("corrupt blob: %v", err)
	}

	rc, err := s.Get(dgst)
	if err != nil {
		t.Fatalf("Get should succeed in opening, got: %v", err)
	}
	defer rc.Close()

	_, readErr := io.ReadAll(rc)
	if !errors.Is(readErr, maturin.ErrDigestMismatch) {
		t.Fatalf("expected ErrDigestMismatch on read, got %v", readErr)
	}
}

func TestStore_Exists_True(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	content := []byte("exists check")
	dgst := mustDigest(content)

	if s.Exists(dgst) {
		t.Fatal("Exists should be false before Put")
	}
	if err := s.Put(dgst, bytes.NewReader(content)); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if !s.Exists(dgst) {
		t.Fatal("Exists should be true after Put")
	}
}

func TestStore_Exists_False(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	if s.Exists(mustDigest([]byte("ghost"))) {
		t.Fatal("Exists should return false for absent blob")
	}
}

func TestStore_Delete_Success(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	content := []byte("delete me")
	dgst := mustDigest(content)

	if err := s.Put(dgst, bytes.NewReader(content)); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := s.Delete(dgst); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if s.Exists(dgst) {
		t.Fatal("blob still exists after Delete")
	}
}

func TestStore_Delete_NotFound(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	err := s.Delete(mustDigest([]byte("absent")))
	if !errors.Is(err, maturin.ErrBlobNotFound) {
		t.Fatalf("expected ErrBlobNotFound, got %v", err)
	}
}

// OS error-path tests — cover branches that require filesystem manipulation.

func TestStore_Put_MkdirAllError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	s := maturin.New(root)

	// Create a FILE at "maturin/blobs" so MkdirAll("maturin/blobs/sha256") fails.
	if err := os.MkdirAll(filepath.Join(root, "maturin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "maturin", "blobs"), []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := s.Put(mustDigest([]byte("x")), bytes.NewReader([]byte("x")))
	if err == nil {
		t.Fatal("expected MkdirAll error, got nil")
	}
}

func TestStore_Put_CreateTempError(t *testing.T) {
	t.Parallel()
	if os.Getuid() == 0 {
		t.Skip("requires non-root: chmod(0555) does not restrict root")
	}

	s := newTestStore(t)
	content := []byte("content")
	dgst := mustDigest(content)
	blobDir := filepath.Join(s.Root(), "maturin", "blobs", "sha256")

	// Create the blob dir, then make it read-only so CreateTemp fails.
	if err := os.MkdirAll(blobDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(blobDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blobDir, 0o700) })

	err := s.Put(dgst, bytes.NewReader(content))
	if err == nil {
		t.Fatal("expected CreateTemp error, got nil")
	}
}

func TestStore_Put_RenameError(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	content := []byte("rename-fail content")
	dgst := mustDigest(content)

	// Pre-create a DIRECTORY at the destination blob path so os.Rename fails (EISDIR).
	destDir := filepath.Join(s.Root(), "maturin", "blobs", "sha256", dgst.Hex())
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		t.Fatal(err)
	}

	err := s.Put(dgst, bytes.NewReader(content))
	if err == nil {
		t.Fatal("expected rename error, got nil")
	}
	if errors.Is(err, maturin.ErrDigestMismatch) {
		t.Fatal("should not be ErrDigestMismatch; content matches")
	}
}

func TestStore_Get_OpenErrorNotNotExist(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	s := maturin.New(root)

	// Create "maturin/blobs/sha256" as a FILE so opening a path under it gives ENOTDIR.
	if err := os.MkdirAll(filepath.Join(root, "maturin", "blobs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "maturin", "blobs", "sha256"), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := s.Get(mustDigest([]byte("anything")))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Is(err, maturin.ErrBlobNotFound) {
		t.Fatal("should not be ErrBlobNotFound (path error is not ENOENT)")
	}
}

func TestStore_Delete_RemoveErrorNotNotExist(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	s := maturin.New(root)

	// Create "maturin/blobs/sha256" as a FILE so os.Remove of a path under it gives ENOTDIR.
	if err := os.MkdirAll(filepath.Join(root, "maturin", "blobs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "maturin", "blobs", "sha256"), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}

	err := s.Delete(mustDigest([]byte("anything")))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Is(err, maturin.ErrBlobNotFound) {
		t.Fatal("should not be ErrBlobNotFound (path error is not ENOENT)")
	}
}

package maturin_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/rodrigo-baliza/maestro/internal/maturin"
)

// testDescriptor builds a v1.Descriptor for testing.
func testDescriptor(content []byte) v1.Descriptor {
	return v1.Descriptor{
		MediaType: v1.MediaTypeImageManifest,
		Digest:    mustDigest(content),
		Size:      int64(len(content)),
	}
}

func TestStore_ListIndex_Empty(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	descs, err := s.ListIndex(context.Background())
	if err != nil {
		t.Fatalf("ListIndex: %v", err)
	}
	if len(descs) != 0 {
		t.Errorf("expected 0 descriptors, got %d", len(descs))
	}
}

func TestStore_AddToIndex_Single(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	desc := testDescriptor([]byte("image A"))

	if err := s.AddToIndex(context.Background(), desc); err != nil {
		t.Fatalf("AddToIndex: %v", err)
	}

	descs, err := s.ListIndex(context.Background())
	if err != nil {
		t.Fatalf("ListIndex: %v", err)
	}
	if len(descs) != 1 {
		t.Fatalf("expected 1 descriptor, got %d", len(descs))
	}
	if descs[0].Digest != desc.Digest {
		t.Errorf("digest = %s, want %s", descs[0].Digest, desc.Digest)
	}
}

func TestStore_AddToIndex_ReplaceExisting(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	content := []byte("image B")
	dgst := mustDigest(content)

	descV1 := v1.Descriptor{MediaType: v1.MediaTypeImageManifest, Digest: dgst, Size: 10}
	descV2 := v1.Descriptor{MediaType: v1.MediaTypeImageManifest, Digest: dgst, Size: 20}

	if err := s.AddToIndex(context.Background(), descV1); err != nil {
		t.Fatalf("AddToIndex v1: %v", err)
	}
	if err := s.AddToIndex(context.Background(), descV2); err != nil {
		t.Fatalf("AddToIndex v2: %v", err)
	}

	descs, err := s.ListIndex(context.Background())
	if err != nil {
		t.Fatalf("ListIndex: %v", err)
	}
	if len(descs) != 1 {
		t.Fatalf("expected 1 descriptor after replace, got %d", len(descs))
	}
	if descs[0].Size != descV2.Size {
		t.Errorf("Size = %d, want %d (replace did not take effect)", descs[0].Size, descV2.Size)
	}
}

func TestStore_AddToIndex_MultipleDistinct(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	descA := testDescriptor([]byte("image A"))
	descB := testDescriptor([]byte("image B"))

	if err := s.AddToIndex(context.Background(), descA); err != nil {
		t.Fatalf("AddToIndex A: %v", err)
	}
	if err := s.AddToIndex(context.Background(), descB); err != nil {
		t.Fatalf("AddToIndex B: %v", err)
	}

	descs, err := s.ListIndex(context.Background())
	if err != nil {
		t.Fatalf("ListIndex: %v", err)
	}
	if len(descs) != 2 {
		t.Errorf("expected 2 descriptors, got %d", len(descs))
	}
}

func TestStore_RemoveFromIndex_Success(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	desc := testDescriptor([]byte("to remove"))

	if err := s.AddToIndex(context.Background(), desc); err != nil {
		t.Fatalf("AddToIndex: %v", err)
	}
	if err := s.RemoveFromIndex(context.Background(), desc.Digest); err != nil {
		t.Fatalf("RemoveFromIndex: %v", err)
	}

	descs, err := s.ListIndex(context.Background())
	if err != nil {
		t.Fatalf("ListIndex: %v", err)
	}
	if len(descs) != 0 {
		t.Errorf("expected 0 descriptors after remove, got %d", len(descs))
	}
}

func TestStore_RemoveFromIndex_NotPresent(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	// No-op: removing a digest that was never added must not error.
	absent := mustDigest([]byte("ghost"))
	if err := s.RemoveFromIndex(context.Background(), absent); err != nil {
		t.Fatalf("RemoveFromIndex for absent entry: %v", err)
	}
}

func TestStore_RemoveFromIndex_OnlyRemovesTarget(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	descA := testDescriptor([]byte("keep me"))
	descB := testDescriptor([]byte("remove me"))

	if err := s.AddToIndex(context.Background(), descA); err != nil {
		t.Fatalf("AddToIndex A: %v", err)
	}
	if err := s.AddToIndex(context.Background(), descB); err != nil {
		t.Fatalf("AddToIndex B: %v", err)
	}
	if err := s.RemoveFromIndex(context.Background(), descB.Digest); err != nil {
		t.Fatalf("RemoveFromIndex B: %v", err)
	}

	descs, err := s.ListIndex(context.Background())
	if err != nil {
		t.Fatalf("ListIndex: %v", err)
	}
	if len(descs) != 1 {
		t.Fatalf("expected 1 descriptor, got %d", len(descs))
	}
	if descs[0].Digest != descA.Digest {
		t.Errorf("remaining = %s, want %s", descs[0].Digest, descA.Digest)
	}
}

func TestStore_Index_ParseError(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	// Pre-populate a malformed index.json.
	indexFile := s.Root() + "/maturin/index.json"
	if err := os.MkdirAll(s.Root()+"/maturin", 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(indexFile, []byte("not json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := s.ListIndex(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed index.json")
	}
}

func TestStore_ListIndex_NilManifestsInJSON(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	// index.json without a "manifests" key → idx.Manifests is nil after unmarshal
	// → the nil-guard branch sets it to an empty slice.
	indexDir := filepath.Join(s.Root(), "maturin")
	if err := os.MkdirAll(indexDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(indexDir, "index.json"), []byte(`{"schemaVersion":2}`), 0o600); err != nil {
		t.Fatal(err)
	}

	descs, err := s.ListIndex(context.Background())
	if err != nil {
		t.Fatalf("ListIndex: %v", err)
	}
	if descs == nil {
		t.Error("expected non-nil slice, got nil")
	}
	if len(descs) != 0 {
		t.Errorf("expected 0 descriptors, got %d", len(descs))
	}
}

func TestStore_AddToIndex_Concurrent(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	const workers = 10

	var wg sync.WaitGroup
	wg.Add(workers)

	for i := range workers {
		go func(n int) {
			defer wg.Done()
			content := []byte{byte(n), byte(n >> 8)}
			desc := testDescriptor(content)
			if err := s.AddToIndex(context.Background(), desc); err != nil {
				t.Errorf("AddToIndex worker %d: %v", n, err)
			}
		}(i)
	}
	wg.Wait()

	descs, err := s.ListIndex(context.Background())
	if err != nil {
		t.Fatalf("ListIndex: %v", err)
	}
	if len(descs) != workers {
		t.Errorf("expected %d descriptors after concurrent adds, got %d", workers, len(descs))
	}
}

func TestStore_AddToIndex_ContextCancelled(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	desc := testDescriptor([]byte("cancelled"))
	err := s.AddToIndex(ctx, desc)
	// The lock is uncontested so it may be acquired before the cancellation
	// is noticed; accept both success and cancellation error.
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStore_RemoveFromIndex_ContextCancelled(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.RemoveFromIndex(ctx, digest.Digest("sha256:"+string(make([]byte, 64))))
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStore_ListIndex_ContextCancelled(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.ListIndex(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected error: %v", err)
	}
}

// OS error-path and locking tests.

func TestStore_ReadIndex_ReadError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	s := maturin.New(root)

	// Create a DIRECTORY at index.json so os.ReadFile returns a non-ENOENT error.
	if err := os.MkdirAll(filepath.Join(root, "maturin", "index.json"), 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := s.ListIndex(context.Background())
	if err == nil {
		t.Fatal("expected ReadFile error, got nil")
	}
}

func TestStore_WriteIndex_WriteError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	s := maturin.New(root)

	// Create a DIRECTORY at index.json.tmp so os.WriteFile fails.
	if err := os.MkdirAll(filepath.Join(root, "maturin", "index.json.tmp"), 0o700); err != nil {
		t.Fatal(err)
	}

	// index.json does not exist → readIndex returns empty index → writeIndex fails.
	err := s.AddToIndex(context.Background(), testDescriptor([]byte("x")))
	if err == nil {
		t.Fatal("expected WriteFile error, got nil")
	}
}

func TestStore_AddToIndex_ReadError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	s := maturin.New(root)

	// Pre-populate a malformed index.json so readIndex fails inside the lock.
	if err := os.MkdirAll(filepath.Join(root, "maturin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "maturin", "index.json"), []byte("bad json"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := s.AddToIndex(context.Background(), testDescriptor([]byte("x")))
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestStore_RemoveFromIndex_ReadError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	s := maturin.New(root)

	if err := os.MkdirAll(filepath.Join(root, "maturin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "maturin", "index.json"), []byte("bad json"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := s.RemoveFromIndex(context.Background(), mustDigest([]byte("x")))
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

// lockIndex error-path tests — require direct filesystem and flock manipulation.

func TestStore_LockIndex_MkdirError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	s := maturin.New(root)

	// Create a FILE at {root}/maturin so MkdirAll inside lockIndex fails.
	if err := os.WriteFile(filepath.Join(root, "maturin"), []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := s.ListIndex(context.Background())
	if err == nil {
		t.Fatal("expected MkdirAll error, got nil")
	}
}

func TestStore_LockIndex_OpenFileError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	s := maturin.New(root)

	// Create the maturin dir, then create a DIRECTORY at .index.lock so OpenFile fails.
	if err := os.MkdirAll(filepath.Join(root, "maturin", ".index.lock"), 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := s.ListIndex(context.Background())
	if err == nil {
		t.Fatal("expected OpenFile error, got nil")
	}
}

// holdIndexLock acquires a syscall-level LOCK_EX on the index lock file and
// returns a release function. The caller MUST call release() when done.
func holdIndexLock(t *testing.T, root string) (release func()) {
	t.Helper()
	lockPath := filepath.Join(root, "maturin", ".index.lock")
	if err := os.MkdirAll(filepath.Join(root, "maturin"), 0o700); err != nil {
		t.Fatalf("holdIndexLock MkdirAll: %v", err)
	}

	lf, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("holdIndexLock OpenFile: %v", err)
	}

	if flockErr := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX); flockErr != nil {
		_ = lf.Close()
		t.Fatalf("holdIndexLock Flock: %v", flockErr)
	}

	return func() {
		_ = syscall.Flock(int(lf.Fd()), syscall.LOCK_UN)
		_ = lf.Close()
	}
}

func TestStore_LockIndex_DeadlineExceeded(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	release := holdIndexLock(t, s.Root())
	defer release()

	// Use a context with an already-expired deadline so lockIndex returns
	// "timeout waiting for index lock" before even entering the select.
	pastDeadline := time.Now().Add(-1 * time.Millisecond)
	ctx, cancel := context.WithDeadline(context.Background(), pastDeadline)
	defer cancel()

	_, err := s.ListIndex(ctx)
	if err == nil {
		t.Fatal("expected deadline/timeout error, got nil")
	}
}

func TestStore_LockIndex_ContextCancelledWhileWaiting(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	release := holdIndexLock(t, s.Root())
	defer release()

	// Cancel the context after 80ms: the poll interval is 50ms so one full
	// poll cycle completes (covering time.After(poll)), then ctx.Done() fires.
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(80 * time.Millisecond)
		cancel()
	}()

	_, err := s.ListIndex(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

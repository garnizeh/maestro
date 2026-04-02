package waystation_test

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/rodrigo-baliza/maestro/internal/waystation"
)

type testRecord struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

func newStore(t *testing.T) *waystation.Store {
	t.Helper()
	dir := t.TempDir()
	s := waystation.New(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return s
}

func TestInit_CreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	s := waystation.New(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	expectedDirs := []string{
		"containers",
		"maturin/blobs/sha256",
		"maturin/manifests",
		"dogan",
		"beam",
		"thinnies",
	}
	for _, d := range expectedDirs {
		path := dir + "/" + d
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected dir %s: %v", d, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", d)
		}
		if info.Mode().Perm() != 0o700 {
			t.Errorf("%s permissions = %o, want 0700", d, info.Mode().Perm())
		}
	}
}

func TestInit_Idempotent(t *testing.T) {
	s := newStore(t)
	// Second Init must not error and must not destroy existing data.
	if err := s.Init(); err != nil {
		t.Fatalf("second Init: %v", err)
	}
}

func TestPutGet(t *testing.T) {
	s := newStore(t)
	rec := testRecord{ID: "c1", Value: "hello"}

	if err := s.Put("containers", "c1", rec); err != nil {
		t.Fatalf("Put: %v", err)
	}

	var got testRecord
	if err := s.Get("containers", "c1", &got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != rec {
		t.Errorf("got %+v, want %+v", got, rec)
	}
}

func TestGet_NotFound(t *testing.T) {
	s := newStore(t)
	var v testRecord
	err := s.Get("containers", "nonexistent", &v)
	if !errors.Is(err, waystation.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestDelete(t *testing.T) {
	s := newStore(t)
	rec := testRecord{ID: "c1"}
	_ = s.Put("containers", "c1", rec)

	if err := s.Delete("containers", "c1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	var v testRecord
	if err := s.Get("containers", "c1", &v); !errors.Is(err, waystation.ErrNotFound) {
		t.Errorf("expected ErrNotFound after Delete, got %v", err)
	}
}

func TestDelete_NotFound(t *testing.T) {
	s := newStore(t)
	err := s.Delete("containers", "nonexistent")
	if !errors.Is(err, waystation.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestList(t *testing.T) {
	s := newStore(t)
	for _, id := range []string{"a", "b", "c"} {
		_ = s.Put("containers", id, testRecord{ID: id})
	}

	keys, err := s.List("containers")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("len(keys) = %d, want 3", len(keys))
	}
}

func TestExists(t *testing.T) {
	s := newStore(t)
	if s.Exists("containers", "missing") {
		t.Error("Exists returned true for missing key")
	}
	_ = s.Put("containers", "present", testRecord{})
	if !s.Exists("containers", "present") {
		t.Error("Exists returned false for present key")
	}
}

// TestPut_Atomic verifies that concurrent writes do not produce corrupt JSON.
func TestPut_Atomic(t *testing.T) {
	s := newStore(t)
	const workers = 20
	var wg sync.WaitGroup

	for i := range workers {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			rec := testRecord{ID: "shared", Value: string(rune('A' + n%26))}
			_ = s.Put("containers", "shared", rec)
		}(i)
	}
	wg.Wait()

	// The file must be valid JSON after all concurrent writes.
	var v testRecord
	if err := s.Get("containers", "shared", &v); err != nil {
		t.Fatalf("Get after concurrent writes: %v", err)
	}
	// Re-verify the raw file is valid JSON.
	data, _ := os.ReadFile(s.Root() + "/containers/shared.json")
	if !json.Valid(data) {
		t.Error("file contains invalid JSON after concurrent writes")
	}
}

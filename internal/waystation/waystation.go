// Package waystation implements Maestro's file-based state store.
//
// Named after the Way Stations in The Dark Tower — the safe havens that Roland
// finds along the path of the Beam, each holding essential supplies for the
// journey ahead.
package waystation

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrNotFound is returned when a requested state record does not exist.
var ErrNotFound = errors.New("not found")

// Store is the top-level state store. All reads/writes go through it.
type Store struct {
	root string
}

// New returns a Store rooted at dir. Call Init to create the directory tree.
func New(root string) *Store {
	return &Store{root: root}
}

// Root returns the root directory path.
func (s *Store) Root() string { return s.root }

// Init creates the required directory tree with 0700 permissions.
// It is idempotent — safe to call on every startup.
func (s *Store) Init() error {
	dirs := []string{
		s.root,
		filepath.Join(s.root, "containers"),
		filepath.Join(s.root, "maturin", "blobs", "sha256"),
		filepath.Join(s.root, "maturin", "manifests"),
		filepath.Join(s.root, "dogan"),
		filepath.Join(s.root, "beam"),
		filepath.Join(s.root, "thinnies"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fmt.Errorf("create dir %s: %w", d, err)
		}
	}
	return nil
}

// path returns the path to the JSON file for a given collection and key.
func (s *Store) path(collection, key string) string {
	return filepath.Join(s.root, collection, key+".json")
}

// Put writes v as JSON to collection/key.json atomically (write-then-rename).
func (s *Store) Put(collection, key string, v any) error {
	dst := s.path(collection, key)

	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	// Ensure the collection directory exists.
	if mkdirErr := os.MkdirAll(filepath.Dir(dst), 0o700); mkdirErr != nil {
		return fmt.Errorf("create collection dir: %w", mkdirErr)
	}

	// Write to a temp file in the same directory so rename is atomic.
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".tmp-"+key+"-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()

	if _, writeErr := tmp.Write(data); writeErr != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write temp: %w", writeErr) //coverage:ignore disk full not simulatable in unit tests
	}
	if closeErr := tmp.Close(); closeErr != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp: %w", closeErr) //coverage:ignore unreachable after successful Write
	}

	if renameErr := os.Rename(tmpName, dst); renameErr != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename: %w", renameErr) //coverage:ignore cross-device mount unreachable in same-dir temp
	}

	return nil
}

// Get reads collection/key.json and unmarshals it into v.
// Returns ErrNotFound when the file does not exist.
func (s *Store) Get(collection, key string, v any) error {
	data, err := os.ReadFile(s.path(collection, key))
	if err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return fmt.Errorf("read: %w", err)
	}
	if unmarshalErr := json.Unmarshal(data, v); unmarshalErr != nil {
		return fmt.Errorf("unmarshal: %w", unmarshalErr)
	}
	return nil
}

// Delete removes collection/key.json.
// Returns ErrNotFound when the file does not exist.
func (s *Store) Delete(collection, key string) error {
	err := os.Remove(s.path(collection, key))
	if err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return fmt.Errorf("delete: %w", err)
	}
	return nil
}

// List returns all keys in a collection (the filenames without .json extension).
func (s *Store) List(collection string) ([]string, error) {
	dir := filepath.Join(s.root, collection)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list %s: %w", collection, err)
	}

	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if filepath.Ext(name) == ".json" {
			keys = append(keys, name[:len(name)-5]) // strip .json
		}
	}
	return keys, nil
}

// Exists returns true when collection/key.json exists.
func (s *Store) Exists(collection, key string) bool {
	_, err := os.Stat(s.path(collection, key))
	return err == nil
}

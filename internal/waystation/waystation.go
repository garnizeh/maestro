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

	"github.com/rs/zerolog/log"

	"github.com/garnizeh/maestro/internal/sys"
)

// ErrNotFound is returned when a requested state record does not exist.
var ErrNotFound = errors.New("not found")

const (
	dirPerm = 0o700
)

type TempFile interface {
	Write(p []byte) (n int, err error)
	Close() error
	Name() string
}

type FS interface {
	MkdirAll(path string, perm os.FileMode) error
	CreateTemp(dir, pattern string) (TempFile, error)
	Remove(name string) error
	Rename(oldpath, newpath string) error
	ReadFile(name string) ([]byte, error)
	ReadDir(name string) ([]os.DirEntry, error)
	Stat(name string) (os.FileInfo, error)
}

type Marshaller interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

type Locker interface {
	Flock(fd int, how int) error
}

// ── Thin Shell Implementations ───────────────────────────────────────────────

type RealFS struct{ sys.RealFS }

func (r RealFS) CreateTemp(d, p string) (TempFile, error) {
	return r.RealFS.CreateTemp(d, p)
}

type RealLocker = sys.RealFS

type realJSON struct{}

func (realJSON) Marshal(v any) ([]byte, error)   { return json.Marshal(v) }
func (realJSON) Unmarshal(d []byte, v any) error { return json.Unmarshal(d, v) }

// Store is the top-level state store. All reads/writes go through it.
type Store struct {
	root   string
	fs     FS
	json   Marshaller
	locker Locker
}

// New returns a Store rooted at dir. Call Init to create the directory tree.
func New(root string) *Store {
	return &Store{
		root:   root,
		fs:     RealFS{},
		json:   realJSON{},
		locker: RealLocker{},
	}
}

// WithFS sets a custom filesystem implementation.
func (s *Store) WithFS(fs FS) *Store {
	s.fs = fs
	return s
}

// WithMarshaller sets a custom JSON marshaller implementation.
func (s *Store) WithMarshaller(m Marshaller) *Store {
	s.json = m
	return s
}

// WithLocker sets a custom file locker implementation.
func (s *Store) WithLocker(l Locker) *Store {
	s.locker = l
	return s
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
		if err := s.fs.MkdirAll(d, dirPerm); err != nil {
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

	data, err := s.json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	// Ensure the collection directory exists.
	if mkdirErr := s.fs.MkdirAll(filepath.Dir(dst), dirPerm); mkdirErr != nil {
		return fmt.Errorf("create collection dir: %w", mkdirErr)
	}

	// Write to a temp file in the same directory so rename is atomic.
	tmp, err := s.fs.CreateTemp(filepath.Dir(dst), ".tmp-"+key+"-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()

	if _, writeErr := tmp.Write(data); writeErr != nil {
		if closeErr := tmp.Close(); closeErr != nil {
			log.Debug().
				Err(closeErr).
				Str("tmp", tmpName).
				Msg("waystation: failed to close temp file after write failure")
		}
		if rmErr := s.fs.Remove(tmpName); rmErr != nil {
			log.Debug().
				Err(rmErr).
				Str("tmp", tmpName).
				Msg("waystation: failed to remove temp file after write failure")
		}
		return fmt.Errorf("write temp: %w", writeErr)
	}
	if closeErr := tmp.Close(); closeErr != nil {
		if rmErr := s.fs.Remove(tmpName); rmErr != nil {
			log.Debug().
				Err(rmErr).
				Str("tmp", tmpName).
				Msg("waystation: failed to remove temp file after close failure")
		}
		return fmt.Errorf("close temp: %w", closeErr)
	}

	if renameErr := s.fs.Rename(tmpName, dst); renameErr != nil {
		if rmErr := s.fs.Remove(tmpName); rmErr != nil {
			log.Debug().
				Err(rmErr).
				Str("tmp", tmpName).
				Msg("waystation: failed to remove temp file after rename failure")
		}
		return fmt.Errorf("rename: %w", renameErr)
	}

	return nil
}

// Get reads collection/key.json and unmarshals it into v.
// Returns ErrNotFound when the file does not exist.
func (s *Store) Get(collection, key string, v any) error {
	data, err := s.fs.ReadFile(s.path(collection, key))
	if err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return fmt.Errorf("read: %w", err)
	}
	if unmarshalErr := s.json.Unmarshal(data, v); unmarshalErr != nil {
		return fmt.Errorf("unmarshal: %w", unmarshalErr)
	}
	return nil
}

// Delete removes collection/key.json.
// Returns ErrNotFound when the file does not exist.
func (s *Store) Delete(collection, key string) error {
	err := s.fs.Remove(s.path(collection, key))
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
	entries, err := s.fs.ReadDir(dir)
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
	_, err := s.fs.Stat(s.path(collection, key))
	return err == nil
}

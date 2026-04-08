package prim

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// AllWorld implements the [Prim] interface using OverlayFS.
// It is the primary, kernel-native driver for Maestro.
//
// Storage layout (under root):
//
//	prim/
//	└── snapshots/
//	    └── <key>/
//	        ├── meta.json          — snapshot metadata
//	        ├── work/              — OverlayFS workdir
//	        └── fs/                — active/committed filesystem content
type AllWorld struct {
	root    string
	mountFn func(source, target, fstype string, flags uintptr, data string) error
	mu      sync.RWMutex
}

// NewAllWorld returns a new AllWorld driver rooted at root.
// The mountFn is usually [syscall.Mount] but can be mocked for testing.
func NewAllWorld(
	root string,
	mountFn func(source, target, fstype string, flags uintptr, data string) error,
) (*AllWorld, error) {
	if mountFn == nil {
		mountFn = osSysMount
	}
	a := &AllWorld{
		root:    root,
		mountFn: mountFn,
	}
	if err := os.MkdirAll(a.snapshotsDir(), dirPerm); err != nil {
		return nil, fmt.Errorf("allworld: create snapshots dir: %w", err)
	}
	return a, nil
}

// Prepare creates a writable (KindActive) snapshot with the given parent.
func (a *AllWorld) Prepare(_ context.Context, key, parent string) ([]Mount, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if err := a.checkNotExists(key); err != nil {
		return nil, err
	}

	snapDir := a.snapshotDir(key)
	if err := os.MkdirAll(filepath.Join(snapDir, "work"), dirPerm); err != nil {
		return nil, fmt.Errorf("allworld: prepare %s: %w", key, err)
	}
	if err := os.MkdirAll(filepath.Join(snapDir, "fs"), dirPerm); err != nil {
		return nil, fmt.Errorf("allworld: prepare %s: %w", key, err)
	}

	meta := VFSMeta{Key: key, Parent: parent, Kind: KindActive}
	if err := writeMeta(snapDir, meta); err != nil {
		_ = os.RemoveAll(snapDir)
		return nil, fmt.Errorf("allworld: prepare %s: write meta: %w", key, err)
	}

	return a.mounts(key, parent)
}

// View creates a read-only (KindView) snapshot.
// View creates a new writable snapshot based on a parent.
func (a *AllWorld) View(_ context.Context, key, parent string) ([]Mount, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if err := a.checkNotExists(key); err != nil {
		return nil, err
	}

	snapDir := a.snapshotDir(key)
	if err := os.MkdirAll(filepath.Join(snapDir, "fs"), dirPerm); err != nil {
		return nil, fmt.Errorf("allworld: view %s: %w", key, err)
	}

	meta := VFSMeta{Key: key, Parent: parent, Kind: KindView}
	if err := writeMeta(snapDir, meta); err != nil {
		_ = os.RemoveAll(snapDir)
		return nil, fmt.Errorf("allworld: view %s: write meta: %w", key, err)
	}

	return a.mounts(key, parent)
}

// Commit seals an active snapshot into an immutable committed snapshot.
func (a *AllWorld) Commit(_ context.Context, name, key string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	meta, err := a.readMeta(key)
	if err != nil {
		return fmt.Errorf("allworld: commit %s→%s: %w", key, name, err)
	}
	if meta.Kind == KindView {
		return fmt.Errorf("allworld: commit %s→%s: %w", key, name, ErrCommitOnReadOnly)
	}

	meta.Kind = KindCommitted
	if writeErr := writeMeta(a.snapshotDir(key), meta); writeErr != nil {
		return fmt.Errorf("allworld: commit %s→%s: write meta: %w", key, name, writeErr)
	}

	src := a.snapshotDir(key)
	dst := a.snapshotDir(name)
	if renameErr := os.Rename(src, dst); renameErr != nil {
		return fmt.Errorf("allworld: commit %s→%s: rename: %w", key, name, renameErr)
	}

	return nil
}

// Remove removes a snapshot and releases all storage.
func (a *AllWorld) Remove(_ context.Context, key string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	has, err := a.hasDependents(key)
	if err != nil {
		return fmt.Errorf("allworld: remove %s: check dependents: %w", key, err)
	}
	if has {
		return fmt.Errorf("allworld: remove %s: %w", key, ErrSnapshotHasDependents)
	}

	if rmErr := os.RemoveAll(a.snapshotDir(key)); rmErr != nil {
		return fmt.Errorf("allworld: remove %s: %w", key, rmErr)
	}

	return nil
}

// Walk calls fn for every snapshot in the store, in insertion order.
func (a *AllWorld) Walk(_ context.Context, fn func(Info) error) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	des, err := os.ReadDir(a.snapshotsDir())
	if err != nil {
		return fmt.Errorf("allworld: walk: %w", err)
	}

	for _, de := range des {
		if !de.IsDir() {
			continue
		}
		meta, metaErr := a.readMeta(de.Name())
		if metaErr != nil {
			continue
		}
		info := Info(meta)
		if walkErr := fn(info); walkErr != nil {
			return walkErr
		}
	}

	return nil
}

// Usage reports disk consumption for a snapshot.
func (a *AllWorld) Usage(_ context.Context, key string) (Usage, error) {
	var usage Usage
	snapDir := filepath.Join(a.snapshotDir(key), "fs")

	err := filepath.WalkDir(snapDir, func(_ string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		usage.Inodes++
		usage.Size += info.Size()
		return nil
	})

	if err != nil {
		return usage, fmt.Errorf("allworld: usage %s: %w", key, err)
	}

	return usage, nil
}

// Internal helpers.

func (a *AllWorld) snapshotsDir() string          { return filepath.Join(a.root, "prim", "snapshots") }
func (a *AllWorld) snapshotDir(key string) string { return filepath.Join(a.snapshotsDir(), key) }

func (a *AllWorld) checkNotExists(key string) error {
	if _, err := os.Stat(a.snapshotDir(key)); err == nil {
		return fmt.Errorf("allworld: %s: %w", key, ErrSnapshotAlreadyExists)
	}
	return nil
}

func (a *AllWorld) hasDependents(key string) (bool, error) {
	des, err := os.ReadDir(a.snapshotsDir())
	if err != nil {
		return false, err
	}
	for _, de := range des {
		if !de.IsDir() || de.Name() == key {
			continue
		}
		meta, metaErr := a.readMeta(de.Name())
		if metaErr != nil {
			continue
		}
		if meta.Parent == key {
			return true, nil
		}
	}
	return false, nil
}

func (a *AllWorld) readMeta(key string) (VFSMeta, error) {
	var m VFSMeta
	data, err := os.ReadFile(filepath.Join(a.snapshotDir(key), "meta.json"))
	if err != nil {
		return m, err
	}
	if jsonErr := json.Unmarshal(data, &m); jsonErr != nil {
		return m, jsonErr
	}
	return m, nil
}

func (a *AllWorld) mounts(key, parent string) ([]Mount, error) {
	// Root layer (no parent).
	if parent == "" {
		return []Mount{{
			Type:   "bind",
			Source: filepath.Join(a.snapshotDir(key), "fs"),
			Options: []string{
				"bind",
				"rw",
			},
		}}, nil
	}

	// Overlay chain.
	var layers []string
	curr := parent
	for curr != "" {
		layers = append(layers, filepath.Join(a.snapshotDir(curr), "fs"))
		meta, err := a.readMeta(curr)
		if err != nil {
			return nil, fmt.Errorf("read meta for %s: %w", curr, err)
		}
		curr = meta.Parent
	}

	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s",
		strings.Join(layers, ":"),
		filepath.Join(a.snapshotDir(key), "fs"),
		filepath.Join(a.snapshotDir(key), "work"),
	)

	return []Mount{{
		Type:    "overlay",
		Source:  "overlay",
		Options: strings.Split(opts, ","),
	}}, nil
}

// ProbeOverlay checks if OverlayFS is functional in the current environment.
func ProbeOverlay(dir string, mountFn func(source, target, fstype string, flags uintptr, data string) error) error {
	if mountFn == nil {
		mountFn = osSysMount
	}
	const (
		lowerDir = "lower"
		upperDir = "upper"
		workDir  = "work"
		mergeDir = "merge"
	)

	for _, d := range []string{lowerDir, upperDir, workDir, mergeDir} {
		if err := os.MkdirAll(filepath.Join(dir, d), dirPerm); err != nil {
			return fmt.Errorf("probe: mkdir %s: %w", d, err)
		}
	}

	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s",
		filepath.Join(dir, lowerDir),
		filepath.Join(dir, upperDir),
		filepath.Join(dir, workDir),
	)

	err := mountFn("overlay", filepath.Join(dir, mergeDir), "overlay", 0, opts)
	if err != nil {
		return fmt.Errorf("probe: mount overlay: %w", err)
	}

	return nil
}

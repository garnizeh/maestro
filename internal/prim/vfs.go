package prim

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// VFSMeta holds snapshot metadata for the VFS driver.
type VFSMeta struct {
	Key    string `json:"key"`
	Parent string `json:"parent"`
	Kind   Kind   `json:"kind"`
}

// VFS implements the [Prim] interface via full filesystem copies.
// Each layer is a complete copy of its parent, stored in its own directory.
// This is slower than OverlayFS but works on any filesystem without kernel
// overlay support, making it the universal fallback (the "vfs" driver).
//
// Storage layout (under root):
//
//	prim/
//	└── snapshots/
//	    └── <key>/
//	        ├── meta.json          — snapshot metadata
//	        └── fs/                — the actual filesystem contents
type VFS struct {
	root string
	mu   sync.RWMutex
}

// NewVFS returns a new VFS driver rooted at root.
func NewVFS(root string) (*VFS, error) {
	v := &VFS{root: root}
	if err := os.MkdirAll(v.snapshotsDir(), dirPerm); err != nil {
		return nil, fmt.Errorf("vfs: create snapshots dir: %w", err)
	}
	return v, nil
}

// Prepare creates a new writable snapshot based on a parent.
func (v *VFS) Prepare(_ context.Context, key, parent string) ([]Mount, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.create(key, parent, KindActive)
}

// View creates a read-only (KindView) snapshot.
func (v *VFS) View(_ context.Context, key, parent string) ([]Mount, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.create(key, parent, KindView)
}

func (v *VFS) create(key, parent string, kind Kind) ([]Mount, error) {
	if err := v.checkNotExists(key); err != nil {
		return nil, err
	}

	snapDir := v.snapshotDir(key)
	if err := os.MkdirAll(filepath.Join(snapDir, "fs"), dirPerm); err != nil {
		return nil, fmt.Errorf("vfs: %s: %w", key, err)
	}

	if parent != "" {
		parentDir := filepath.Join(v.snapshotDir(parent), "fs")
		if _, statErr := os.Stat(parentDir); statErr != nil {
			_ = os.RemoveAll(snapDir)
			if os.IsNotExist(statErr) {
				return nil, fmt.Errorf("vfs: %s: %w", key, ErrSnapshotNotFound)
			}
			return nil, fmt.Errorf("vfs: %s: stat parent %s: %w", key, parent, statErr)
		}

		if err := copyDir(parentDir, filepath.Join(snapDir, "fs")); err != nil {
			_ = os.RemoveAll(snapDir)
			return nil, fmt.Errorf("vfs: %s: copy parent %s: %w", key, parent, err)
		}
	}

	meta := VFSMeta{Key: key, Parent: parent, Kind: kind}
	if err := writeMeta(snapDir, meta); err != nil {
		_ = os.RemoveAll(snapDir)
		return nil, fmt.Errorf("vfs: %s: write meta: %w", key, err)
	}

	opts := "ro"
	if kind == KindActive {
		opts = "rw"
	}

	return []Mount{{Type: "bind", Source: filepath.Join(snapDir, "fs"), Options: []string{opts}}}, nil
}

// Commit seals an active snapshot into an immutable committed snapshot.
func (v *VFS) Commit(_ context.Context, name, key string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	meta, err := v.readMeta(key)
	if err != nil {
		return fmt.Errorf("vfs: commit %s→%s: %w", key, name, err)
	}
	if meta.Kind == KindView {
		return fmt.Errorf("vfs: commit %s→%s: %w", key, name, ErrCommitOnReadOnly)
	}

	meta.Kind = KindCommitted
	if writeErr := writeMeta(v.snapshotDir(key), meta); writeErr != nil {
		return fmt.Errorf("vfs: commit %s→%s: write meta: %w", key, name, writeErr)
	}

	src := v.snapshotDir(key)
	dst := v.snapshotDir(name)
	if renameErr := os.Rename(src, dst); renameErr != nil {
		return fmt.Errorf("vfs: commit %s→%s: rename: %w", key, name, renameErr)
	}

	return nil
}

// Remove removes a snapshot and releases all storage.
func (v *VFS) Remove(_ context.Context, key string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	has, err := v.hasDependents(key)
	if err != nil {
		return fmt.Errorf("vfs: remove %s: check dependents: %w", key, err)
	}
	if has {
		return fmt.Errorf("vfs: remove %s: %w", key, ErrSnapshotHasDependents)
	}

	if _, statErr := os.Stat(v.snapshotDir(key)); statErr != nil {
		if os.IsNotExist(statErr) {
			return fmt.Errorf("vfs: remove %s: %w", key, ErrSnapshotNotFound)
		}
		return fmt.Errorf("vfs: remove %s: stat: %w", key, statErr)
	}

	if rmErr := os.RemoveAll(v.snapshotDir(key)); rmErr != nil {
		return fmt.Errorf("vfs: remove %s: %w", key, rmErr)
	}

	return nil
}

// Walk calls fn for every snapshot in the store, in insertion order.
func (v *VFS) Walk(_ context.Context, fn func(Info) error) error {
	v.mu.RLock()
	defer v.mu.RUnlock()

	des, err := os.ReadDir(v.snapshotsDir())
	if err != nil {
		return fmt.Errorf("vfs: walk: %w", err)
	}

	for _, de := range des {
		if !de.IsDir() {
			continue
		}
		meta, metaErr := v.readMeta(de.Name())
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
func (v *VFS) Usage(_ context.Context, key string) (Usage, error) {
	var usage Usage

	snapDir := v.snapshotDir(key)
	if _, statErr := os.Stat(snapDir); statErr != nil {
		if os.IsNotExist(statErr) {
			return usage, fmt.Errorf("vfs: usage %s: %w", key, ErrSnapshotNotFound)
		}
		return usage, fmt.Errorf("vfs: usage %s: stat: %w", key, statErr)
	}

	fsDir := filepath.Join(snapDir, "fs")
	if _, statErr := os.Stat(fsDir); statErr != nil {
		if os.IsNotExist(statErr) {
			return usage, fmt.Errorf("vfs: usage %s: %w", key, ErrSnapshotNotFound)
		}
		return usage, fmt.Errorf("vfs: usage %s: stat fs: %w", key, statErr)
	}

	err := filepath.WalkDir(fsDir, func(_ string, d fs.DirEntry, walkErr error) error {
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
		return usage, fmt.Errorf("vfs: usage %s: %w", key, err)
	}

	return usage, nil
}

// Internal helpers.

func (v *VFS) snapshotsDir() string          { return filepath.Join(v.root, "prim", "snapshots") }
func (v *VFS) snapshotDir(key string) string { return filepath.Join(v.snapshotsDir(), key) }

func (v *VFS) checkNotExists(key string) error {
	if _, err := os.Stat(v.snapshotDir(key)); err == nil {
		return fmt.Errorf("vfs: %s: %w", key, ErrSnapshotAlreadyExists)
	}
	return nil
}

func (v *VFS) hasDependents(key string) (bool, error) {
	des, err := os.ReadDir(v.snapshotsDir())
	if err != nil {
		return false, err
	}
	for _, de := range des {
		if !de.IsDir() || de.Name() == key {
			continue
		}
		meta, metaErr := v.readMeta(de.Name())
		if metaErr != nil {
			continue
		}
		if meta.Parent == key {
			return true, nil
		}
	}
	return false, nil
}

func (v *VFS) readMeta(key string) (VFSMeta, error) {
	var m VFSMeta
	data, err := os.ReadFile(filepath.Join(v.snapshotDir(key), "meta.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return m, fmt.Errorf("vfs: %s: %w", key, ErrSnapshotNotFound)
		}
		return m, err
	}
	if jsonErr := json.Unmarshal(data, &m); jsonErr != nil {
		return m, jsonErr
	}
	return m, nil
}

func writeMeta(dir string, meta VFSMeta) error {
	data, _ := json.MarshalIndent(meta, "", "  ")
	return os.WriteFile(filepath.Join(dir, "meta.json"), data, filePerm)
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, info.Mode()) //nolint:gosec // dst is internal and managed by the driver
}

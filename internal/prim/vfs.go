package prim

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/rs/zerolog/log"

	"github.com/rodrigo-baliza/maestro/pkg/archive"
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
	fs   FS
}

// NewVFS returns a new VFS driver rooted at root.
func NewVFS(root string) (*VFS, error) {
	v := &VFS{
		root: root,
		fs:   RealFS{},
	}
	if err := v.fs.MkdirAll(v.snapshotsDir(), dirPerm); err != nil {
		return nil, fmt.Errorf("vfs: create snapshots dir: %w", err)
	}
	return v, nil
}

// WithFS sets the filesystem implementation.
func (v *VFS) WithFS(fs FS) *VFS {
	v.fs = fs
	return v
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
	if err := v.fs.MkdirAll(filepath.Join(snapDir, "fs"), fsDirPerm); err != nil {
		return nil, fmt.Errorf("vfs: %s: %w", key, err)
	}

	if parent != "" {
		if _, err := v.readMeta(parent); err != nil {
			return nil, fmt.Errorf("vfs: parent %s: %w", parent, ErrSnapshotNotFound)
		}
		parentDir := filepath.Join(v.snapshotDir(parent), "fs")
		if err := v.copyDir(parentDir, filepath.Join(snapDir, "fs")); err != nil {
			if rmErr := v.fs.RemoveAll(snapDir); rmErr != nil {
				log.Warn().Err(rmErr).Str("snapDir", snapDir).
					Msg("vfs: failed to cleanup directory after copy failure")
			}
			return nil, fmt.Errorf("vfs: %s: copy parent %s: %w", key, parent, err)
		}
	}

	meta := VFSMeta{Key: key, Parent: parent, Kind: kind}
	if err := v.writeMeta(snapDir, meta); err != nil {
		if rmErr := v.fs.RemoveAll(snapDir); rmErr != nil {
			log.Warn().Err(rmErr).Str("snapDir", snapDir).
				Msg("vfs: failed to cleanup directory after meta write failure")
		}
		return nil, fmt.Errorf("vfs: %s: write meta: %w", key, err)
	}

	opts := "ro"
	if kind == KindActive {
		opts = "rw"
	}

	return []Mount{
		{Type: "bind", Source: filepath.Join(snapDir, "fs"), Options: []string{"bind", opts}},
	}, nil
}

// Commit seals an active snapshot into an immutable committed snapshot.
func (v *VFS) Commit(_ context.Context, name, key string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	// If the destination exists, we only allow overwriting it if it's a "zombie"
	// (exists but no meta.json). If it's a valid snapshot, we must fail.
	if _, statErr := v.fs.Stat(v.snapshotDir(name)); statErr == nil {
		_, metaErr := v.readMeta(name)
		if metaErr == nil {
			return fmt.Errorf("vfs: commit %s→%s: %w", key, name, ErrSnapshotAlreadyExists)
		}
		// Destination exists but is invalid. Clean it up.
		if rmErr := v.fs.RemoveAll(v.snapshotDir(name)); rmErr != nil {
			return fmt.Errorf("vfs: commit %s→%s: cleanup zombie: %w", key, name, rmErr)
		}
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("vfs: commit %s→%s: stat dest: %w", key, name, statErr)
	}

	meta, err := v.readMeta(key)
	if err != nil {
		return fmt.Errorf("vfs: commit %s→%s: %w", key, name, err)
	}
	if meta.Kind == KindView {
		return fmt.Errorf("vfs: commit %s→%s: %w", key, name, ErrCommitOnReadOnly)
	}

	meta.Kind = KindCommitted
	meta.Key = name
	if writeErr := v.writeMeta(v.snapshotDir(key), meta); writeErr != nil {
		return fmt.Errorf("vfs: commit %s→%s: write meta: %w", key, name, writeErr)
	}

	src := v.snapshotDir(key)
	dst := v.snapshotDir(name)
	if renameErr := v.fs.Rename(src, dst); renameErr != nil {
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

	if _, statErr := v.fs.Stat(v.snapshotDir(key)); statErr != nil {
		if os.IsNotExist(statErr) {
			return fmt.Errorf("vfs: remove %s: %w", key, ErrSnapshotNotFound)
		}
		return fmt.Errorf("vfs: remove %s: stat: %w", key, statErr)
	}

	if rmErr := v.fs.RemoveAll(v.snapshotDir(key)); rmErr != nil {
		return fmt.Errorf("vfs: remove %s: %w", key, rmErr)
	}

	return nil
}

// Walk calls fn for every snapshot in the store, in insertion order.
func (v *VFS) Walk(_ context.Context, fn func(Info) error) error {
	v.mu.RLock()
	defer v.mu.RUnlock()

	des, err := v.fs.ReadDir(v.snapshotsDir())
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

func (v *VFS) Usage(_ context.Context, key string) (Usage, error) {
	var usage Usage
	snapDir := v.snapshotDir(key)
	if _, statErr := v.fs.Stat(snapDir); statErr != nil {
		if os.IsNotExist(statErr) {
			return usage, fmt.Errorf("vfs: usage %s: %w", key, ErrSnapshotNotFound)
		}
		return usage, fmt.Errorf("vfs: usage %s: stat: %w", key, statErr)
	}

	fsDir := filepath.Join(snapDir, "fs")
	err := v.fs.WalkDir(fsDir, func(_ string, d os.DirEntry, walkErr error) error {
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

// WritableDir returns the absolute path to the writable directory for the given snapshot.
func (v *VFS) WritableDir(key string) string {
	return filepath.Join(v.snapshotDir(key), "fs")
}

// WhiteoutFormat returns the whiteout handling strategy for the VFS driver.
func (v *VFS) WhiteoutFormat() archive.WhiteoutFormat {
	return archive.WhiteoutVFS
}

// Internal helpers.

func (v *VFS) snapshotsDir() string          { return filepath.Join(v.root, "prim", "snapshots") }
func (v *VFS) snapshotDir(key string) string { return filepath.Join(v.snapshotsDir(), key) }

func (v *VFS) checkNotExists(key string) error {
	if _, err := v.fs.Stat(v.snapshotDir(key)); err == nil {
		return fmt.Errorf("vfs: %s: %w", key, ErrSnapshotAlreadyExists)
	}
	return nil
}

func (v *VFS) hasDependents(key string) (bool, error) {
	des, err := v.fs.ReadDir(v.snapshotsDir())
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
	data, err := v.fs.ReadFile(filepath.Join(v.snapshotDir(key), "meta.json"))
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

func (v *VFS) writeMeta(dir string, meta VFSMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("vfs: marshal meta: %w", err)
	}
	return v.fs.WriteFile(filepath.Join(dir, "meta.json"), data, filePerm)
}

func (v *VFS) copyDir(src, dst string) error {
	return v.fs.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, errRel := filepath.Rel(src, path)
		if errRel != nil {
			return fmt.Errorf("vfs: copy: path relativity: %w", errRel)
		}
		target := filepath.Join(dst, rel)

		switch {
		case info.Mode().IsDir():
			return v.fs.MkdirAll(target, info.Mode())
		case info.Mode()&os.ModeSymlink != 0:
			return v.copySymlink(path, target)
		case info.Mode().IsRegular():
			return v.copyFile(path, target)
		default:
			// For other types (devices, pipes), we skip them in rootless VFS.
			return nil
		}
	})
}

func (v *VFS) copySymlink(src, dst string) error {
	target, err := v.fs.Readlink(src)
	if err != nil {
		return err
	}
	return v.fs.Symlink(target, dst)
}

func (v *VFS) copyFile(src, dst string) (err error) {
	in, err := v.fs.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := v.fs.FileStat(in)
	if err != nil {
		return err
	}

	out, err := v.fs.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer func() {
		closeErr := out.Close()
		if err == nil {
			err = closeErr
		}
	}()

	if _, err = v.fs.Copy(out, in); err != nil {
		return err
	}
	return nil
}

package prim

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"

	"github.com/garnizeh/maestro/pkg/archive"
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
	root string
	mu   sync.RWMutex
	fs   FS
	mnt  Mounter
}

// NewAllWorld returns a new AllWorld driver rooted at root.
func NewAllWorld(root string) (*AllWorld, error) {
	a := &AllWorld{
		root: root,
		fs:   RealFS{},
		mnt:  &RealMounter{},
	}
	if err := a.fs.MkdirAll(a.snapshotsDir(), dirPerm); err != nil {
		return nil, fmt.Errorf("allworld: create snapshots dir: %w", err)
	}
	return a, nil
}

// WithFS sets the filesystem implementation.
func (a *AllWorld) WithFS(fs FS) *AllWorld {
	a.fs = fs
	return a
}

// WithMounter sets the mounter implementation.
func (a *AllWorld) WithMounter(mnt Mounter) *AllWorld {
	a.mnt = mnt
	return a
}

// Prepare creates a writable (KindActive) snapshot with the given parent.
func (a *AllWorld) Prepare(ctx context.Context, key, parent string) ([]Mount, error) {
	return prepareHelper(
		ctx,
		a.fs,
		&a.mu,
		a.snapshotDir,
		a.checkNotExists,
		a.writeMeta,
		a.mounts,
		key,
		parent,
	)
}

// View creates a read-only (KindView) snapshot.
func (a *AllWorld) View(ctx context.Context, key, parent string) ([]Mount, error) {
	return viewHelper(
		ctx,
		a.fs,
		&a.mu,
		a.snapshotDir,
		a.checkNotExists,
		a.writeMeta,
		a.mounts,
		key,
		parent,
		"allworld",
	)
}

// Commit seals an active snapshot into an immutable committed snapshot.
func (a *AllWorld) Commit(_ context.Context, name, key string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// If the destination exists, we must fail with ErrSnapshotAlreadyExists.
	if _, statErr := a.fs.Stat(a.snapshotDir(name)); statErr == nil {
		return fmt.Errorf("allworld: commit %s→%s: %w", key, name, ErrSnapshotAlreadyExists)
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("allworld: commit %s→%s: stat dest: %w", key, name, statErr)
	}

	meta, err := a.readMeta(key)
	if err != nil {
		return fmt.Errorf("allworld: commit %s→%s: %w", key, name, err)
	}
	if meta.Kind == KindView {
		return fmt.Errorf("allworld: commit %s→%s: %w", key, name, ErrCommitOnReadOnly)
	}

	meta.Kind = KindCommitted
	meta.Key = name // FIX: Update the key to the new name!
	log.Debug().Str("id", name).Str("src", key).Msg("allworld: finalizing commit")
	if writeErr := a.writeMeta(a.snapshotDir(key), meta); writeErr != nil {
		return fmt.Errorf("allworld: commit %s→%s: write meta: %w", key, name, writeErr)
	}

	src := a.snapshotDir(key)
	dst := a.snapshotDir(name)
	log.Debug().Str("src", src).Str("dst", dst).Msg("allworld: renaming snapshot directory")
	if renameErr := a.fs.Rename(src, dst); renameErr != nil {
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

	if rmErr := a.fs.RemoveAll(a.snapshotDir(key)); rmErr != nil {
		return fmt.Errorf("allworld: remove %s: %w", key, rmErr)
	}

	return nil
}

// Walk calls fn for every snapshot in the store, in insertion order.
func (a *AllWorld) Walk(_ context.Context, fn func(Info) error) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	des, err := a.fs.ReadDir(a.snapshotsDir())
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

	err := a.fs.WalkDir(snapDir, func(_ string, d os.DirEntry, walkErr error) error {
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

// WritableDir returns the absolute path to the writable directory for the given snapshot.
func (a *AllWorld) WritableDir(key string) string {
	return filepath.Join(a.snapshotDir(key), "fs")
}

// WhiteoutFormat returns the whiteout handling strategy for the AllWorld driver.
func (a *AllWorld) WhiteoutFormat() archive.WhiteoutFormat {
	return archive.WhiteoutOverlay
}

// Internal helpers.

func (a *AllWorld) snapshotsDir() string          { return filepath.Join(a.root, "prim", "snapshots") }
func (a *AllWorld) snapshotDir(key string) string { return filepath.Join(a.snapshotsDir(), key) }

func (a *AllWorld) checkNotExists(key string) error {
	if _, err := a.fs.Stat(a.snapshotDir(key)); err == nil {
		return fmt.Errorf("allworld: %s: %w", key, ErrSnapshotAlreadyExists)
	}
	return nil
}

func (a *AllWorld) hasDependents(key string) (bool, error) {
	des, err := a.fs.ReadDir(a.snapshotsDir())
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
	data, err := a.fs.ReadFile(filepath.Join(a.snapshotDir(key), "meta.json"))
	if err != nil {
		return m, err
	}
	if jsonErr := json.Unmarshal(data, &m); jsonErr != nil {
		return m, jsonErr
	}
	return m, nil
}

func (a *AllWorld) writeMeta(dir string, meta VFSMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal meta: %w", err)
	}
	return a.fs.WriteFile(filepath.Join(dir, "meta.json"), data, filePerm)
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
func ProbeOverlay(ctx context.Context, dir string, mnt Mounter) error {
	if mnt == nil {
		mnt = &RealMounter{}
	}
	const (
		lowerDir = "lower"
		upperDir = "upper"
		workDir  = "work"
		mergeDir = "merge"
	)

	for _, d := range []string{lowerDir, upperDir, workDir, mergeDir} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o700); err != nil {
			return fmt.Errorf("probe: mkdir %s: %w", d, err)
		}
	}

	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s",
		filepath.Join(dir, lowerDir),
		filepath.Join(dir, upperDir),
		filepath.Join(dir, workDir),
	)

	target := filepath.Join(dir, mergeDir)
	if err := mnt.Mount(ctx, "", target, "overlay", 0, opts); err != nil {
		return fmt.Errorf("probe: mount overlay: %w", err)
	}

	return nil
}

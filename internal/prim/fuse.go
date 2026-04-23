package prim

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rodrigo-baliza/maestro/internal/white"
	"github.com/rodrigo-baliza/maestro/pkg/archive"
)

// FuseOverlay implements the [Prim] interface using fuse-overlayfs.
// It is the rootless fallback for OverlayFS when native kernel support is missing.
//
// Storage layout (under root):
//
//	prim/
//	└── snapshots/
//	    └── <key>/
//	        ├── meta.json          — snapshot metadata
//	        ├── work/              — fuse-overlayfs workdir
//	        └── fs/                — active/committed filesystem content
type FuseOverlay struct {
	root string
	mu   sync.RWMutex
	fs   FS
	mnt  Mounter
}

// NewFuseOverlay returns a new FuseOverlay driver rooted at root.
func NewFuseOverlay(root string) (*FuseOverlay, error) {
	f := &FuseOverlay{
		root: root,
		fs:   RealFS{},
		mnt:  &RealMounter{},
	}
	if err := f.fs.MkdirAll(f.snapshotsDir(), dirPerm); err != nil {
		return nil, fmt.Errorf("fuse-overlayfs: create snapshots dir: %w", err)
	}
	return f, nil
}

// WithFS sets the filesystem implementation.
func (f *FuseOverlay) WithFS(fs FS) *FuseOverlay {
	f.fs = fs
	return f
}

// WithMounter sets the mounter implementation.
func (f *FuseOverlay) WithMounter(mnt Mounter) *FuseOverlay {
	f.mnt = mnt
	return f
}

// Prepare creates a writable (KindActive) snapshot with the given parent.
func (f *FuseOverlay) Prepare(ctx context.Context, key, parent string) ([]Mount, error) {
	return prepareHelper(
		ctx,
		f.fs,
		&f.mu,
		f.snapshotDir,
		f.checkNotExists,
		f.writeMeta,
		f.mounts,
		key,
		parent,
	)
}

// View creates a read-only (KindView) snapshot.
func (f *FuseOverlay) View(ctx context.Context, key, parent string) ([]Mount, error) {
	return viewHelper(
		ctx,
		f.fs,
		&f.mu,
		f.snapshotDir,
		f.checkNotExists,
		f.writeMeta,
		f.mounts,
		key,
		parent,
		"fuse-overlayfs",
	)
}

// Commit seals an active snapshot into an immutable committed snapshot.
func (f *FuseOverlay) Commit(ctx context.Context, name, key string) error {
	// Reuse AllWorld logic for directory structures
	a, err := NewAllWorld(f.root)
	if err != nil {
		return err
	}
	return a.WithFS(f.fs).Commit(ctx, name, key)
}

// Remove removes a snapshot and releases all storage.
func (f *FuseOverlay) Remove(ctx context.Context, key string) error {
	a, err := NewAllWorld(f.root)
	if err != nil {
		return err
	}
	return a.WithFS(f.fs).Remove(ctx, key)
}

// Walk calls fn for every snapshot in the store, in insertion order.
func (f *FuseOverlay) Walk(ctx context.Context, fn func(Info) error) error {
	a, err := NewAllWorld(f.root)
	if err != nil {
		return err
	}
	return a.WithFS(f.fs).Walk(ctx, fn)
}

// Usage reports disk consumption for a snapshot.
func (f *FuseOverlay) Usage(ctx context.Context, key string) (Usage, error) {
	a, err := NewAllWorld(f.root)
	if err != nil {
		return Usage{}, err
	}
	return a.WithFS(f.fs).Usage(ctx, key)
}

// WritableDir returns the absolute path to the writable directory for the given snapshot.
func (f *FuseOverlay) WritableDir(key string) string {
	return filepath.Join(f.snapshotDir(key), "fs")
}

// WhiteoutFormat returns the whiteout handling strategy for the FuseOverlay driver.
func (f *FuseOverlay) WhiteoutFormat() archive.WhiteoutFormat {
	return archive.WhiteoutOverlay
}

// Internal helpers.

func (f *FuseOverlay) snapshotsDir() string          { return filepath.Join(f.root, "prim", "snapshots") }
func (f *FuseOverlay) snapshotDir(key string) string { return filepath.Join(f.snapshotsDir(), key) }

func (f *FuseOverlay) checkNotExists(key string) error {
	if _, err := f.fs.Stat(f.snapshotDir(key)); err == nil {
		return fmt.Errorf("fuse-overlayfs: %s: %w", key, ErrSnapshotAlreadyExists)
	}
	return nil
}

func (f *FuseOverlay) readMeta(key string) (VFSMeta, error) {
	a := &AllWorld{root: f.root, fs: f.fs}
	return a.readMeta(key)
}

func (f *FuseOverlay) writeMeta(dir string, meta VFSMeta) error {
	a := &AllWorld{root: f.root, fs: f.fs}
	return a.writeMeta(dir, meta)
}

func (f *FuseOverlay) mounts(key, parent string) ([]Mount, error) {
	// Root layer (no parent).
	if parent == "" {
		return []Mount{{
			Type:   "bind",
			Source: filepath.Join(f.snapshotDir(key), "fs"),
			Options: []string{
				"bind",
				"rw",
			},
		}}, nil
	}

	// Fuse-overlay chain.
	var layers []string
	curr := parent
	for curr != "" {
		layers = append(layers, filepath.Join(f.snapshotDir(curr), "fs"))
		meta, err := f.readMeta(curr)
		if err != nil {
			return nil, fmt.Errorf("read meta for %s: %w", curr, err)
		}
		curr = meta.Parent
	}

	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s",
		strings.Join(layers, ":"),
		filepath.Join(f.snapshotDir(key), "fs"),
		filepath.Join(f.snapshotDir(key), "work"),
	)

	// In rootless mode, we must provide UID/GID mappings to fuse-overlayfs
	// so it can handle multiple UIDs (like Nginx's UID 101) correctly.
	username := "userone" // Fallback
	if u, err := user.Current(); err == nil {
		username = u.Username
	}

	//nolint:gosec // G115: UIDs are within uint32 range on Linux
	if uids, gids, errMaps := white.BuildIDMappings(username,
		uint32(os.Getuid()), uint32(os.Getgid())); errMaps == nil {
		var uidStr, gidStr []string
		for _, m := range uids {
			uidStr = append(uidStr, fmt.Sprintf("%d:%d:%d", m.ContainerID, m.HostID, m.Size))
		}
		for _, m := range gids {
			gidStr = append(gidStr, fmt.Sprintf("%d:%d:%d", m.ContainerID, m.HostID, m.Size))
		}
		opts += fmt.Sprintf(",uidmapping=%s,gidmapping=%s",
			strings.Join(uidStr, ":"), strings.Join(gidStr, ":"))
	}

	return []Mount{{
		Type:    "fuse-overlayfs",
		Source:  string(DriverFuseOverlay),
		Options: strings.Split(opts, ","),
	}}, nil
}

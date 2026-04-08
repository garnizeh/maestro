// Package prim implements the Prim storage management layer for Maestro.
//
// Named after the Prim from The Dark Tower — the primordial magical chaos
// from which all of reality was shaped. Prim shapes image layers and container
// root filesystems into usable storage through a snapshotter abstraction.
//
// The Prim interface is implemented by:
//   - AllWorld (OverlayFS): kernel-native overlay, primary driver
//   - fuse-overlayfs: rootless fallback for kernels 4.18–5.12
//   - VFS: copy-on-write via full copy, universal fallback
//
// Snapshots follow an immutable chain:
//
//	L1 (committed) → L2 (committed) → ... → container-rw (active/writable)
package prim

import (
	"context"
	"errors"
)

const (
	// dirPerm is the default permission for snapshot directories.
	dirPerm = 0o700
	// filePerm is the default permission for snapshot metadata and log files.
	filePerm = 0o600
)

// ErrSnapshotNotFound is returned when a snapshot key is not present.
var ErrSnapshotNotFound = errors.New("snapshot not found")

// ErrSnapshotHasDependents is returned when a snapshot cannot be removed
// because other snapshots reference it as their parent.
var ErrSnapshotHasDependents = errors.New("snapshot has dependents")

// ErrCommitOnReadOnly is returned when Commit is called on a view (read-only) snapshot.
var ErrCommitOnReadOnly = errors.New("cannot commit a read-only view snapshot")

// ErrSnapshotAlreadyExists is returned when a key already exists.
var ErrSnapshotAlreadyExists = errors.New("snapshot already exists")

// Kind is the snapshot type.
type Kind int

const (
	// KindCommitted is an immutable, sealed snapshot (an image layer).
	KindCommitted Kind = iota
	// KindActive is a writable snapshot (a container's upper layer).
	KindActive
	// KindView is a read-only snapshot (a mounted image layer view).
	KindView
)

// String returns a human-readable name for the Kind.
func (k Kind) String() string {
	switch k {
	case KindCommitted:
		return "committed"
	case KindActive:
		return "active"
	case KindView:
		return "view"
	default:
		return "unknown"
	}
}

// Mount describes how to mount a snapshot's filesystem.
type Mount struct {
	// Type is the mount type (e.g. "overlay", "bind", "").
	Type string
	// Source is the device or directory to mount.
	Source string
	// Options are comma-separated mount options.
	Options []string
}

// Info holds metadata for a single snapshot.
type Info struct {
	// Key is the unique identifier for this snapshot.
	Key string
	// Parent is the key of the parent snapshot (empty for root snapshots).
	Parent string
	// Kind indicates whether the snapshot is committed, active, or view.
	Kind Kind
}

// Usage reports disk consumption for a snapshot.
type Usage struct {
	// Inodes is the number of inodes used.
	Inodes int64
	// Size is the total disk space in bytes.
	Size int64
}

// Prim abstracts filesystem operations for image layers and container rootfs.
// Different drivers shape the Prim differently, but the interface is the same.
//
// All methods are safe to call concurrently with distinct keys. Concurrent
// operations on the same key with mutations must be serialised by the caller.
type Prim interface {
	// Prepare creates a writable (KindActive) snapshot with the given parent.
	// Returns mount instructions for the writable filesystem.
	// parent may be empty for a root snapshot with no base layers.
	Prepare(ctx context.Context, key, parent string) ([]Mount, error)

	// View creates a read-only (KindView) snapshot.
	// Returns mount instructions for the read-only filesystem.
	View(ctx context.Context, key, parent string) ([]Mount, error)

	// Commit seals an active snapshot into an immutable committed snapshot.
	// name becomes the stable, referenceable identifier for the committed layer.
	Commit(ctx context.Context, name, key string) error

	// Remove removes a snapshot and releases all storage.
	// Returns ErrSnapshotHasDependents if children still reference it.
	Remove(ctx context.Context, key string) error

	// Walk calls fn for every snapshot in the store, in insertion order.
	Walk(ctx context.Context, fn func(Info) error) error

	// Usage returns disk consumption for the snapshot identified by key.
	Usage(ctx context.Context, key string) (Usage, error)
}

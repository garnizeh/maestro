package prim

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/rs/zerolog/log"

	"github.com/rodrigo-baliza/maestro/internal/bin"
)

//nolint:gochecknoglobals // findBinary is a variable for testing purposes.
var findBinary = bin.Find

// DriverKind identifies the snapshotter driver implementation.
type DriverKind string

const (
	// DriverAllWorld is the OverlayFS (AllWorld) driver.
	DriverAllWorld DriverKind = "overlay"
	// DriverVFS is the full-copy fallback driver.
	DriverVFS DriverKind = "vfs"
	// DriverFuseOverlay is the FUSE-based overlay driver.
	DriverFuseOverlay DriverKind = "fuse-overlayfs"
)

// DetectResult holds the detection outcome.
type DetectResult struct {
	// Driver is the kind of driver selected.
	Driver DriverKind
	// Prim is the initialised driver instance.
	Prim Prim
	// Rootless indicates whether the driver is running in rootless mode.
	Rootless bool
	// Mounter is the initialized mounter instance (potentially with a binary path).
	Mounter Mounter
}

// Detect selects and initialises the best available snapshotter driver.
// It probes for OverlayFS support first; if the probe fails or the caller
// passes forceVFS=true, it falls back to the VFS driver.
//
// If in rootless mode and native OverlayFS fails, it attempts to use fuse-overlayfs
// if the binary is available.
func Detect(
	ctx context.Context,
	root string,
	forceVFS bool,
	mnt Mounter,
	fs FS,
) (*DetectResult, error) {
	rootless := isRootless()

	if mnt == nil {
		mnt = &RealMounter{}
	}
	if fs == nil {
		fs = RealFS{}
	}

	if forceVFS {
		return detectVFS(root, rootless)
	}

	// 1. Probe for native OverlayFS.
	res, err := detectAllWorld(ctx, root, mnt, fs, rootless)
	if err == nil {
		res.Mounter = mnt
		return res, nil
	}

	// 2. If rootless and native OverlayFS failed, try fuse-overlayfs.
	if rootless {
		if resFuse, errFuse := detectFuseOverlay(root, mnt, rootless); errFuse == nil {
			resFuse.Mounter = mnt
			return resFuse, nil
		}
	}

	// 3. Fallback to VFS.
	resVFS, errVFS := detectVFS(root, rootless)
	if errVFS == nil {
		resVFS.Mounter = mnt
	}
	return resVFS, errVFS
}

func detectAllWorld(
	ctx context.Context,
	root string,
	mnt Mounter,
	fs FS,
	rootless bool,
) (*DetectResult, error) {
	tmp, err := fs.MkdirTemp("", "maestro-prim-probe-*")
	if err != nil {
		return nil, fmt.Errorf("detect: create probe dir: %w", err)
	}
	defer func() {
		if rmErr := fs.RemoveAll(tmp); rmErr != nil {
			log.Debug().
				Err(rmErr).
				Str("tmp", tmp).
				Msg("detect: failed to remove probe temp directory")
		}
	}()

	if probeErr := ProbeOverlay(ctx, tmp, mnt); probeErr == nil {
		a, initErr := NewAllWorld(root)
		if initErr != nil {
			return nil, fmt.Errorf("detect: init allworld: %w", initErr)
		}
		return &DetectResult{Driver: DriverAllWorld, Prim: a, Rootless: rootless}, nil
	}
	return nil, errors.New("detect: native overlay not available")
}

func detectFuseOverlay(root string, mnt Mounter, rootless bool) (*DetectResult, error) {
	p, err := findBinary(string(DriverFuseOverlay))
	if err != nil {
		return nil, err
	}

	f, initErr := NewFuseOverlay(root)
	if initErr != nil {
		return nil, fmt.Errorf("detect: init fuse-overlayfs: %w", initErr)
	}

	// Update mounter if it's the RealMounter to use the found path
	if rm, ok := mnt.(*RealMounter); ok {
		rm.BinaryPath = p
		f.WithMounter(rm)
	}

	return &DetectResult{Driver: DriverFuseOverlay, Prim: f, Rootless: rootless}, nil
}

func detectVFS(root string, rootless bool) (*DetectResult, error) {
	v, err := NewVFS(root)
	if err != nil {
		return nil, fmt.Errorf("detect: init vfs: %w", err)
	}
	return &DetectResult{Driver: DriverVFS, Prim: v, Rootless: rootless}, nil
}

func isRootless() bool {
	return os.Geteuid() != 0
}

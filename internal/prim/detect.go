package prim

import (
	"fmt"
	"os"
)

// DriverKind identifies the snapshotter driver implementation.
type DriverKind string

const (
	// DriverAllWorld is the OverlayFS (AllWorld) driver.
	DriverAllWorld DriverKind = "overlay"
	// DriverVFS is the full-copy fallback driver.
	DriverVFS DriverKind = "vfs"
)

// DetectResult holds the detection outcome.
type DetectResult struct {
	// Driver is the kind of driver selected.
	Driver DriverKind
	// Prim is the initialised driver instance.
	Prim Prim
	// Rootless indicates whether the driver is running in rootless mode.
	Rootless bool
}

// Detect selects and initialises the best available snapshotter driver.
// It probes for OverlayFS support first; if the probe fails or the caller
// passes forceVFS=true, it falls back to the VFS driver.
//
// The probeDir is a temporary directory used for the OverlayFS mount test.
// If probeDir is empty, [os.MkdirTemp] is used.
func Detect(
	root string,
	forceVFS bool,
	mountFn func(source, target, fstype string, flags uintptr, data string) error,
) (*DetectResult, error) {
	rootless := isRootless()

	if forceVFS {
		v, err := NewVFS(root)
		if err != nil {
			return nil, fmt.Errorf("detect: init vfs: %w", err)
		}
		return &DetectResult{Driver: DriverVFS, Prim: v, Rootless: rootless}, nil
	}

	// Probe for OverlayFS.
	tmp, err := os.MkdirTemp("", "maestro-prim-probe-*")
	if err != nil {
		return nil, fmt.Errorf("detect: create probe dir: %w", err)
	}
	defer os.RemoveAll(tmp)

	if probeErr := ProbeOverlay(tmp, mountFn); probeErr == nil {
		a, initErr := NewAllWorld(root, mountFn)
		if initErr != nil {
			return nil, fmt.Errorf("detect: init allworld: %w", initErr)
		}
		return &DetectResult{Driver: DriverAllWorld, Prim: a, Rootless: rootless}, nil
	}

	// Fallback to VFS.
	v, err := NewVFS(root)
	if err != nil {
		return nil, fmt.Errorf("detect: init vfs: %w", err)
	}
	return &DetectResult{Driver: DriverVFS, Prim: v, Rootless: rootless}, nil
}

func isRootless() bool {
	return os.Geteuid() != 0
}

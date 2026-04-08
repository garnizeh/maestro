//go:build !linux

package prim

import (
	"errors"
)

// ErrUnsupportedOperatingSystem is returned when an operation is not supported
// on the current OS.
var ErrUnsupportedOperatingSystem = errors.New("operation not supported on this OS")

// osSysMount is a stub for non-Linux platforms.
func osSysMount(_, _, _ string, _ uintptr, _ string) error {
	return ErrUnsupportedOperatingSystem
}

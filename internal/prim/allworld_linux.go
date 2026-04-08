package prim

import (
	"golang.org/x/sys/unix"
)

// osSysMount is the production mount function for Linux.
// It uses unix.Mount directly.
func osSysMount(source, target, fstype string, flags uintptr, data string) error {
	return unix.Mount(source, target, fstype, flags, data)
}

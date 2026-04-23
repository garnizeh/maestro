//go:build linux

package beam

import "golang.org/x/sys/unix"

type realSyscallMounter struct{}

func (realSyscallMounter) Unshare(f int) error { return unix.Unshare(f) }
func (realSyscallMounter) Mount(s, t, ft string, fl uintptr, d string) error {
	return unix.Mount(s, t, ft, fl, d)
}
func (realSyscallMounter) Unmount(t string, f int) error { return unix.Unmount(t, f) }

//go:build !linux

package beam

import "fmt"

type realSyscallMounter struct{}

func (realSyscallMounter) Unshare(_ int) error { return fmt.Errorf("not supported") }
func (realSyscallMounter) Mount(_, _, _ string, _ uintptr, _ string) error {
	return fmt.Errorf("not supported")
}
func (realSyscallMounter) Unmount(_ string, _ int) error { return fmt.Errorf("not supported") }

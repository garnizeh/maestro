//go:build !linux

package beam

import "fmt"

func newDefaultMounter() Mounter {
	return &unsupportedMounter{}
}

type unsupportedMounter struct{}

func (m *unsupportedMounter) NewNS(nsPath string) (string, error) {
	return "", fmt.Errorf("network namespaces are only supported on Linux")
}

func (m *unsupportedMounter) DeleteNS(_ string) error {
	return fmt.Errorf("network namespaces are only supported on Linux")
}

// RealMounter is a stub to allow compilation of Todash.WithFS on non-Linux platforms.
type RealMounter struct {
	sys      SyscallMounter
	fs       FS
	rootless bool
}

func (m *RealMounter) NewNS(
	nsPath string,
) (string, error) {
	return "", fmt.Errorf("not supported")
}

func (m *RealMounter) SetFS(fs FS) {
	m.fs = fs
}
func (m *RealMounter) DeleteNS(_ string) error { return fmt.Errorf("not supported") }

package bin

import _ "embed"

//go:embed assets/fuse-overlayfs
var fuseOverlayFS []byte

//go:embed assets/pasta
var pasta []byte

// GetEmbedded returns the embedded binary data by name.
func GetEmbedded(name string) ([]byte, bool) {
	switch name {
	case "fuse-overlayfs":
		return fuseOverlayFS, true
	case "pasta":
		return pasta, true
	default:
		return nil, false
	}
}

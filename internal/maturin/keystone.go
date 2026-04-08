package maturin

import (
	"errors"
	"fmt"
	"runtime"
	"strings"

	ggcr "github.com/google/go-containerregistry/pkg/v1"
)

// ErrNoPlatformMatch is returned by [keystoneSelect] when no manifest in an
// OCI Image Index matches the requested platform.
var ErrNoPlatformMatch = errors.New("no compatible platform found")

// platformParts is the maximum number of slash-separated components in an
// "os/arch[/variant]" platform string.
const platformParts = 3

// hostPlatform returns the [ggcr.Platform] for the current host.
func hostPlatform() ggcr.Platform {
	return ggcr.Platform{OS: runtime.GOOS, Architecture: runtime.GOARCH}
}

// parsePlatform parses an optional "os/arch[/variant]" platform string.
// Returns the host platform when s is empty.
func parsePlatform(s string) (ggcr.Platform, error) {
	if s == "" {
		return hostPlatform(), nil
	}
	parts := strings.SplitN(s, "/", platformParts)
	if len(parts) < 2 { //nolint:mnd // 2 = minimum os/arch components
		return ggcr.Platform{}, fmt.Errorf("invalid platform %q: expected os/arch[/variant]", s)
	}
	p := ggcr.Platform{OS: parts[0], Architecture: parts[1]}
	if len(parts) == 3 { //nolint:mnd // 3 = os/arch/variant
		p.Variant = parts[2]
	}
	return p, nil
}

// keystoneSelect selects the manifest descriptor from idx that best matches
// want. It returns the first exact match (OS + arch + variant) or, if no exact
// match exists, the first entry with matching OS and architecture (any
// variant). Returns [ErrNoPlatformMatch] if no entry matches.
func keystoneSelect(idx ggcr.ImageIndex, want ggcr.Platform) (ggcr.Descriptor, error) {
	manifest, manifestErr := idx.IndexManifest()
	if manifestErr != nil {
		return ggcr.Descriptor{}, fmt.Errorf("fetch index manifest: %w", manifestErr)
	}

	var fallback *ggcr.Descriptor
	platforms := make([]string, 0, len(manifest.Manifests))

	for i := range manifest.Manifests {
		m := &manifest.Manifests[i]
		if m.Platform == nil {
			continue
		}
		plat := m.Platform
		platforms = append(platforms, plat.OS+"/"+plat.Architecture+variantSuffix(plat.Variant))

		if plat.OS != want.OS || plat.Architecture != want.Architecture {
			continue
		}

		// Exact match: OS + arch + variant (or no variant preference).
		if want.Variant == "" || plat.Variant == want.Variant {
			return *m, nil
		}

		// Partial match: OS + arch, any variant — save as fallback.
		if fallback == nil {
			d := *m
			fallback = &d
		}
	}

	if fallback != nil {
		return *fallback, nil
	}

	return ggcr.Descriptor{}, fmt.Errorf("%w: %s/%s, available: [%s]",
		ErrNoPlatformMatch, want.OS, want.Architecture, strings.Join(platforms, ", "))
}

// variantSuffix returns "/variant" when v is non-empty, otherwise "".
func variantSuffix(v string) string {
	if v == "" {
		return ""
	}
	return "/" + v
}

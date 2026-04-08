package shardik

import "strings"

// ThinnyConfig holds mirror/proxy entries read from katet.toml.
// Each key is a registry hostname (e.g. "docker.io"), and the value is
// the mirror URL to try first (e.g. "mirror.example.com").
type ThinnyConfig struct {
	Mirrors map[string]string
}

// Thinny resolves a registry reference through configured mirrors.
// Named after Thinny — the unstable places where worlds grow thin in
// The Dark Tower.
type Thinny struct {
	cfg ThinnyConfig
}

// NewThinny creates a Thinny mirror resolver.
func NewThinny(cfg ThinnyConfig) *Thinny {
	return &Thinny{cfg: cfg}
}

// Resolve returns the preferred registry to use for the given original registry
// hostname. If a mirror is configured it returns the mirror; otherwise it
// returns the original registry unchanged.
func (t *Thinny) Resolve(registry string) string {
	if t.cfg.Mirrors == nil {
		return registry
	}
	bare := bareHost(registry)
	if mirror, ok := t.cfg.Mirrors[bare]; ok {
		return mirror
	}
	if mirror, ok := t.cfg.Mirrors[registry]; ok {
		return mirror
	}
	return registry
}

// RewriteReference rewrites a full image reference to use the configured mirror
// when one exists. For example "docker.io/library/nginx:latest" becomes
// "mirror.example.com/library/nginx:latest" if docker.io → mirror.example.com.
func (t *Thinny) RewriteReference(ref string) string {
	if t.cfg.Mirrors == nil {
		return ref
	}
	for orig, mirror := range t.cfg.Mirrors {
		if suffix, ok := strings.CutPrefix(ref, orig+"/"); ok {
			return mirror + "/" + suffix
		}
		bare := bareHost(orig)
		if suffix, ok := strings.CutPrefix(ref, bare+"/"); ok {
			return mirror + "/" + suffix
		}
	}
	return ref
}

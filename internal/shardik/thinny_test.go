package shardik_test

import (
	"testing"

	"github.com/garnizeh/maestro/internal/shardik"
)

// ── Task #27 — Thinny mirror resolution ──────────────────────────────────────

func TestThinny_ResolvesMirror(t *testing.T) {
	thinny := shardik.NewThinny(shardik.ThinnyConfig{
		Mirrors: map[string]string{
			"docker.io": "mirror.example.com",
		},
	})

	got := thinny.Resolve("docker.io")
	if got != "mirror.example.com" {
		t.Errorf("Resolve = %q, want %q", got, "mirror.example.com")
	}
}

func TestThinny_ReturnsOriginalWhenNoMirror(t *testing.T) {
	thinny := shardik.NewThinny(shardik.ThinnyConfig{
		Mirrors: map[string]string{
			"docker.io": "mirror.example.com",
		},
	})

	got := thinny.Resolve("ghcr.io")
	if got != "ghcr.io" {
		t.Errorf("Resolve = %q, want %q", got, "ghcr.io")
	}
}

func TestThinny_ReturnsOriginalWhenNoMirrorsConfigured(t *testing.T) {
	thinny := shardik.NewThinny(shardik.ThinnyConfig{})

	got := thinny.Resolve("docker.io")
	if got != "docker.io" {
		t.Errorf("Resolve = %q, want %q", got, "docker.io")
	}
}

func TestThinny_RewriteReference_WithMirror(t *testing.T) {
	thinny := shardik.NewThinny(shardik.ThinnyConfig{
		Mirrors: map[string]string{
			"docker.io": "mirror.example.com",
		},
	})

	ref := "docker.io/library/nginx:latest"
	got := thinny.RewriteReference(ref)
	want := "mirror.example.com/library/nginx:latest"
	if got != want {
		t.Errorf("RewriteReference = %q, want %q", got, want)
	}
}

func TestThinny_RewriteReference_NoMirror(t *testing.T) {
	thinny := shardik.NewThinny(shardik.ThinnyConfig{
		Mirrors: map[string]string{
			"docker.io": "mirror.example.com",
		},
	})

	ref := "ghcr.io/owner/repo:tag"
	got := thinny.RewriteReference(ref)
	if got != ref {
		t.Errorf("RewriteReference = %q, want %q", got, ref)
	}
}

func TestThinny_RewriteReference_NilMirrors(t *testing.T) {
	thinny := shardik.NewThinny(shardik.ThinnyConfig{Mirrors: nil})

	ref := "docker.io/library/nginx:latest"
	got := thinny.RewriteReference(ref)
	if got != ref {
		t.Errorf("RewriteReference with nil mirrors = %q, want %q", got, ref)
	}
}

func TestThinny_Resolve_WithPortUsesKeyBareHost(t *testing.T) {
	// Mirror keyed as "docker.io" should match when registry is "docker.io:443".
	thinny := shardik.NewThinny(shardik.ThinnyConfig{
		Mirrors: map[string]string{
			"docker.io": "mirror.example.com",
		},
	})

	// Exact key match with port won't exist, but bareHost("docker.io:443") == "docker.io".
	got := thinny.Resolve("docker.io:443")
	if got != "mirror.example.com" {
		t.Errorf("Resolve(host:port) = %q, want %q", got, "mirror.example.com")
	}
}

func TestThinny_Resolve_FullKeyMatch(t *testing.T) {
	// Mirror keyed as "docker.io:443" (with port): bare="docker.io" won't match;
	// the second lookup with the full registry string "docker.io:443" must match.
	thinny := shardik.NewThinny(shardik.ThinnyConfig{
		Mirrors: map[string]string{
			"docker.io:443": "mirror.example.com",
		},
	})

	got := thinny.Resolve("docker.io:443")
	if got != "mirror.example.com" {
		t.Errorf("Resolve = %q, want %q", got, "mirror.example.com")
	}
}

func TestThinny_RewriteReference_BareHostMatch(t *testing.T) {
	// Mirror keyed as "docker.io:443" (with port) should match ref starting with "docker.io/".
	thinny := shardik.NewThinny(shardik.ThinnyConfig{
		Mirrors: map[string]string{
			"docker.io:443": "mirror.example.com",
		},
	})

	ref := "docker.io/library/nginx:latest"
	want := "mirror.example.com/library/nginx:latest"
	got := thinny.RewriteReference(ref)
	if got != want {
		t.Errorf("RewriteReference = %q, want %q", got, want)
	}
}

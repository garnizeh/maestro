package tower_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/garnizeh/maestro/internal/tower"
)

func TestConfigPath_Default(t *testing.T) {
	// Unset XDG so we get the HOME-based default.
	t.Setenv("XDG_CONFIG_HOME", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	path, err := tower.ConfigPath("")
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	want := filepath.Join(home, ".config", "maestro", "katet.toml")
	if path != want {
		t.Errorf("got %s, want %s", path, want)
	}
}

func TestConfigPath_XDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/xdg")
	path, err := tower.ConfigPath("")
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	want := "/custom/xdg/maestro/katet.toml"
	if path != want {
		t.Errorf("got %s, want %s", path, want)
	}
}

func TestConfigPath_Override(t *testing.T) {
	path, err := tower.ConfigPath("/explicit/path.toml")
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if path != "/explicit/path.toml" {
		t.Errorf("got %s, want /explicit/path.toml", path)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	// Point at a non-existent file so we get pure defaults.
	dir := t.TempDir()
	cfg, err := tower.LoadConfig(filepath.Join(dir, "nonexistent.toml"))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// We expect defaults as defined in tower/config.go:defaults()
	// Since we can't easily reproduce the exact paths (HomeDir based),
	// we compare the key fields.
	if cfg.Runtime.Default != "auto" {
		t.Errorf("Runtime.Default = %s, want auto", cfg.Runtime.Default)
	}
	if !cfg.Security.Rootless {
		t.Error("Security.Rootless should be true by default")
	}
	if cfg.Log.Level != "warn" {
		t.Errorf("Log.Level = %s, want warn", cfg.Log.Level)
	}
}

func TestLoadConfig_FromFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "katet.toml")
	content := `[runtime]
default = "runc"
[security]
rootless = false
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := tower.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Runtime.Default != "runc" {
		t.Errorf("Runtime.Default = %s, want runc", cfg.Runtime.Default)
	}
	if cfg.Security.Rootless {
		t.Error("Security.Rootless should be false per config file")
	}
}

func TestLoadConfig_EnvOverride(t *testing.T) {
	t.Setenv("MAESTRO_RUNTIME", "youki")
	dir := t.TempDir()
	cfg, err := tower.LoadConfig(filepath.Join(dir, "nonexistent.toml"))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Runtime.Default != "youki" {
		t.Errorf("Runtime.Default = %s, want youki (from env)", cfg.Runtime.Default)
	}
}

func TestEnsureDefault_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "sub", "katet.toml")

	created, path, err := tower.EnsureDefault(cfgPath)
	if err != nil {
		t.Fatalf("EnsureDefault: %v", err)
	}
	if !created {
		t.Error("expected created=true on first run")
	}
	if path != cfgPath {
		t.Errorf("path = %s, want %s", path, cfgPath)
	}
	if _, statErr := os.Stat(cfgPath); statErr != nil {
		t.Errorf("config file not created: %v", statErr)
	}
}

func TestEnsureDefault_IdempotentOnSecondRun(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "katet.toml")

	created, _, err := tower.EnsureDefault(cfgPath)
	if err != nil || !created {
		t.Fatalf("first EnsureDefault: err=%v created=%v", err, created)
	}

	created, _, err = tower.EnsureDefault(cfgPath)
	if err != nil {
		t.Fatalf("second EnsureDefault: %v", err)
	}
	if created {
		t.Error("expected created=false on second run")
	}
}

func TestConfigToTOML(t *testing.T) {
	dir := t.TempDir()
	cfg, err := tower.LoadConfig(filepath.Join(dir, "none.toml"))
	if err != nil {
		t.Fatal(err)
	}
	out := cfg.ToTOML()
	if out == "" {
		t.Error("ToTOML returned empty string")
	}
	// Basic sanity: must contain expected keys.
	for _, key := range []string{"[runtime]", "[storage]", "[network]", "[security]", "[log]"} {
		if !contains(out, key) {
			t.Errorf("ToTOML output missing %q", key)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && func() bool {
		for i := range s {
			if i+len(sub) <= len(s) && s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}()
}

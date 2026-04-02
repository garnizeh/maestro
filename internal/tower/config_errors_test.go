package tower_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rodrigo-baliza/maestro/internal/tower"
)

func TestLoadConfig_InvalidTOML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "katet.toml")
	if err := os.WriteFile(cfgPath, []byte("[[invalid toml"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := tower.LoadConfig(cfgPath)
	if err == nil {
		t.Error("expected error for invalid TOML")
	}
}

func TestLoadConfig_UnreadableFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission error as root")
	}
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "katet.toml")
	if err := os.WriteFile(cfgPath, []byte("[runtime]\ndefault = 'runc'\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(cfgPath, 0o600) })

	_, err := tower.LoadConfig(cfgPath)
	if err == nil {
		t.Error("expected error for unreadable config file")
	}
}

func TestApplyEnvOverrides_AllVars(t *testing.T) {
	t.Setenv("MAESTRO_RUNTIME", "crun")
	t.Setenv("MAESTRO_STORAGE_DRIVER", "btrfs")
	t.Setenv("MAESTRO_LOG_LEVEL", "debug")
	t.Setenv("MAESTRO_ROOT", "/custom/root")
	t.Setenv("MAESTRO_ROOTLESS", "false")

	dir := t.TempDir()
	cfg, err := tower.LoadConfig(filepath.Join(dir, "none.toml"))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Runtime.Default != "crun" {
		t.Errorf("Runtime.Default = %q, want crun", cfg.Runtime.Default)
	}
	if cfg.Storage.Driver != "btrfs" {
		t.Errorf("Storage.Driver = %q, want btrfs", cfg.Storage.Driver)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want debug", cfg.Log.Level)
	}
	if cfg.State.Root != "/custom/root" {
		t.Errorf("State.Root = %q, want /custom/root", cfg.State.Root)
	}
	if cfg.Security.Rootless {
		t.Error("Security.Rootless should be false when MAESTRO_ROOTLESS=false")
	}
}

func TestApplyEnvOverrides_RootlessZero(t *testing.T) {
	t.Setenv("MAESTRO_ROOTLESS", "0")
	dir := t.TempDir()
	cfg, err := tower.LoadConfig(filepath.Join(dir, "none.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Security.Rootless {
		t.Error("Security.Rootless should be false when MAESTRO_ROOTLESS=0")
	}
}

func TestApplyEnvOverrides_RootlessTrue(t *testing.T) {
	t.Setenv("MAESTRO_ROOTLESS", "true")
	dir := t.TempDir()
	cfg, err := tower.LoadConfig(filepath.Join(dir, "none.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Security.Rootless {
		t.Error("Security.Rootless should be true when MAESTRO_ROOTLESS=true")
	}
}

func TestEnsureDefault_MkdirError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission error as root")
	}
	// Unwriteable parent so MkdirAll cannot create the config directory.
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })

	cfgPath := filepath.Join(parent, "subdir", "katet.toml")
	_, _, err := tower.EnsureDefault(cfgPath)
	if err == nil {
		t.Error("expected error when config directory cannot be created")
	}
}

func TestEnsureDefault_WriteError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission error as root")
	}
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "maestro")
	// Create the dir, then make it read-only.
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cfgDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(cfgDir, 0o700) })

	cfgPath := filepath.Join(cfgDir, "katet.toml")
	_, _, err := tower.EnsureDefault(cfgPath)
	if err == nil {
		t.Error("expected error when config file cannot be written to read-only dir")
	}
}

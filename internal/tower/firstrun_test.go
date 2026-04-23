package tower_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/garnizeh/maestro/internal/tower"
)

func TestFirstRun_CreatesConfigAndReturnsTrue(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "katet.toml")

	created, err := tower.FirstRun(cfgPath, "")
	if err != nil {
		t.Fatalf("FirstRun: %v", err)
	}
	if !created {
		t.Error("expected created=true on first run")
	}
	if _, statErr := os.Stat(cfgPath); statErr != nil {
		t.Errorf("config file not found after first run: %v", statErr)
	}
}

func TestFirstRun_EnsureDefaultError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission error as root")
	}
	// Make the parent unwriteable so EnsureDefault cannot create the config dir.
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(parent, 0o700); err != nil {
			t.Fatal(err)
		}
	})

	cfgPath := filepath.Join(parent, "subdir", "katet.toml")
	_, err := tower.FirstRun(cfgPath, "")
	if err == nil {
		t.Error("expected error when config directory cannot be created")
	}
}

func TestFirstRun_SecondRunReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "katet.toml")

	if _, err := tower.FirstRun(cfgPath, ""); err != nil {
		t.Fatal(err)
	}
	created, err := tower.FirstRun(cfgPath, "")
	if err != nil {
		t.Fatalf("second FirstRun: %v", err)
	}
	if created {
		t.Error("expected created=false on second run")
	}
}

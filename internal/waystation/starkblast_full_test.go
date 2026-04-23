package waystation_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/garnizeh/maestro/internal/waystation"
)

// TestCheckAndMigrate_RunsMigrations verifies that a store at schema version 0
// is migrated to the current version, exercising the migration loop and
// the migrate(0→1) function.
func TestCheckAndMigrate_RunsMigrations(t *testing.T) {
	s := newStore(t)

	// Manually write version 0 so CheckAndMigrate has something to migrate.
	type versionedMeta struct {
		Version int `json:"version"`
	}
	if err := s.Put("meta", "schema", versionedMeta{Version: 0}); err != nil {
		t.Fatalf("seed schema v0: %v", err)
	}

	if err := s.CheckAndMigrate(); err != nil {
		t.Fatalf("CheckAndMigrate with v0 seed: %v", err)
	}

	v, err := s.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if v != waystation.CurrentSchemaVersion {
		t.Errorf("after migration, version = %d, want %d", v, waystation.CurrentSchemaVersion)
	}
}

// TestCheckAndMigrate_ReadError exercises the error path when the schema
// record exists but is unreadable.
func TestCheckAndMigrate_ReadError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission error as root")
	}
	s := newStore(t)

	// First initialise so meta/ dir is created.
	if err := s.CheckAndMigrate(); err != nil {
		t.Fatalf("initial CheckAndMigrate: %v", err)
	}

	// Make the schema file unreadable.
	schemaPath := filepath.Join(s.Root(), "meta", "schema.json")
	if err := os.Chmod(schemaPath, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(schemaPath, 0o600); err != nil {
			t.Fatal(err)
		}
	})

	if err := s.CheckAndMigrate(); err == nil {
		t.Error("expected error when schema file is unreadable")
	}
}

// TestSchemaVersion_ReadError exercises the error path in SchemaVersion when
// the file exists but cannot be read.
func TestSchemaVersion_ReadError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission error as root")
	}
	s := newStore(t)
	if err := s.CheckAndMigrate(); err != nil {
		t.Fatalf("initial CheckAndMigrate: %v", err)
	}

	schemaPath := filepath.Join(s.Root(), "meta", "schema.json")
	if err := os.Chmod(schemaPath, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(schemaPath, 0o600); err != nil {
			t.Fatal(err)
		}
	})

	_, err := s.SchemaVersion()
	if err == nil {
		t.Error("expected error when schema file is unreadable")
	}
}

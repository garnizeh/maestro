package waystation_test

import (
	"testing"

	"github.com/rodrigo-baliza/maestro/internal/waystation"
)

func TestCheckAndMigrate_FreshStore(t *testing.T) {
	s := newStore(t)
	if err := s.CheckAndMigrate(); err != nil {
		t.Fatalf("CheckAndMigrate on fresh store: %v", err)
	}
	v, err := s.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if v != waystation.CurrentSchemaVersion {
		t.Errorf("schema version = %d, want %d", v, waystation.CurrentSchemaVersion)
	}
}

func TestCheckAndMigrate_Idempotent(t *testing.T) {
	s := newStore(t)
	if err := s.CheckAndMigrate(); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckAndMigrate(); err != nil {
		t.Fatalf("second CheckAndMigrate: %v", err)
	}
}

func TestSchemaVersion_Empty(t *testing.T) {
	s := newStore(t)
	v, err := s.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if v != 0 {
		t.Errorf("expected 0 for uninitialised store, got %d", v)
	}
}

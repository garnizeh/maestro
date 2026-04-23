package waystation_test

import (
	"testing"

	"github.com/rodrigo-baliza/maestro/internal/waystation"
)

func TestCheckAndMigrate_FutureVersion(t *testing.T) {
	s := newStore(t)

	// Manually write a schema version higher than the current binary supports.
	type fakeMeta struct {
		Version int `json:"version"`
	}
	if err := s.Put("meta", "schema", fakeMeta{Version: waystation.CurrentSchemaVersion + 99}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	err := s.CheckAndMigrate()
	if err == nil {
		t.Error("expected error when store schema is newer than binary")
	}
}

// TestList_EmptyCollection verifies List returns nil (not error) for a
// collection directory that does not exist yet.
func TestList_EmptyCollection(t *testing.T) {
	s := newStore(t)
	keys, err := s.List("nonexistent-collection")
	if err != nil {
		t.Fatalf("List on nonexistent collection: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected empty list, got %v", keys)
	}
}

// TestPut_MarshalError verifies that an unmarshalable value returns an error.
func TestPut_MarshalError(t *testing.T) {
	s := newStore(t)
	// A channel cannot be marshalled to JSON.
	err := s.Put("containers", "bad", make(chan int))
	if err == nil {
		t.Error("expected marshal error for channel value")
	}
}

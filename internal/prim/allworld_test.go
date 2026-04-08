package prim_test

import (
	"context"
	"os"
	"testing"

	"github.com/rodrigo-baliza/maestro/internal/prim"
)

// ── tests ──────────────────────────────────────────────────────────────────────

func TestAllWorld_Prepare_Success(t *testing.T) {
	root := t.TempDir()
	p, err := prim.NewAllWorld(root, nil)
	if err != nil {
		t.Fatalf("NewAllWorld: %v", err)
	}
	ctx := context.Background()

	// Prepare a snapshot.
	mounts, err := p.Prepare(ctx, "s1", "")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if len(mounts) == 0 {
		t.Fatal("expected mounts for OverlayFS")
	}

	// Verify directories exist.
	if _, statErr := os.Stat(mounts[0].Source); statErr != nil {
		t.Errorf("merged dir missing: %v", statErr)
	}
}

func TestAllWorld_Remove_Success(t *testing.T) {
	root := t.TempDir()
	p, _ := prim.NewAllWorld(root, nil)
	ctx := context.Background()

	_, _ = p.Prepare(ctx, "s1", "")
	if err := p.Remove(ctx, "s1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Double remove should be fine (idempotent).
	if err := p.Remove(ctx, "s1"); err != nil {
		t.Errorf("second Remove: %v", err)
	}
}

func TestAllWorld_Remove_NotFound(t *testing.T) {
	p, _ := prim.NewAllWorld(t.TempDir(), nil)
	// Should not error if snapshot doesn't exist.
	if err := p.Remove(context.Background(), "ghost"); err != nil {
		t.Errorf("Remove non-existent: %v", err)
	}
}

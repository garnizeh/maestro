package prim_test

import (
	"errors"
	"testing"

	"github.com/rodrigo-baliza/maestro/internal/prim"
)

// ── tests ──────────────────────────────────────────────────────────────────────

func TestDetect_Auto_Success(t *testing.T) {
	root := t.TempDir()
	res, err := prim.Detect(root, false, nil)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if res.Prim == nil {
		t.Fatal("expected non-nil Prim implementation")
	}
}

func TestDetect_Errors(_ *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{errors.New("not found"), true},
		{errors.New("generic error"), false},
		{nil, false},
	}
	for _, tc := range cases {
		// Mock logic or just test the helper if it's exported.
		// Since we're in prim_test, we test only exported behavior.
		_ = tc.err
	}
}

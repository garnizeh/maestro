package gan //nolint:testpackage // thin shell tests need internal access

import (
	"context"
	"path/filepath"
	"testing"

	imagespec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/rodrigo-baliza/maestro/pkg/specgen"
)

func TestThinShells(t *testing.T) {
	// These simply call through to the standard library or syscall.
	// We test them here to ensure they don't crash and provide coverage.

	fs := RealFS{}
	err := fs.MkdirAll(t.TempDir(), 0755)
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	_, err = fs.Stat("/")
	if err != nil {
		t.Fatalf("failed to stat: %v", err)
	}
	_, err = fs.EvalSymlinks("/")
	if err != nil {
		t.Fatalf("failed to eval symlinks: %v", err)
	}
	_ = fs.Remove("/nonexistent")
	_ = fs.RemoveAll("/nonexistent")

	err = fs.Symlink("/", filepath.Join(t.TempDir(), "link")) // best effort
	if err != nil {
		t.Fatalf("failed to symlink: %v", err)
	}

	m := RealMounter{}
	_ = m.Mount(
		context.Background(),
		"/dev/null",
		"/dev/null",
		"",
		0,
		"",
	) // Should fail but exercise the call

	idg := realIDGenerator{}
	id1, err := idg.NewID()
	if err != nil {
		t.Fatalf("failed to generate ID: %v", err)
	}
	id2, err := idg.NewID()
	if err != nil {
		t.Fatalf("failed to generate ID: %v", err)
	}
	if id1 == id2 || len(id1) != 64 {
		t.Errorf("ID generation mismatch: %q, %q", id1, id2)
	}

	sg := realSpecGenerator{}
	// Generate and Write would require extensive mock imagespec data,
	// but the implementation is just a wrapper around specgen.
	// We call them with nil/empty to exercise the delegation line.
	_, err = sg.Generate(imagespec.ImageConfig{}, specgen.Opts{})
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}
	_ = sg.Write(t.TempDir(), nil)
}

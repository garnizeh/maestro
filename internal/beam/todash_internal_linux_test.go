//go:build linux

package beam //nolint:testpackage // internal tests for unexported namespace mounter

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestTodash_NewNS_DeleteNS_Root(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip(
			"TestTodash_NewNS_DeleteNS_Root: requires root (CAP_SYS_ADMIN) to create network namespaces",
		)
	}

	td := NewTodash(t.TempDir())
	id := "ns-test-roundtrip"

	nsPath, launcherPath, err := td.NewNS("test", nil)
	if err != nil {
		t.Fatalf("NewNS: %v", err)
	}
	if nsPath == "" {
		t.Fatal("expected non-empty nsPath")
	}

	t.Logf("nsPath: %s", nsPath)
	t.Logf("launcherPath: %s", launcherPath)

	if errRm := td.DeleteNS(id); errRm != nil {
		t.Fatalf("DeleteNS: %v", errRm)
	}
}

func TestTodash_MockedFailures_Linux(t *testing.T) {
	t.Parallel()
	m := &mockMounter{
		newNSFn: func(_ string, _ *MountRequest) (string, string, error) {
			return "", "", errors.New("failed to create ns")
		},
		delErr: errors.New("failed to delete ns"),
	}
	td := NewTodash(t.TempDir()).WithMounter(m).WithRootless(true)
	id := "test-id"

	_, _, err := td.NewNS(id, nil)
	if err == nil {
		t.Fatal("expected error from NewNS")
	}

	err = td.DeleteNS(id)
	if err == nil {
		t.Fatal("expected error from DeleteNS")
	}
}

func TestRealMounter_NewNS_CreateFail(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("RealMounter only exists on Linux")
	}
	m := newDefaultMounter().(*RealMounter)
	dir := t.TempDir()
	path := filepath.Join(dir, "blocked")
	// Use a read-only dir.
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if errChmod := os.Chmod(dir, 0755); errChmod != nil {
			t.Errorf("failed to restore directory permissions: %v", errChmod)
		}
	}()

	if os.Geteuid() == 0 {
		t.Skip("root ignores permission bits")
	}

	_, _, err := m.NewNS(path, nil)
	if err == nil {
		t.Fatal("expected error from os.Create in read-only dir")
	}
}

func TestRealMounter_DeleteNS_RemoveFail(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("RealMounter only exists on Linux")
	}
	m := newDefaultMounter().(*RealMounter)
	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	// Make dir read-only to prevent removal
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if errChmod := os.Chmod(dir, 0755); errChmod != nil {
			t.Errorf("failed to restore directory permissions: %v", errChmod)
		}
	}()

	if os.Geteuid() == 0 {
		t.Skip("root ignores permission bits")
	}

	if err := m.DeleteNS(path); err == nil {
		t.Fatal("expected error from os.Remove in read-only dir")
	}
}

func TestRealMounter_NewNS_RootlessSequence(t *testing.T) {
	// Skip this test in unit test mode as it tries to launch a real holder process
	// and wait for a socket that will never appear under mocks.
	t.Skip(
		"Skipping rootless sequence unit test - requires execution of maestro binary and socket handshake",
	)
}

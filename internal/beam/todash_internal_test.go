package beam

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewTodash(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	td := NewTodash(tmp)
	if td.basePath != tmp {
		t.Errorf("expected basePath %q, got %q", tmp, td.basePath)
	}
}

func TestTodash_NSPath(t *testing.T) {
	t.Parallel()
	td := &Todash{basePath: "/tmp/netns"}
	got := td.NSPath("container-abc")
	want := filepath.Join("/tmp/netns", "container-abc")
	if got != want {
		t.Errorf("NSPath: got %q, want %q", got, want)
	}
}

func TestTodash_NewNS_MkdirFailure(t *testing.T) {
	t.Parallel()

	// Point basePath at a regular file, so MkdirAll fails
	tmpFile, err := os.CreateTemp(t.TempDir(), "notadir")
	if err != nil {
		t.Fatal(err)
	}
	if errClose := tmpFile.Close(); errClose != nil {
		t.Fatalf("failed to close temp file: %v", errClose)
	}

	// Use RealFS which will fail MkdirAll on a file
	td := NewTodash(tmpFile.Name()).WithMounter(&mockMounter{})
	_, _, err = td.NewNS("test", nil)
	if err == nil {
		t.Fatal("expected error when basePath is a file, got nil")
	}
}

func TestTodash_MockedSuccess(t *testing.T) {
	t.Parallel()
	m := &mockMounter{}
	td := NewTodash(t.TempDir()).WithMounter(m)
	id := "test-id"
	expected := td.NSPath(id)

	nsPath, launcherPath, err := td.NewNS(id, nil)
	if err != nil {
		t.Fatalf("NewNS failed: %v", err)
	}
	if nsPath != expected {
		t.Errorf("expected %s, got %s", expected, nsPath)
	}
	if launcherPath != "" {
		t.Errorf("expected empty launcherPath, got %s", launcherPath)
	}

	if delErr := td.DeleteNS(id); delErr != nil {
		t.Fatalf("DeleteNS failed: %v", delErr)
	}

	if m.newCalls != 1 || m.delCalls != 1 {
		t.Errorf("expected 1 call each, got %d, %d", m.newCalls, m.delCalls)
	}
}

type mockMounter struct {
	newCalls   int
	delCalls   int
	newNSFn    func(path string, mount *MountRequest) (string, string, error)
	deleteNSFn func(path string) error
	newErr     error
	delErr     error
}

func (m *mockMounter) NewNS(path string, mount *MountRequest) (string, string, error) {
	m.newCalls++
	if m.newNSFn != nil {
		return m.newNSFn(path, mount)
	}
	return path, "", m.newErr
}

func (m *mockMounter) DeleteNS(path string) error {
	m.delCalls++
	if m.deleteNSFn != nil {
		return m.deleteNSFn(path)
	}
	return m.delErr
}

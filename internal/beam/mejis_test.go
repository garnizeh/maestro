package beam //nolint:testpackage // needs internal access to mock exec

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kr/pretty"
)

func TestMejis_FindBinary(t *testing.T) {
	t.Run("PastaFirst", func(t *testing.T) {
		m := NewMejis(t.TempDir())
		m.lookPath = func(binary string) (string, error) {
			if binary == "pasta" {
				return "/usr/bin/pasta", nil
			}
			return "", errors.New("not found")
		}

		path, name, err := m.FindBinary()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != "pasta" {
			t.Errorf("expected pasta, got %s", name)
		}
		if path != "/usr/bin/pasta" {
			t.Errorf("expected path /usr/bin/pasta, got %s", path)
		}
	})

	t.Run("SlirpFallback", func(t *testing.T) {
		m := NewMejis(t.TempDir())
		m.lookPath = func(binary string) (string, error) {
			if binary == "slirp4netns" {
				return "/usr/bin/slirp4netns", nil
			}
			return "", errors.New("not found")
		}

		path, name, err := m.FindBinary()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != "slirp4netns" {
			t.Errorf("expected slirp4netns, got %s", name)
		}
		if path != "/usr/bin/slirp4netns" {
			t.Errorf("expected path /usr/bin/slirp4netns, got %s", path)
		}
	})

	t.Run("NoneFound", func(t *testing.T) {
		m := NewMejis(t.TempDir())
		m.lookPath = func(_ string) (string, error) {
			return "", errors.New("not found")
		}

		_, _, err := m.FindBinary()
		if err == nil {
			t.Fatal("expected error when no binaries found, got nil")
		}
	})
}

func TestMejis_Lifecycle(t *testing.T) {
	stateDir := t.TempDir()
	m := NewMejis(stateDir)
	m.killProcessFn = func(_ int) error { return nil }
	m.lookPath = func(_ string) (string, error) { return "/usr/bin/pasta", nil }
	m.runCmd = func(cmd *exec.Cmd) error {
		// Mock a successful start by assigning a dummy process
		proc, err := os.FindProcess(os.Getpid())
		if err != nil {
			t.Fatalf("fail to find process: %v", err)
		}
		cmd.Process = proc
		return nil
	}

	t.Run("Attach", func(t *testing.T) {
		mappings := []PortMapping{
			{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"},
			{ContainerPort: 53, HostPort: 5353, Protocol: "udp"},
		}
		var capturedArgs []string
		m.runCmd = func(cmd *exec.Cmd) error {
			capturedArgs = cmd.Args
			proc, _ := os.FindProcess(os.Getpid())
			cmd.Process = proc
			return nil
		}

		err := m.Attach(context.Background(), "ctr-1", "/run/netns/ctr-1", "", mappings)
		if err != nil {
			t.Fatalf("Attach() unexpected error: %v", err)
		}

		expectedArgs := []string{
			"/usr/bin/pasta",
			"--netns",
			"/run/netns/ctr-1",
			"-f",
			"--config-net",
			"--host-lo-to-ns-lo",
			"-T",
			"none",
			"-U",
			"none",
			"-t",
			"8080:80",
			"-u",
			"5353:53",
		}
		if diff := pretty.Diff(expectedArgs, capturedArgs); len(diff) > 0 {
			t.Log("Mejis.Attach() args mismatch")
			t.Errorf("\n%s", diff)
		}

		// Verify PID file
		pidPath := filepath.Join(stateDir, "ctr-1.pid")
		data, err := os.ReadFile(pidPath)
		if err != nil {
			t.Fatalf("PID file not found: %v", err)
		}
		if string(data) == "" {
			t.Error("expected non-empty PID in file")
		}
	})

	t.Run("Detach", func(t *testing.T) {
		m.killProcessFn = func(pid int) error {
			if pid != os.Getpid() {
				t.Errorf("expected pid %d, got %d", os.Getpid(), pid)
			}
			return nil
		}
		err := m.Detach(context.Background(), "ctr-1")
		if err != nil {
			t.Fatalf("Detach() unexpected error: %v", err)
		}

		// Verify PID file is gone
		pidPath := filepath.Join(stateDir, "ctr-1.pid")
		if _, errStat := os.Stat(pidPath); !os.IsNotExist(errStat) {
			t.Error("PID file should have been removed")
		}
	})
}

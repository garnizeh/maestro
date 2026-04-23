//go:build linux

package beam

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// mockHolderCommander simulates the Maestro netns holder process.

func TestRealMounter_CleanupHolderByPIDFile(t *testing.T) {
	t.Parallel()
	m := &RealMounter{fs: RealFS{}}

	// Case 1: Invalid PID
	m.cleanupHolderByPIDFile([]byte("abc"))
	m.cleanupHolderByPIDFile([]byte("0"))
	m.cleanupHolderByPIDFile([]byte("1"))

	// Case 2: Non-existent PID
	m.cleanupHolderByPIDFile([]byte("999999"))
}

func TestRealMounter_PrepareHolderSocket(t *testing.T) {
	t.Parallel()
	m := &RealMounter{}
	tmp := t.TempDir()
	nsPath := filepath.Join(tmp, "my-very-long-container-id-that-should-be-truncated")

	sockPath := m.prepareHolderSocket(nsPath)
	base := filepath.Base(sockPath)

	if len(base) != holderIDLen+len(".sock") {
		t.Errorf("expected socket base length %d, got %d (%s)", holderIDLen+5, len(base), base)
	}

	// Ensure it removes existing
	if err := os.WriteFile(sockPath, []byte("stale"), 0644); err != nil {
		t.Fatal(err)
	}
	_ = m.prepareHolderSocket(nsPath)
	if _, err := os.Stat(sockPath); err == nil {
		t.Error("expected prepareHolderSocket to remove stale socket")
	}
}

func TestRealMounter_KillHolder(t *testing.T) {
	t.Parallel()
	m := &RealMounter{}

	// Nil cmd/process should not panic
	m.killHolder(nil, "test")
	m.killHolder(&exec.Cmd{}, "test")

	// Valid process
	cmd := exec.Command("sleep", "10")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	m.killHolder(cmd, "test")
	_ = cmd.Wait() // cleanup
}

func TestRealMounter_WaitForNSReady_Timeout(t *testing.T) {
	t.Parallel()
	m := &RealMounter{fs: RealFS{}}

	start := time.Now()
	err := m.waitForNSReady("/non/existent/path/for/sure/12345")
	elapsed := time.Since(start)

	if err == nil {
		t.Error("expected timeout error")
	}
	if elapsed < 100*time.Millisecond {
		t.Errorf("expected wait, but returned too fast: %v", elapsed)
	}
}

func TestRealMounter_ResolveIDMappings(t *testing.T) {
	m := &RealMounter{}
	u, g, err := m.resolveIDMappings()
	// This depends on the host system, but should at least not panic.
	if err != nil {
		t.Logf("resolveIDMappings failed as expected in some environments: %v", err)
	} else if len(u) == 0 || len(g) == 0 {
		t.Error("expected non-empty mappings on success")
	}
}

func TestHolderInvoke_DialFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, err := HolderInvoke(ctx, "/tmp/no-such-ns", ExecRequest{})
	if err == nil {
		t.Error("expected dial error")
	}
}

func TestRealMounter_DeleteNSRootless_Full(t *testing.T) {
	tmp := t.TempDir()
	nsPath := filepath.Join(tmp, "test-ns")
	pidFile := nsPath + ".pid"
	base := filepath.Base(nsPath)
	if len(base) > holderIDLen {
		base = base[:holderIDLen]
	}
	sockPath := filepath.Join(tmp, base+".sock")

	m := &RealMounter{fs: RealFS{}, rootless: true}

	// Create dummy files
	if err := os.WriteFile(nsPath, []byte("ns"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pidFile, []byte("999998"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sockPath, []byte("sock"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := m.DeleteNS(nsPath); err != nil {
		t.Fatalf("DeleteNS failure: %v", err)
	}

	// Verify cleanup
	for _, p := range []string{nsPath, pidFile, sockPath} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("file %s was not removed", p)
		}
	}
}

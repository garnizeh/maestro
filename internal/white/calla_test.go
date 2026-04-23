package white //nolint:testpackage // internal tests need access to unexported functions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kr/pretty"
)

func TestGetSubIDRange(t *testing.T) {
	// GIVEN
	tmpDir := t.TempDir()
	subuidPath := filepath.Join(tmpDir, "subuid")
	content := "alice:100000:65536\nbob:200000:1000\n"
	if err := os.WriteFile(subuidPath, []byte(content), 0644); err != nil {
		t.Fatalf("setup: failed to write subuid file: %v", err)
	}

	t.Run("ValidUser", func(t *testing.T) {
		start, count, err := GetSubIDRange("alice", subuidPath)
		if err != nil {
			t.Fatalf("GetSubIDRange() unexpected error: %v", err)
		}
		if start != 100000 {
			t.Errorf("start: got %v, want %v", start, 100000)
		}
		if count != 65536 {
			t.Errorf("count: got %v, want %v", count, 65536)
		}
	})

	t.Run("MissingUser", func(t *testing.T) {
		_, _, err := GetSubIDRange("charlie", subuidPath)
		if err == nil {
			t.Fatal("expected error for missing user, got nil")
		}
		want := "no subordinate ID allocation found for user charlie"
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error message: got %q, want it to contain %q", err.Error(), want)
		}
	})

	t.Run("MissingFile", func(t *testing.T) {
		_, _, err := GetSubIDRange("alice", "/non/existent/file")
		if err == nil {
			t.Fatal("expected error for missing file, got nil")
		}
	})

	t.Run("MalformedFile", func(t *testing.T) {
		malformedPath := filepath.Join(tmpDir, "malformed")
		content := []byte("alice:notanumber:65536")
		if err := os.WriteFile(malformedPath, content, 0644); err != nil {
			t.Fatalf("setup: failed to write malformed file: %v", err)
		}

		_, _, err := GetSubIDRange("alice", malformedPath)
		if err == nil {
			t.Fatal("expected error for malformed file, got nil")
		}
	})
}

func TestValidateSubIDRange(t *testing.T) {
	t.Run("ValidRange", func(t *testing.T) {
		if err := ValidateSubIDRange(65536); err != nil {
			t.Errorf("ValidateSubIDRange(65536) unexpected error: %v", err)
		}
	})

	t.Run("InsufficientRange", func(t *testing.T) {
		err := ValidateSubIDRange(1000)
		if err == nil {
			t.Fatal("expected error for insufficient range, got nil")
		}
		want := "insufficient subordinate ID range (found 1000, required 65536)"
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error message: got %q, want it to contain %q", err.Error(), want)
		}
	})
}

func TestBuildMappings(t *testing.T) {
	// GIVEN
	tmpDir := t.TempDir()
	subuidPath := filepath.Join(tmpDir, "subuid")
	subgidPath := filepath.Join(tmpDir, "subgid")

	if err := os.WriteFile(subuidPath, []byte("alice:100000:65536\n"), 0644); err != nil {
		t.Fatalf("setup: failed to write subuid: %v", err)
	}
	if err := os.WriteFile(subgidPath, []byte("alice:200000:65536\n"), 0644); err != nil {
		t.Fatalf("setup: failed to write subgid: %v", err)
	}

	t.Run("SubIDRangeSuccess", func(t *testing.T) {
		uidMaps, gidMaps, err := buildMappings("alice", 1000, 1000, subuidPath, subgidPath)
		if err != nil {
			t.Fatalf("buildMappings() unexpected error: %v", err)
		}

		expectedUID := []IDMapping{
			{ContainerID: 0, HostID: 1000, Size: 1},
			{ContainerID: 1, HostID: 100000, Size: 65536},
		}
		expectedGID := []IDMapping{
			{ContainerID: 0, HostID: 1000, Size: 1},
			{ContainerID: 1, HostID: 200000, Size: 65536},
		}

		checkMappings(t, "uidMaps", uidMaps, expectedUID)
		checkMappings(t, "gidMaps", gidMaps, expectedGID)
	})

	t.Run("SubUIDFailure", func(t *testing.T) {
		_, _, err := buildMappings("missing", 1000, 1000, subuidPath, subgidPath)
		if err == nil {
			t.Fatal("expected error for missing subuid user, got nil")
		}
	})

	t.Run("SubGIDFailure", func(t *testing.T) {
		// valid subuid, missing subgid
		emptyPath := filepath.Join(tmpDir, "empty")
		if err := os.WriteFile(emptyPath, []byte(""), 0644); err != nil {
			t.Fatalf("setup: failed to write empty file: %v", err)
		}

		_, _, err := buildMappings("alice", 1000, 1000, subuidPath, emptyPath)
		if err == nil {
			t.Fatal("expected error for missing subgid, got nil")
		}
	})
}

func checkMappings(t *testing.T, name string, got, want []IDMapping) {
	t.Helper()
	if diff := pretty.Diff(want, got); len(diff) > 0 {
		t.Logf("%s mismatch", name)
		t.Logf("want: %v", want)
		t.Logf("got: %v", got)
		t.Errorf("\n%s", diff)
	}
}

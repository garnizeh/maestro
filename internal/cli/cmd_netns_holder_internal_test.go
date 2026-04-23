package cli

import (
	"encoding/json"
	"net"
	"testing"

	"github.com/rodrigo-baliza/maestro/internal/beam"
)

func TestHandleInitialMount_Empty(t *testing.T) {
	s, c := net.Pipe()
	defer s.Close()
	defer c.Close()

	go func() {
		req := beam.MountRequest{}
		_ = json.NewEncoder(s).Encode(req)
	}()

	cmd := handleInitialMount(c)
	if cmd != nil {
		t.Error("expected nil cmd for empty mount request")
	}
}

func TestHandleExecConnection_Empty(_ *testing.T) {
	s, c := net.Pipe()
	defer s.Close()
	defer c.Close()

	go func() {
		req := beam.ExecRequest{} // empty args
		_ = json.NewEncoder(s).Encode(req)

		var res beam.ExecResponse
		_ = json.NewDecoder(s).Decode(&res)
	}()

	handleExecConnection(c)
}

func TestSanitizeFuseOverlayFSOptions(t *testing.T) {
	tests := []struct {
		input []string
		want  []string
	}{
		{[]string{"rw", "lazytime"}, []string{"rw"}},
		{[]string{"relatime", "nodev"}, []string{"nodev"}},
		{[]string{"rw,lazytime,noatime"}, []string{"rw", "noatime"}},
	}

	for _, tt := range tests {
		got := sanitizeFuseOverlayFSOptions(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("input %v: got %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("input %v: got %v, want %v", tt.input, got, tt.want)
			}
		}
	}
}

func TestIsMountedAt_Negative(t *testing.T) {
	// Simple sanity check for a non-mounted path
	mounted, err := isMountedAt("/non/existent/path/that/cannot/be/mounted")
	if err != nil {
		t.Fatalf("isMountedAt: %v", err)
	}
	if mounted {
		t.Error("expected false for nonexistent path")
	}
}

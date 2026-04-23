package cli_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rodrigo-baliza/maestro/internal/beam"
	"github.com/rodrigo-baliza/maestro/internal/cli"
)

func TestNetNSHolderCmd_SocketRequired(t *testing.T) {
	h := cli.NewHandler()
	root := cli.NewRootCommand(h)
	root.SilenceErrors = true
	root.SetArgs([]string{"_netns_holder"}) // missing --socket

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "socket path is required") {
		t.Fatalf("expected 'socket path is required' error, got: %v", err)
	}
}

func TestNetNSHolderCmd_ListenFail(t *testing.T) {
	// Try to listen on a path that is a directory
	tmpDir := t.TempDir()

	h := cli.NewHandler()
	root := cli.NewRootCommand(h)
	root.SilenceErrors = true
	root.SetArgs([]string{"_netns_holder", "--socket", tmpDir})

	err := root.Execute()
	if err == nil || (!strings.Contains(err.Error(), "failed to listen") &&
		!strings.Contains(err.Error(), "is a directory")) {
		t.Fatalf("expected listen error, got: %v", err)
	}
}

func TestNetNSHolderCmd_Lifecycle(t *testing.T) {
	// This tests the acceptor loop by connecting and closing.
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "holder.sock")

	h := cli.NewHandler()
	root := cli.NewRootCommand(h)

	// Start the holder in a goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		root.SetArgs([]string{"_netns_holder", "--socket", sockPath})
		errCh <- root.ExecuteContext(ctx)
	}()

	// Wait for socket to appear
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sockPath); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if _, err := os.Stat(sockPath); err != nil {
		t.Fatal("socket was not created in time")
	}

	// Connect and send a dummy mount request to satisfy handleInitialMount
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	_ = json.NewEncoder(conn).Encode(beam.MountRequest{})
	conn.Close()

	// Shutdown the holder
	cancel()

	// Wait for cleanup
	select {
	case runErr := <-errCh:
		if runErr != nil && !errors.Is(runErr, context.Canceled) &&
			!strings.Contains(runErr.Error(), "use of closed network connection") {
			t.Errorf("holder exited with unexpected error: %v", runErr)
		}
	case <-time.After(1 * time.Second):
		t.Error("holder did not exit in time")
	}
}

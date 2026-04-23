package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rodrigo-baliza/maestro/internal/cli"
)

func TestConfigCmd_Show(t *testing.T) {
	h := cli.NewHandler()
	root := cli.NewRootCommand(h)
	buf := new(bytes.Buffer)
	root.SetOut(buf)

	// Test default TOML output
	root.SetArgs([]string{"config", "show"})
	if err := root.Execute(); err != nil {
		t.Fatalf("config show: %v", err)
	}
	if !strings.Contains(buf.String(), "[runtime]") {
		t.Errorf("expected [runtime] in config show output, got: %s", buf.String())
	}
}

func TestConfigCmd_Edit_NoEditor(t *testing.T) {
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")

	h := cli.NewHandler()
	root := cli.NewRootCommand(h)
	root.SilenceErrors = true
	root.SetArgs([]string{"config", "edit"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "no editor configured") {
		t.Fatalf("expected 'no editor configured' error, got: %v", err)
	}
}

func TestConfigCmd_Edit_WithEditor(t *testing.T) {
	// Use 'true' as a mock editor that does nothing but exits success.
	// This works on most Unix-like systems.
	t.Setenv("EDITOR", "true")

	h := cli.NewHandler()
	root := cli.NewRootCommand(h)
	root.SetArgs([]string{"config", "edit"})

	if err := root.Execute(); err != nil {
		t.Fatalf("config edit with mock editor failed: %v", err)
	}
}

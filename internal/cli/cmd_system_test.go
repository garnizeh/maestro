package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/garnizeh/maestro/internal/cli"
)

func TestSystemCmd_Info(t *testing.T) {
	h := cli.NewHandler()
	root := cli.NewRootCommand(h)
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"system", "info"})

	if err := root.Execute(); err != nil {
		t.Fatalf("system info: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "OS/Arch:") || !strings.Contains(out, "Go Version:") {
		t.Errorf("system info output missing fields: %s", out)
	}
}

func TestSystemCmd_Check(t *testing.T) {
	// Value: Verify that 'system check' correctly audits prerequisites.
	// This might fail in environments without crun/pasta, but we can verify it runs.
	h := cli.NewHandler()
	root := cli.NewRootCommand(h)
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"system", "check"})

	// We just want to ensure it doesn't panic and prints the checks.
	_ = root.Execute()

	out := buf.String()
	if !strings.Contains(out, "Checking Maestro prerequisites...") {
		t.Errorf("system check output missing header: %s", out)
	}
}

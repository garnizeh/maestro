package cli_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/rodrigo-baliza/maestro/internal/cli"
)

// execRoot runs the root cobra command with the given args and captures output.
func execRoot(args ...string) (string, error) {
	root := cli.NewRootCommand()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

// ── Help & Root ──────────────────────────────────────────────────────────────

func TestRootCmd_Help(t *testing.T) {
	out, err := execRoot("--help")
	if err != nil {
		t.Fatalf("--help: %v", err)
	}
	for _, want := range []string{"container", "image", "network", "volume", "config", "run", "ps", "pull"} {
		if !strings.Contains(out, want) {
			t.Errorf("help output missing %q", want)
		}
	}
}

func TestUnknownCmd(t *testing.T) {
	_, err := execRoot("foobar-unknown")
	if err == nil {
		t.Error("expected error for unknown command")
	}
}

// ── Version ──────────────────────────────────────────────────────────────────

func TestVersionCmd_Table(t *testing.T) {
	out, err := execRoot("version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if !strings.Contains(out, "Version:") {
		t.Errorf("version output missing 'Version:', got: %s", out)
	}
}

func TestVersionCmd_JSON(t *testing.T) {
	out, err := execRoot("--format", "json", "version")
	if err != nil {
		t.Fatalf("version --format json: %v", err)
	}
	if !strings.Contains(out, `"version"`) {
		t.Errorf("version JSON missing 'version' field, got: %s", out)
	}
}

func TestVersionCmd_YAML(t *testing.T) {
	out, err := execRoot("--format", "yaml", "version")
	if err != nil {
		t.Fatalf("version --format yaml: %v", err)
	}
	if !strings.Contains(out, "version:") {
		t.Errorf("version YAML missing 'version:', got: %s", out)
	}
}

func TestVersionCmd_Template(t *testing.T) {
	out, err := execRoot("--format", "{{.OS}}/{{.Arch}}", "version")
	if err != nil {
		t.Fatalf("version template: %v", err)
	}
	if !strings.Contains(out, "/") {
		t.Errorf("version template output unexpected: %s", out)
	}
}

func TestVersionCmd_TemplateExecError(t *testing.T) {
	// {{.NoSuchField}} parses OK but fails at execution; exercises error path in newVersionCmd.
	_, err := execRoot("--format", "{{.NoSuchField}}", "version")
	if err == nil {
		t.Error("expected error when version template accesses non-existent field")
	}
}

// ── Subcommand groups ────────────────────────────────────────────────────────

func TestContainerCmd_Help(t *testing.T) {
	out, err := execRoot("container", "--help")
	if err != nil {
		t.Fatalf("container --help: %v", err)
	}
	if !strings.Contains(out, "create") || !strings.Contains(out, "start") {
		t.Errorf("container help missing subcommands, got: %s", out)
	}
}

func TestImageCmd_Help(t *testing.T) {
	out, err := execRoot("image", "--help")
	if err != nil {
		t.Fatalf("image --help: %v", err)
	}
	if !strings.Contains(out, "pull") {
		t.Errorf("image help missing 'pull', got: %s", out)
	}
}

func TestNetworkCmd_Help(t *testing.T) {
	out, err := execRoot("network", "--help")
	if err != nil {
		t.Fatalf("network --help: %v", err)
	}
	if !strings.Contains(out, "create") {
		t.Errorf("network help missing 'create', got: %s", out)
	}
}

// ── Stub commands ────────────────────────────────────────────────────────────

func TestStubCmdsReturnNotImplemented(t *testing.T) {
	cmds := [][]string{
		{"run"},
		{"exec"},
		{"ps"},
		{"push"},
		{"container", "create"},
		{"network", "create"},
		{"volume", "create"},
		{"system", "info"},
		{"artifact", "push"},
		{"service", "generate"},
	}
	for _, args := range cmds {
		_, err := execRoot(args...)
		if err == nil {
			t.Errorf("expected error for stub command %v", args)
		}
	}
}

// ── Formatter ────────────────────────────────────────────────────────────────

func TestFormatter_Table(t *testing.T) {
	f := cli.NewFormatter("table", false)
	out, err := f.Format(map[string]string{"key": "value"})
	if err != nil {
		t.Fatal(err)
	}
	// table format falls back to JSON representation
	if out == "" {
		t.Error("table format returned empty string")
	}
}

func TestFormatter_QuietNoFn(t *testing.T) {
	// quiet=true but no quietFn set — should still format normally
	f := cli.NewFormatter("json", true)
	out, err := f.Format(map[string]string{"key": "value"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "key") {
		t.Errorf("quiet without quietFn unexpected output: %s", out)
	}
}

// ── Config commands ──────────────────────────────────────────────────────────

func TestConfigShow(t *testing.T) {
	dir := t.TempDir()
	out, err := execRoot("--config", dir+"/none.toml", "config", "show")
	if err != nil {
		t.Fatalf("config show: %v", err)
	}
	if !strings.Contains(out, "[runtime]") {
		t.Errorf("config show missing [runtime] section, got: %s", out)
	}
}

func TestConfigShow_JSON(t *testing.T) {
	dir := t.TempDir()
	out, err := execRoot("--config", dir+"/none.toml", "--format", "json", "config", "show")
	if err != nil {
		t.Fatalf("config show --format json: %v", err)
	}
	if !strings.Contains(out, `"runtime"`) {
		t.Errorf("config show JSON missing runtime field, got: %s", out)
	}
}

func TestConfigEdit_NoEditor(t *testing.T) {
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")
	_, err := execRoot("config", "edit")
	if err == nil {
		t.Error("expected error when EDITOR is unset")
	}
}

func TestConfigEdit_VisualFallback(t *testing.T) {
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "true") // `true` exits 0 on Unix
	dir := t.TempDir()
	_, err := execRoot("--config", dir+"/katet.toml", "config", "edit")
	if err != nil {
		t.Fatalf("config edit with VISUAL fallback: %v", err)
	}
}

func TestConfigEdit_LaunchesEditor(t *testing.T) {
	t.Setenv("EDITOR", "true") // `true` is always available on Unix, exits 0
	dir := t.TempDir()
	_, err := execRoot("--config", dir+"/katet.toml", "config", "edit")
	if err != nil {
		t.Fatalf("config edit with 'true' editor: %v", err)
	}
}

// ── Completions ──────────────────────────────────────────────────────────────

func TestGenerateCompletions_Bash(t *testing.T) {
	root := cli.NewRootCommand()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"generate", "completions", "bash"})
	if err := root.Execute(); err != nil {
		t.Fatalf("generate completions bash: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("bash completion output is empty")
	}
}

func TestGenerateCompletions_Zsh(t *testing.T) {
	root := cli.NewRootCommand()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"generate", "completions", "zsh"})
	if err := root.Execute(); err != nil {
		t.Fatalf("generate completions zsh: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("zsh completion output is empty")
	}
}

func TestGenerateCompletions_Fish(t *testing.T) {
	root := cli.NewRootCommand()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"generate", "completions", "fish"})
	if err := root.Execute(); err != nil {
		t.Fatalf("generate completions fish: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("fish completion output is empty")
	}
}

func TestGenerateCompletions_UnknownShell(t *testing.T) {
	// Passes an unrecognised shell name — hits the default `return nil` branch.
	_, err := execRoot("generate", "completions", "unknownshell")
	if err != nil {
		t.Fatalf("generate completions unknownshell: %v", err)
	}
}

func TestGenerateCompletions_PowerShell(t *testing.T) {
	root := cli.NewRootCommand()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"generate", "completions", "powershell"})
	if err := root.Execute(); err != nil {
		t.Fatalf("generate completions powershell: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("powershell completion output is empty")
	}
}

// ── Execute os.Exit path ─────────────────────────────────────────────────────

// TestExecute_ErrorExitsNonZero spawns a subprocess that calls Execute with a
// bad command, verifying that the [os.Exit](1) code path is reachable and
// produces exit code 1.
func TestExecute_ErrorExitsNonZero(t *testing.T) {
	if os.Getenv("MAESTRO_EXIT_TEST") == "1" {
		// Running inside the subprocess — call Execute with a bad command.
		oldArgs := os.Args
		os.Args = []string{"maestro", "totally-unknown-command-xyz"}
		defer func() { os.Args = oldArgs }()
		cli.Execute()
		return // should not be reached; Execute calls os.Exit(1)
	}

	proc := exec.CommandContext(
		context.Background(),
		os.Args[0],
		"-test.run=^TestExecute_ErrorExitsNonZero$",
		"-test.v",
	)
	proc.Env = append(os.Environ(), "MAESTRO_EXIT_TEST=1")
	err := proc.Run()
	if err == nil {
		t.Fatal("expected non-zero exit from Execute with bad command")
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if exitErr.ExitCode() != 1 {
			t.Errorf("exit code = %d, want 1", exitErr.ExitCode())
		}
	}
}

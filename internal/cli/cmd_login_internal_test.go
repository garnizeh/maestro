package cli

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/rodrigo-baliza/maestro/internal/shardik"
)

// execRootForLogin runs the root command for login/logout tests.
func execRootForLogin(h *Handler, stdin io.Reader, args ...string) (string, error) {
	root := NewRootCommand(h)
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	if stdin != nil {
		root.SetIn(stdin)
	}
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

// --- login tests ---

func TestLoginCmd_HelpFlag(t *testing.T) {
	h := NewHandler()
	out, err := execRootForLogin(h, nil, "login", "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"--username", "--password-stdin", "SERVER"} {
		if !strings.Contains(out, want) {
			t.Errorf("help output missing %q, got: %s", want, out)
		}
	}
}

func TestLoginCmd_TooManyArgs(t *testing.T) {
	h := NewHandler()
	_, err := execRootForLogin(h, nil, "login", "reg1", "reg2")
	if err == nil {
		t.Fatal("expected error for too many args")
	}
}

func TestLoginCmd_PasswordStdin_RequiresUsername(t *testing.T) {
	h := NewHandler()
	_, err := execRootForLogin(h, nil, "login", "--password-stdin", "ghcr.io")
	if err == nil {
		t.Fatal("expected error when --password-stdin used without --username")
	}
	if !strings.Contains(err.Error(), "--username") {
		t.Errorf("error should mention --username, got: %v", err)
	}
}

func TestLoginCmd_AllFlags_DefaultRegistry(t *testing.T) {
	h := NewHandler()
	var capturedReg, capturedUser, capturedPass string
	h.LoginSaveFn = func(reg, user, pass string, _ shardik.SigulConfig) error {
		capturedReg, capturedUser, capturedPass = reg, user, pass
		return nil
	}

	out, err := execRootForLogin(h, nil, "login", "-u", "alice", "-p", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedReg != "docker.io" {
		t.Errorf("registry = %q, want docker.io", capturedReg)
	}
	if capturedUser != "alice" {
		t.Errorf("username = %q, want alice", capturedUser)
	}
	if capturedPass != "secret" {
		t.Errorf("password = %q, want secret", capturedPass)
	}
	if !strings.Contains(out, "Login Succeeded") {
		t.Errorf("expected 'Login Succeeded' in output, got: %s", out)
	}
}

func TestLoginCmd_ExplicitRegistry(t *testing.T) {
	h := NewHandler()
	var capturedReg string
	h.LoginSaveFn = func(reg, _, _ string, _ shardik.SigulConfig) error { capturedReg = reg; return nil }

	if _, err := execRootForLogin(h, nil, "login", "-u", "u", "-p", "p", "ghcr.io"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedReg != "ghcr.io" {
		t.Errorf("registry = %q, want ghcr.io", capturedReg)
	}
}

func TestLoginCmd_PromptUsername(t *testing.T) {
	h := NewHandler()
	var capturedUser string
	h.LoginSaveFn = func(_, user, _ string, _ shardik.SigulConfig) error { capturedUser = user; return nil }
	h.LoginReadPasswordFn = func() (string, error) { return "pwd", nil }

	// stdin provides the username; password comes from the mock
	_, err := execRootForLogin(h, strings.NewReader("bob\n"), "login")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedUser != "bob" {
		t.Errorf("username = %q, want bob", capturedUser)
	}
}

func TestLoginCmd_PromptUsername_Error(t *testing.T) {
	h := NewHandler()
	readErr := errors.New("stdin closed")
	h.LoginReadLineFn = func(_ io.Reader) (string, error) { return "", readErr }

	_, err := execRootForLogin(h, nil, "login")
	if err == nil {
		t.Fatal("expected error reading username")
	}
	if !strings.Contains(err.Error(), "read username") {
		t.Errorf("error should mention 'read username', got: %v", err)
	}
}

func TestLoginCmd_PasswordStdin_Success(t *testing.T) {
	h := NewHandler()
	var capturedPass string
	h.LoginSaveFn = func(_, _, pass string, _ shardik.SigulConfig) error { capturedPass = pass; return nil }

	_, err := execRootForLogin(
		h,
		strings.NewReader("s3cret\n"),
		"login",
		"-u",
		"alice",
		"--password-stdin",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedPass != "s3cret" {
		t.Errorf("password = %q, want s3cret", capturedPass)
	}
}

func TestLoginCmd_PasswordStdin_Error(t *testing.T) {
	h := NewHandler()
	readErr := errors.New("pipe broken")
	h.LoginReadLineFn = func(_ io.Reader) (string, error) { return "", readErr }

	_, err := execRootForLogin(h, nil, "login", "-u", "alice", "--password-stdin")
	if err == nil {
		t.Fatal("expected error reading password from stdin")
	}
	if !strings.Contains(err.Error(), "read password from stdin") {
		t.Errorf("error should mention 'read password from stdin', got: %v", err)
	}
}

func TestLoginCmd_PasswordPrompt(t *testing.T) {
	h := NewHandler()
	var capturedPass string
	h.LoginSaveFn = func(_, _, pass string, _ shardik.SigulConfig) error { capturedPass = pass; return nil }
	h.LoginReadPasswordFn = func() (string, error) { return "interactive!", nil }

	_, err := execRootForLogin(h, strings.NewReader("carol\n"), "login")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedPass != "interactive!" {
		t.Errorf("password = %q, want interactive!", capturedPass)
	}
}

func TestLoginCmd_PasswordPrompt_Error(t *testing.T) {
	h := NewHandler()
	pwdErr := errors.New("terminal error")
	h.LoginReadPasswordFn = func() (string, error) { return "", pwdErr }

	_, err := execRootForLogin(h, strings.NewReader("dave\n"), "login")
	if err == nil {
		t.Fatal("expected error from password prompt")
	}
	if !strings.Contains(err.Error(), "read password") {
		t.Errorf("error should mention 'read password', got: %v", err)
	}
}

func TestLoginCmd_SaveError(t *testing.T) {
	h := NewHandler()
	saveErr := errors.New("disk full")
	h.LoginSaveFn = func(_, _, _ string, _ shardik.SigulConfig) error { return saveErr }

	_, err := execRootForLogin(h, nil, "login", "-u", "u", "-p", "p")
	if err == nil {
		t.Fatal("expected save error")
	}
	if !strings.Contains(err.Error(), "save credentials") {
		t.Errorf("error should mention 'save credentials', got: %v", err)
	}
}

func TestLoginCmd_Quiet(t *testing.T) {
	h := NewHandler()
	h.LoginSaveFn = func(_, _, _ string, _ shardik.SigulConfig) error { return nil }

	out, err := execRootForLogin(h, nil, "--quiet", "login", "-u", "u", "-p", "p")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "Login Succeeded") {
		t.Errorf("quiet mode should suppress output, got: %s", out)
	}
}

// --- logout tests ---

func TestLogoutCmd_HelpFlag(t *testing.T) {
	h := NewHandler()
	out, err := execRootForLogin(h, nil, "logout", "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "SERVER") {
		t.Errorf("help output missing 'SERVER', got: %s", out)
	}
}

func TestLogoutCmd_TooManyArgs(t *testing.T) {
	h := NewHandler()
	_, err := execRootForLogin(h, nil, "logout", "reg1", "reg2")
	if err == nil {
		t.Fatal("expected error for too many args")
	}
}

func TestLogoutCmd_DefaultRegistry(t *testing.T) {
	h := NewHandler()
	var capturedReg string
	h.LoginRemoveFn = func(reg string, _ shardik.SigulConfig) error { capturedReg = reg; return nil }

	out, err := execRootForLogin(h, nil, "logout")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedReg != "docker.io" {
		t.Errorf("registry = %q, want docker.io", capturedReg)
	}
	if !strings.Contains(out, "docker.io") {
		t.Errorf("expected registry in output, got: %s", out)
	}
}

func TestLogoutCmd_ExplicitRegistry(t *testing.T) {
	h := NewHandler()
	var capturedReg string
	h.LoginRemoveFn = func(reg string, _ shardik.SigulConfig) error { capturedReg = reg; return nil }

	if _, err := execRootForLogin(h, nil, "logout", "quay.io"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedReg != "quay.io" {
		t.Errorf("registry = %q, want quay.io", capturedReg)
	}
}

func TestLogoutCmd_RemoveError(t *testing.T) {
	h := NewHandler()
	removeErr := errors.New("permission denied")
	h.LoginRemoveFn = func(_ string, _ shardik.SigulConfig) error { return removeErr }

	_, err := execRootForLogin(h, nil, "logout")
	if err == nil {
		t.Fatal("expected remove error")
	}
	if !strings.Contains(err.Error(), "remove credentials") {
		t.Errorf("error should mention 'remove credentials', got: %v", err)
	}
}

func TestLogoutCmd_Quiet(t *testing.T) {
	h := NewHandler()
	h.LoginRemoveFn = func(_ string, _ shardik.SigulConfig) error { return nil }

	out, err := execRootForLogin(h, nil, "--quiet", "logout")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "Removing") {
		t.Errorf("quiet mode should suppress output, got: %s", out)
	}
}

// --- defaultReadLine unit tests ---

func TestDefaultReadLine_Success(t *testing.T) {
	got, err := defaultReadLine(strings.NewReader("hello\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want hello", got)
	}
}

func TestDefaultReadLine_EOF(t *testing.T) {
	_, err := defaultReadLine(strings.NewReader(""))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("got %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestDefaultReadLine_ScanError(t *testing.T) {
	readErr := errors.New("read failure")
	_, err := defaultReadLine(iotest.ErrReader(readErr))
	if !errors.Is(err, readErr) {
		t.Errorf("got %v, want %v", err, readErr)
	}
}

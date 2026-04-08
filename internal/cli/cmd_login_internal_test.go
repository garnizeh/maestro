package cli

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

// execRootForLogin runs the root command for login/logout tests.
func execRootForLogin(stdin io.Reader, args ...string) (string, error) {
	root := NewRootCommand()
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

// cleanup resets all login DI vars and globalFlags after each test.
func cleanupLoginDI(t *testing.T) {
	t.Helper()
	origSave := loginSaveFn
	origRemove := loginRemoveFn
	origReadPwd := loginReadPasswordFn
	origReadLine := loginReadLineFn
	t.Cleanup(func() {
		loginSaveFn = origSave
		loginRemoveFn = origRemove
		loginReadPasswordFn = origReadPwd
		loginReadLineFn = origReadLine
		globalFlags = GlobalFlags{}
	})
}

// --- login tests ---

func TestLoginCmd_HelpFlag(t *testing.T) {
	out, err := execRootForLogin(nil, "login", "--help")
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
	_, err := execRootForLogin(nil, "login", "reg1", "reg2")
	if err == nil {
		t.Fatal("expected error for too many args")
	}
}

func TestLoginCmd_PasswordStdin_RequiresUsername(t *testing.T) {
	cleanupLoginDI(t)
	_, err := execRootForLogin(nil, "login", "--password-stdin", "ghcr.io")
	if err == nil {
		t.Fatal("expected error when --password-stdin used without --username")
	}
	if !strings.Contains(err.Error(), "--username") {
		t.Errorf("error should mention --username, got: %v", err)
	}
}

func TestLoginCmd_AllFlags_DefaultRegistry(t *testing.T) {
	cleanupLoginDI(t)
	var capturedReg, capturedUser, capturedPass string
	loginSaveFn = func(reg, user, pass, _ string) error {
		capturedReg, capturedUser, capturedPass = reg, user, pass
		return nil
	}

	out, err := execRootForLogin(nil, "login", "-u", "alice", "-p", "secret")
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
	cleanupLoginDI(t)
	var capturedReg string
	loginSaveFn = func(reg, _, _, _ string) error { capturedReg = reg; return nil }

	if _, err := execRootForLogin(nil, "login", "-u", "u", "-p", "p", "ghcr.io"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedReg != "ghcr.io" {
		t.Errorf("registry = %q, want ghcr.io", capturedReg)
	}
}

func TestLoginCmd_PromptUsername(t *testing.T) {
	cleanupLoginDI(t)
	var capturedUser string
	loginSaveFn = func(_, user, _, _ string) error { capturedUser = user; return nil }
	loginReadPasswordFn = func() (string, error) { return "pwd", nil }

	// stdin provides the username; password comes from the mock
	_, err := execRootForLogin(strings.NewReader("bob\n"), "login")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedUser != "bob" {
		t.Errorf("username = %q, want bob", capturedUser)
	}
}

func TestLoginCmd_PromptUsername_Error(t *testing.T) {
	cleanupLoginDI(t)
	readErr := errors.New("stdin closed")
	loginReadLineFn = func(_ io.Reader) (string, error) { return "", readErr }

	_, err := execRootForLogin(nil, "login")
	if err == nil {
		t.Fatal("expected error reading username")
	}
	if !strings.Contains(err.Error(), "read username") {
		t.Errorf("error should mention 'read username', got: %v", err)
	}
}

func TestLoginCmd_PasswordStdin_Success(t *testing.T) {
	cleanupLoginDI(t)
	var capturedPass string
	loginSaveFn = func(_, _, pass, _ string) error { capturedPass = pass; return nil }

	_, err := execRootForLogin(strings.NewReader("s3cret\n"), "login", "-u", "alice", "--password-stdin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedPass != "s3cret" {
		t.Errorf("password = %q, want s3cret", capturedPass)
	}
}

func TestLoginCmd_PasswordStdin_Error(t *testing.T) {
	cleanupLoginDI(t)
	readErr := errors.New("pipe broken")
	loginReadLineFn = func(_ io.Reader) (string, error) { return "", readErr }

	_, err := execRootForLogin(nil, "login", "-u", "alice", "--password-stdin")
	if err == nil {
		t.Fatal("expected error reading password from stdin")
	}
	if !strings.Contains(err.Error(), "read password from stdin") {
		t.Errorf("error should mention 'read password from stdin', got: %v", err)
	}
}

func TestLoginCmd_PasswordPrompt(t *testing.T) {
	cleanupLoginDI(t)
	var capturedPass string
	loginSaveFn = func(_, _, pass, _ string) error { capturedPass = pass; return nil }
	loginReadPasswordFn = func() (string, error) { return "interactive!", nil }

	_, err := execRootForLogin(strings.NewReader("carol\n"), "login")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedPass != "interactive!" {
		t.Errorf("password = %q, want interactive!", capturedPass)
	}
}

func TestLoginCmd_PasswordPrompt_Error(t *testing.T) {
	cleanupLoginDI(t)
	pwdErr := errors.New("terminal error")
	loginReadPasswordFn = func() (string, error) { return "", pwdErr }

	_, err := execRootForLogin(strings.NewReader("dave\n"), "login")
	if err == nil {
		t.Fatal("expected error from password prompt")
	}
	if !strings.Contains(err.Error(), "read password") {
		t.Errorf("error should mention 'read password', got: %v", err)
	}
}

func TestLoginCmd_SaveError(t *testing.T) {
	cleanupLoginDI(t)
	saveErr := errors.New("disk full")
	loginSaveFn = func(_, _, _, _ string) error { return saveErr }

	_, err := execRootForLogin(nil, "login", "-u", "u", "-p", "p")
	if err == nil {
		t.Fatal("expected save error")
	}
	if !strings.Contains(err.Error(), "save credentials") {
		t.Errorf("error should mention 'save credentials', got: %v", err)
	}
}

func TestLoginCmd_Quiet(t *testing.T) {
	cleanupLoginDI(t)
	loginSaveFn = func(_, _, _, _ string) error { return nil }

	out, err := execRootForLogin(nil, "--quiet", "login", "-u", "u", "-p", "p")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "Login Succeeded") {
		t.Errorf("quiet mode should suppress output, got: %s", out)
	}
}

// --- logout tests ---

func TestLogoutCmd_HelpFlag(t *testing.T) {
	out, err := execRootForLogin(nil, "logout", "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "SERVER") {
		t.Errorf("help output missing 'SERVER', got: %s", out)
	}
}

func TestLogoutCmd_TooManyArgs(t *testing.T) {
	_, err := execRootForLogin(nil, "logout", "reg1", "reg2")
	if err == nil {
		t.Fatal("expected error for too many args")
	}
}

func TestLogoutCmd_DefaultRegistry(t *testing.T) {
	cleanupLoginDI(t)
	var capturedReg string
	loginRemoveFn = func(reg, _ string) error { capturedReg = reg; return nil }

	out, err := execRootForLogin(nil, "logout")
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
	cleanupLoginDI(t)
	var capturedReg string
	loginRemoveFn = func(reg, _ string) error { capturedReg = reg; return nil }

	if _, err := execRootForLogin(nil, "logout", "quay.io"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedReg != "quay.io" {
		t.Errorf("registry = %q, want quay.io", capturedReg)
	}
}

func TestLogoutCmd_RemoveError(t *testing.T) {
	cleanupLoginDI(t)
	removeErr := errors.New("permission denied")
	loginRemoveFn = func(_, _ string) error { return removeErr }

	_, err := execRootForLogin(nil, "logout")
	if err == nil {
		t.Fatal("expected remove error")
	}
	if !strings.Contains(err.Error(), "remove credentials") {
		t.Errorf("error should mention 'remove credentials', got: %v", err)
	}
}

func TestLogoutCmd_Quiet(t *testing.T) {
	cleanupLoginDI(t)
	loginRemoveFn = func(_, _ string) error { return nil }

	out, err := execRootForLogin(nil, "--quiet", "logout")
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

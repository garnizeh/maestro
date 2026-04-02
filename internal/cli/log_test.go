package cli_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/rs/zerolog"

	"github.com/rodrigo-baliza/maestro/internal/cli"
)

func TestInitLogger_InvalidLevelDefaultsToWarn(t *testing.T) {
	if err := cli.InitLogger("not-a-level", false); err != nil {
		t.Fatal(err)
	}
	if zerolog.GlobalLevel() != zerolog.WarnLevel {
		t.Errorf("expected WarnLevel fallback, got %s", zerolog.GlobalLevel())
	}
}

func TestInitLogger_DebugLevel(t *testing.T) {
	if err := cli.InitLogger("debug", false); err != nil {
		t.Fatal(err)
	}
	if zerolog.GlobalLevel() != zerolog.DebugLevel {
		t.Errorf("expected DebugLevel, got %s", zerolog.GlobalLevel())
	}
	t.Cleanup(func() { _ = cli.InitLogger("warn", false) })
}

// TestInitLoggerTo_NonTTY exercises the non-TTY (plain writer) path.
// A [bytes.Buffer] is not [os.File] so the ConsoleWriter branch is skipped.
func TestInitLoggerTo_NonTTY(t *testing.T) {
	var buf bytes.Buffer
	if err := cli.InitLoggerTo(&buf, "info", false); err != nil {
		t.Fatal(err)
	}
}

// TestInitLoggerTo_TTY exercises the ConsoleWriter branch by overriding
// IsTerminalFn so it returns true without requiring a real terminal.
func TestInitLoggerTo_TTY(t *testing.T) {
	old := cli.IsTerminalFn
	cli.IsTerminalFn = func(uintptr) bool { return true }
	t.Cleanup(func() { cli.IsTerminalFn = old })

	// Pass os.Stderr so the *os.File type assertion succeeds and IsTerminalFn
	// is called, entering the ConsoleWriter branch.
	if err := cli.InitLoggerTo(os.Stderr, "warn", false); err != nil {
		t.Fatalf("InitLoggerTo with fake TTY: %v", err)
	}
}

// TestInitLoggerTo_TTYNoColor verifies that noColor=true skips ConsoleWriter
// even when the fd is a TTY.
func TestInitLoggerTo_TTYNoColor(t *testing.T) {
	old := cli.IsTerminalFn
	cli.IsTerminalFn = func(uintptr) bool { return true }
	t.Cleanup(func() { cli.IsTerminalFn = old })

	if err := cli.InitLoggerTo(os.Stderr, "warn", true); err != nil {
		t.Fatalf("InitLoggerTo with noColor: %v", err)
	}
}

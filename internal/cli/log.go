package cli

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// IsTerminalFn determines whether a file descriptor is a TTY.
// Override in tests to simulate TTY vs pipe environments.
//
//nolint:gochecknoglobals // dependency injection point: overridden in tests to avoid real TTY checks
var IsTerminalFn func(fd uintptr) bool = isatty.IsTerminal

// InitLogger initializes the package logger to write to standard error using the
// specified log level and color preference. If the provided level is invalid it
// defaults to warning; when stderr is a TTY and color is enabled the output uses
// a timestamped console format, otherwise JSON-style logging is used.
func InitLogger(level string, noColor bool) error {
	return InitLoggerTo(os.Stderr, level, noColor)
}

// InitLoggerTo configures zerolog for dest. It is exported so tests can inject
// arbitrary writers and exercise both the TTY (ConsoleWriter) and non-TTY
// InitLoggerTo initializes the package-level zerolog logger to write to dest and sets the global log level.
// It parses level case-insensitively and falls back to zerolog.WarnLevel if parsing fails.
// If dest is an *os.File that is a terminal (as determined by IsTerminalFn) and noColor is false,
// the logger uses a console writer with RFC3339 timestamps and color support; otherwise it writes the raw
// (JSON-style) output directly to dest.
func InitLoggerTo(dest io.Writer, level string, noColor bool) error {
	lvl, err := zerolog.ParseLevel(strings.ToLower(level))
	if err != nil {
		lvl = zerolog.WarnLevel
	}
	zerolog.SetGlobalLevel(lvl)

	tty := false
	if f, ok := dest.(*os.File); ok {
		tty = IsTerminalFn(f.Fd())
	}

	var w io.Writer
	if tty && !noColor {
		w = zerolog.ConsoleWriter{
			Out:        dest,
			TimeFormat: time.RFC3339,
			NoColor:    noColor,
		}
	} else {
		w = dest
	}

	logger := zerolog.New(w).With().Timestamp().Logger()
	log.Logger = logger //nolint:reassign // zerolog idiom: global logger is designed to be reconfigured
	return nil
}

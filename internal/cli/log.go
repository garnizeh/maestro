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

// InitLogger configures zerolog for [os.Stderr].
func InitLogger(level string, noColor bool) error {
	return InitLoggerTo(os.Stderr, level, noColor)
}

// InitLoggerTo configures zerolog for dest. It is exported so tests can inject
// arbitrary writers and exercise both the TTY (ConsoleWriter) and non-TTY
// (JSON) output paths without requiring a real terminal.
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

package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// InitLogger configures zerolog for [os.Stderr].
func InitLogger(h *Handler) error {
	return InitLoggerTo(nil, os.Stderr, h.LogLevel, h.NoColor, h.IsTerminalFn)
}

// InitLoggerTo configures zerolog for dest.
func InitLoggerTo(
	_ *Handler,
	dest io.Writer,
	level string,
	noColor bool,
	isTerminal func(uintptr) bool,
) error {
	lvl, err := zerolog.ParseLevel(strings.ToLower(level))
	if err != nil {
		lvl = zerolog.WarnLevel
	}
	zerolog.SetGlobalLevel(lvl)

	tty := false
	if f, ok := dest.(*os.File); ok {
		if isTerminal != nil {
			tty = isTerminal(f.Fd())
		}
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

func printFf(w io.Writer, format string, a ...any) {
	if _, err := fmt.Fprintf(w, format, a...); err != nil {
		log.Error().Err(err).Msgf("failed to print: %s", format)
	}
}

package tower

import (
	"fmt"
	"os"

	"github.com/rs/zerolog/log"
)

// FirstRun checks for a missing config file and missing state directories,
// creates them, and prints a welcome message to stderr on first start.
// It returns true when this was a first run.
func FirstRun(configOverride string, _ string) (bool, error) {
	created, path, err := EnsureDefault(configOverride)
	if err != nil {
		return false, fmt.Errorf("ensure config: %w", err)
	}

	if created {
		log.Debug().Str("path", path).Msg("tower: first run detected, created default config")
		if _, printErr := fmt.Fprintf(os.Stderr,
			"\nWelcome to Maestro!\n\n"+
				"A default configuration file has been created at:\n  %s\n\n"+
				"Edit it with: maestro config edit\n\n",
			path,
		); printErr != nil {
			log.Debug().Err(printErr).Msg("tower: failed to write welcome message")
		}
	} else {
		log.Debug().Str("path", path).Msg("tower: using existing config")
	}

	return created, nil
}

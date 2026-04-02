package tower

import (
	"fmt"
	"os"
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
		fmt.Fprintf(os.Stderr,
			"\nWelcome to Maestro!\n\n"+
				"A default configuration file has been created at:\n  %s\n\n"+
				"Edit it with: maestro config edit\n\n",
			path,
		)
	}

	return created, nil
}

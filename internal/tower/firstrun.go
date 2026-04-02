package tower

import (
	"fmt"
	"os"
)

// FirstRun checks for a missing config file and missing state directories,
// creates them, and prints a welcome message to stderr on first start.
// FirstRun ensures the application's default configuration and required state
// directories exist, creating them if they are missing and printing a welcome
// message to standard error when a new config file is created. The
// configOverride argument, if non-empty, specifies an alternate config path;
// the second string parameter is ignored. It returns true when a default
// configuration file was created, or false otherwise, and an error if
// initialization fails.
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

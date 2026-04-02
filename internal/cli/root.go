// Package cli provides the command-line interface for Maestro.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// GlobalFlags holds all flags shared across every command.
type GlobalFlags struct {
	Config        string
	LogLevel      string
	Runtime       string
	StorageDriver string
	Root          string
	Host          string
	Format        string
	NoColor       bool
	Quiet         bool
}

//nolint:gochecknoglobals // shared flag state bound to cobra persistent flags
var globalFlags GlobalFlags

// NewRootCommand creates the Cobra root command for the maestro CLI, configures
// persistent global flags, registers a logging initializer, and attaches all
// subcommands and top-level shortcut commands.
// It returns the fully constructed *cobra.Command ready for execution.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "maestro",
		Short: "A daemonless, rootless OCI container manager",
		Long: `Maestro — daemonless OCI container management.

Rootless by default. OCI v1.1 native. No daemon required.

  maestro run -d -p 8080:80 nginx:latest
  maestro ps
  maestro logs -f web`,
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			return InitLogger(globalFlags.LogLevel, globalFlags.NoColor)
		},
	}

	// Global flags
	pf := root.PersistentFlags()
	pf.StringVar(&globalFlags.Config, "config", "", "Path to katet.toml (default: ~/.config/maestro/katet.toml)")
	pf.StringVar(&globalFlags.LogLevel, "log-level", "warn", "Log verbosity: debug, info, warn, error")
	pf.StringVar(&globalFlags.Runtime, "runtime", "auto", "OCI runtime: runc, crun, youki, runsc, kata, auto")
	pf.StringVar(&globalFlags.StorageDriver, "storage-driver", "auto", "Storage driver: overlay, btrfs, zfs, vfs, auto")
	pf.StringVar(&globalFlags.Root, "root", "", "Waystation root directory (default: ~/.local/share/maestro)")
	pf.StringVar(&globalFlags.Host, "host", "", "Positronics socket URI for API mode")
	pf.StringVar(&globalFlags.Format, "format", "table", "Output format: table, json, yaml, or Go template")
	pf.BoolVar(&globalFlags.NoColor, "no-color", false, "Disable colored output")
	pf.BoolVarP(&globalFlags.Quiet, "quiet", "q", false, "Show only resource IDs")

	// Subcommand groups
	root.AddCommand(
		newContainerCmd(),
		newImageCmd(),
		newVolumeCmd(),
		newNetworkCmd(),
		newArtifactCmd(),
		newSystemCmd(),
		newServiceCmd(),
		newGenerateCmd(),
		newConfigCmd(),
	)

	// Top-level shortcuts
	root.AddCommand(
		newRunCmd(),
		newExecCmd(),
		newPsCmd(),
		newPullCmd(),
		newPushCmd(),
		newImagesCmd(),
		newLoginCmd(),
		newLogoutCmd(),
		newVersionCmd(),
	)

	return root
}

// Execute builds the root CLI command and runs it.
//
// If command execution returns an error, the error is written to standard
// error prefixed with "Error:" and the process exits with status code 1.
func Execute() {
	root := NewRootCommand()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

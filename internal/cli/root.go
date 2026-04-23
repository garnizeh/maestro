// Package cli provides the command-line interface for Maestro.
package cli

import (
	"fmt"
	"os"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// NewRootCommand builds the cobra root command with all subcommands attached.
func NewRootCommand(h *Handler) *cobra.Command {
	root := &cobra.Command{
		Use:   "maestro",
		Short: "A daemonless, rootless OCI container manager",
		Long: `Maestro — daemonless OCI container management.

Rootless by default. OCI v1.1 native. No daemon required.

Made by garnizeH labs.

  maestro run -d -p 8080:80 nginx:latest
  maestro ps
  maestro logs -f web`,
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if err := InitLogger(h); err != nil {
				return err
			}
			log.Debug().
				Str("command", cmd.CalledAs()).
				Str("logLevel", h.LogLevel).
				Str("runtime", h.Runtime).
				Str("storageDriver", h.StorageDriver).
				Str("root", h.Root).
				Msg("maestro: startup")
			return nil
		},
	}

	// Global flags
	pf := root.PersistentFlags()
	pf.StringVar(
		&h.Config,
		"config",
		"",
		"Path to katet.toml (default: ~/.config/maestro/katet.toml)",
	)
	pf.StringVar(&h.LogLevel, "log-level", "warn", "Log verbosity: debug, info, warn, error")
	pf.StringVar(&h.Runtime, "runtime", "auto", "OCI runtime: runc, crun, youki, runsc, kata, auto")
	pf.StringVar(
		&h.StorageDriver,
		"storage-driver",
		"auto",
		"Storage driver: overlay, btrfs, zfs, vfs, auto",
	)
	pf.StringVar(&h.Root, "root", "", "Waystation root directory (default: ~/.local/share/maestro)")
	pf.StringVar(&h.Host, "host", "", "Positronics socket URI for API mode")
	pf.StringVar(&h.Format, "format", "table", "Output format: table, json, yaml, or Go template")
	pf.BoolVar(&h.NoColor, "no-color", false, "Disable colored output")
	pf.BoolVarP(&h.Quiet, "quiet", "q", false, "Show only resource IDs")

	// Subcommand groups
	root.AddCommand(
		newContainerCmd(h),
		newImageCmd(h),
		newVolumeCmd(h),
		newNetworkCmd(h),
		newArtifactCmd(h),
		newSystemCmd(h),
		newServiceCmd(h),
		newGenerateCmd(h),
		newConfigCmd(h),
	)

	// Top-level shortcuts
	root.AddCommand(
		newRunCmd(h),
		newExecCmd(h),
		newPsCmd(h),
		newLogsCmd(h),
		newStopCmd(h),
		newRmCmd(h),
		newInspectCmd(h),
		newPullCmd(h),
		newPushCmd(h),
		newImagesCmd(h),
		newLoginCmd(h),
		newLogoutCmd(h),
		newVersionCmd(h),
		newNetNSHolderCmd(h),
	)

	return root
}

// Execute is the main entry point called by main.go.
func Execute() {
	h := NewHandler()
	root := NewRootCommand(h)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

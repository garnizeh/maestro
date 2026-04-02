package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/rodrigo-baliza/maestro/internal/tower"
)

// It registers the "show" and "edit" subcommands.
func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage Maestro configuration",
	}
	cmd.AddCommand(
		newConfigShowCmd(),
		newConfigEditCmd(),
	)
	return cmd
}

// newConfigShowCmd creates the "show" subcommand that displays the effective configuration in TOML by default or in JSON when the global format is set to JSON.
// The command loads the configuration and writes it to the command's output writer, returning any error encountered while loading or formatting the configuration.
func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Display the effective configuration in TOML format",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := tower.LoadConfig(globalFlags.Config)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			w := cmd.OutOrStdout()
			switch globalFlags.Format {
			case string(FormatJSON):
				f := NewFormatter(string(FormatJSON), false)
				out, fmtErr := f.Format(cfg)
				if fmtErr != nil {
					return fmtErr //coverage:ignore Config only contains JSON-serializable fields; Format never errors here
				}
				fmt.Fprintln(w, out)
			default:
				fmt.Fprint(w, cfg.ToTOML())
			}
			return nil
		},
	}
}

// newConfigEditCmd creates a Cobra command that opens katet.toml in the user's configured editor.
// 
// The command selects the editor from the EDITOR environment variable, falling back to VISUAL,
// and returns an error if neither is set. It resolves the configuration file path and launches
// the editor as a subprocess with standard input, output, and error connected to the current process.
func newConfigEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Open katet.toml in $EDITOR",
		RunE: func(cmd *cobra.Command, _ []string) error {
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = os.Getenv("VISUAL")
			}
			if editor == "" {
				return errors.New("no editor configured; set the EDITOR environment variable")
			}

			path, err := tower.ConfigPath(globalFlags.Config)
			if err != nil {
				return err //coverage:ignore only fails when os.UserHomeDir() fails, unreachable in unit tests
			}

			//nolint:gosec // G702: editor binary is user-controlled by design
			c := exec.CommandContext(cmd.Context(), editor, path)
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}
}

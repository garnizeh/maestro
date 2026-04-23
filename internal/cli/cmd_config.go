package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/rodrigo-baliza/maestro/internal/tower"
)

func newConfigCmd(h *Handler) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage Maestro configuration",
	}
	cmd.AddCommand(
		newConfigShowCmd(h),
		newConfigEditCmd(h),
	)
	return cmd
}

func newConfigShowCmd(h *Handler) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Display the effective configuration in TOML format",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := tower.LoadConfig(h.Config)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			w := cmd.OutOrStdout()
			switch h.Format {
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

func newConfigEditCmd(h *Handler) *cobra.Command {
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

			path, err := tower.ConfigPath(h.Config)
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

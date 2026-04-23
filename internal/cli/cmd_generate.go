package cli

import (
	"github.com/spf13/cobra"
)

func newGenerateCmd(_ *Handler) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate auxiliary files (completions, man pages)",
	}
	cmd.AddCommand(newGenerateCompletionsCmd())
	return cmd
}

func newGenerateCompletionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "completions [bash|zsh|fish|powershell]",
		Short:     "Generate shell completion scripts",
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		Args:      cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := cmd.Root()
			out := cmd.OutOrStdout()
			switch args[0] {
			case "bash":
				return root.GenBashCompletion(out)
			case "zsh":
				return root.GenZshCompletion(out)
			case "fish":
				return root.GenFishCompletion(out, true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(out)
			}
			return nil
		},
	}
}

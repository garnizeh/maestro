package cli

import (
	"github.com/spf13/cobra"
)

// newGenerateCmd creates a Cobra "generate" command for producing auxiliary files
// such as shell completions and man pages. The command registers a "completions"
// subcommand.
func newGenerateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate auxiliary files (completions, man pages)",
	}
	cmd.AddCommand(newGenerateCompletionsCmd())
	return cmd
}

// newGenerateCompletionsCmd creates a Cobra command that generates shell completion
// scripts for bash, zsh, fish, and PowerShell.
// The command requires exactly one argument specifying the shell and writes the
// generated completion script to the command's output writer.
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

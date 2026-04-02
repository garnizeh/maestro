package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

const tabwriterPadding = 2

// newVersionCmd creates a Cobra `version` command that writes build and runtime
// information to the command's output (stdout by default).
//
// The command respects the global format flag: "json" outputs indented JSON,
// "table" or empty outputs a tab-aligned table of fields, and any other value
// is rendered via the configured custom formatter.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := GetBuildInfo()
			out := cmd.OutOrStdout()
			format := globalFlags.Format

			switch strings.ToLower(format) {
			case "json":
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(info)
			case "table", "":
				w := tabwriter.NewWriter(out, 0, 0, tabwriterPadding, ' ', 0)
				fmt.Fprintf(w, "Version:\t%s\n", info.Version)
				fmt.Fprintf(w, "Commit:\t%s\n", info.Commit)
				fmt.Fprintf(w, "Build Date:\t%s\n", info.BuildDate)
				fmt.Fprintf(w, "Go Version:\t%s\n", info.GoVersion)
				fmt.Fprintf(w, "OS/Arch:\t%s/%s\n", info.OS, info.Arch)
				return w.Flush()
			default:
				f := NewFormatter(format, globalFlags.Quiet)
				s, err := f.Format(info)
				if err != nil {
					return err
				}
				fmt.Fprintln(out, s)
				return nil
			}
		},
	}
}

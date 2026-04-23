package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

const tabwriterPadding = 2

func newVersionCmd(h *Handler) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := GetBuildInfo()
			out := cmd.OutOrStdout()
			format := h.Format

			switch strings.ToLower(format) {
			case string(FormatJSON):
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(info)
			case "table", "":
				w := tabwriter.NewWriter(out, 0, 0, tabwriterPadding, ' ', 0)
				printFf(w, "Version:\t%s\n", info.Version)
				printFf(w, "Commit:\t%s\n", info.Commit)
				printFf(w, "Build Date:\t%s\n", info.BuildDate)
				printFf(w, "Go Version:\t%s\n", info.GoVersion)
				printFf(w, "OS/Arch:\t%s/%s\n", info.OS, info.Arch)
				printFf(w, "Made by:\tgarnizeH labs\n")
				return w.Flush()
			default:
				f := NewFormatter(format, h.Quiet)
				s, err := f.Format(info)
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(out, s)
				if err != nil {
					return err
				}
				return nil
			}
		},
	}
}

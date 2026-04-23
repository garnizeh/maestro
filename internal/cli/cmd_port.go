package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// newContainerPortCmd creates the "maestro container port" command.
func newContainerPortCmd(h *Handler) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "port CONTAINER [PRIVATE_PORT[/PROTO]]",
		Short: "List port mappings or a specific mapping for the container",
		Long: `List port mappings for the container.

If a private port is specified, only that mapping is shown.`,
		Args: cobra.RangeArgs(1, 2), //nolint:mnd // RangeArgs(min, max)
		RunE: func(cmd *cobra.Command, args []string) error {
			ops, err := h.ContainerOpsFn(cmd.Context(), h.StoreRoot())
			if err != nil {
				return fmt.Errorf("init: %w", err)
			}

			ctr, err := ops.LoadContainer(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			if len(ctr.Ports) == 0 {
				return nil
			}

			filterPort, filterProto := parseFilters(args)
			for _, p := range ctr.Ports {
				if !matchesFilter(p, filterPort, filterProto) {
					continue
				}
				printPort(cmd, p)
			}
			return nil
		},
	}
	return cmd
}

func parseFilters(args []string) (string, string) {
	if len(args) != 2 { //nolint:mnd // RangeArgs(1, 2)
		return "", ""
	}
	parts := strings.Split(args[1], "/")
	if len(parts) > 1 {
		return parts[0], parts[1]
	}
	return parts[0], ""
}

func matchesFilter(p, filterPort, filterProto string) bool {
	if filterPort == "" {
		return true
	}
	if !strings.Contains(p, filterPort) {
		return false
	}
	if filterProto != "" && !strings.Contains(p, "/"+filterProto) {
		return false
	}
	return true
}

func printPort(cmd *cobra.Command, p string) {
	if !strings.Contains(p, ":") {
		fmt.Fprintln(cmd.OutOrStdout(), p)
		return
	}

	parts := strings.Split(p, ":")
	var hostStr, cPort string

	switch len(parts) {
	case 2: //nolint:mnd // basic host:container map
		hostStr = "0.0.0.0:" + parts[0]
		cPort = parts[1]
	case 3: //nolint:mnd // full hostIP:hostPort:containerPort map
		hostStr = parts[0] + ":" + parts[1]
		cPort = parts[2]
	default:
		cPort = p
	}

	if !strings.Contains(cPort, "/") {
		cPort += "/tcp"
	}

	printFf(cmd.OutOrStdout(), "%s -> %s\n", cPort, hostStr)
}

package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

var errNotImplemented = errors.New("not yet implemented")

func stubCmd(use, short string, aliases ...string) *cobra.Command {
	return &cobra.Command{
		Use:     use,
		Short:   short,
		Aliases: aliases,
		RunE:    func(_ *cobra.Command, _ []string) error { return errNotImplemented },
	}
}

// ── Volume ───────────────────────────────────────────────────────────────────

func newVolumeCmd(_ *Handler) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "volume",
		Short: "Manage volumes",
	}
	cmd.AddCommand(
		stubCmd("create", "Create a volume"),
		stubCmd("ls", "List volumes", "list"),
		stubCmd("rm", "Remove one or more volumes"),
		stubCmd("inspect", "Display detailed volume information"),
		stubCmd("prune", "Remove all unused volumes"),
	)
	return cmd
}

// ── Network ──────────────────────────────────────────────────────────────────

func newNetworkCmd(h *Handler) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "network",
		Short:   "Manage networks",
		Aliases: []string{"net"},
	}
	cmd.AddCommand(
		newNetworkCreateCmd(h),
		stubCmd("ls", "List networks", "list"),
		stubCmd("rm", "Remove one or more networks"),
		stubCmd("inspect", "Display detailed network information"),
		stubCmd("connect", "Connect a container to a network"),
		stubCmd("disconnect", "Disconnect a container from a network"),
		stubCmd("prune", "Remove all unused networks"),
	)
	return cmd
}

func newNetworkCreateCmd(_ *Handler) *cobra.Command {
	return stubCmd("create", "Create a network")
}

// ── Artifact ─────────────────────────────────────────────────────────────────

func newArtifactCmd(_ *Handler) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "artifact",
		Short: "Manage OCI artifacts (ORAS)",
	}
	cmd.AddCommand(
		stubCmd("push", "Push an OCI artifact"),
		stubCmd("pull", "Pull an OCI artifact"),
		stubCmd("ls", "List artifacts in a repository", "list"),
		stubCmd("attach", "Attach a referrer artifact to a subject"),
		stubCmd("discover", "Discover referrers for a subject"),
	)
	return cmd
}

// ── System ───────────────────────────────────────────────────────────────────

func newSystemCmd(h *Handler) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "system",
		Short: "System-level operations and diagnostics",
	}
	cmd.AddCommand(
		newSystemCheckCmd(h),
		newSystemInfoCmd(h),
		newSystemMonitorCmd(h),
		stubCmd("events", "Monitor real-time system events"),
		stubCmd("df", "Show disk usage for images, containers, volumes"),
		stubCmd("prune", "Remove all unused resources"),
	)
	return cmd
}

// ── Service ──────────────────────────────────────────────────────────────────

func newServiceCmd(_ *Handler) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Manage systemd unit files for containers",
	}
	cmd.AddCommand(
		stubCmd("generate", "Generate a systemd unit file for a container"),
		stubCmd("install", "Install and enable the generated systemd unit"),
		stubCmd("uninstall", "Disable and remove the systemd unit"),
	)
	return cmd
}

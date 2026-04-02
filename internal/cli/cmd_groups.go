package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

var errNotImplemented = errors.New("not yet implemented")

// stubCmd creates a Cobra command that always fails with errNotImplemented when run.
// The command's Use, Short, and Aliases fields are populated from the provided parameters.
func stubCmd(use, short string, aliases ...string) *cobra.Command {
	return &cobra.Command{
		Use:     use,
		Short:   short,
		Aliases: aliases,
		RunE:    func(_ *cobra.Command, _ []string) error { return errNotImplemented },
	}
}

// newContainerCmd creates the top-level "container" command for managing containers.
// It sets usage, short description, and aliases, and registers stub subcommands for
// common container operations: create, start, stop, kill, rm, run, exec, ls (ps, list),
// inspect, logs, stats, pause, unpause, cp, rename, wait, and top.

func newContainerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "container",
		Short:   "Manage containers",
		Aliases: []string{"c"},
	}
	cmd.AddCommand(
		stubCmd("create", "Create a container without starting it"),
		stubCmd("start", "Start one or more stopped containers"),
		stubCmd("stop", "Stop one or more running containers"),
		stubCmd("kill", "Kill one or more running containers"),
		stubCmd("rm", "Remove one or more containers"),
		stubCmd("run", "Create and start a container"),
		stubCmd("exec", "Run a command in a running container"),
		stubCmd("ls", "List containers", "ps", "list"),
		stubCmd("inspect", "Display detailed container information"),
		stubCmd("logs", "Fetch container logs"),
		stubCmd("stats", "Display resource usage statistics"),
		stubCmd("pause", "Pause all processes in a container"),
		stubCmd("unpause", "Resume all processes in a paused container"),
		stubCmd("cp", "Copy files between host and container"),
		stubCmd("rename", "Rename a container"),
		stubCmd("wait", "Block until a container stops"),
		stubCmd("top", "Display running processes in a container"),
	)
	return cmd
}

// newImageCmd creates the top-level "image" command for managing container images.
// The command uses "image", has the short description "Manage images", includes the alias "i", and registers subcommands: pull, push, ls (list), rm, inspect, tag, save, load, build, prune, and history.

func newImageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "image",
		Short:   "Manage images",
		Aliases: []string{"i"},
	}
	cmd.AddCommand(
		stubCmd("pull", "Pull an image from a registry"),
		stubCmd("push", "Push an image to a registry"),
		stubCmd("ls", "List images", "list"),
		stubCmd("rm", "Remove one or more images"),
		stubCmd("inspect", "Display detailed image information"),
		stubCmd("tag", "Create a tag pointing to an image"),
		stubCmd("save", "Save image to a tar archive"),
		stubCmd("load", "Load image from a tar archive"),
		stubCmd("build", "Build an image from a Dockerfile"),
		stubCmd("prune", "Remove unused images"),
		stubCmd("history", "Show image layer history"),
	)
	return cmd
}

// newVolumeCmd creates the top-level "volume" Cobra command for managing volumes.
// It registers the subcommands: create, ls (alias "list"), rm, inspect, and prune.
// The returned *cobra.Command has Use "volume" and Short "Manage volumes".

func newVolumeCmd() *cobra.Command {
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

// newNetworkCmd creates a top-level "network" command for managing networks.
// The command includes subcommands: create, ls (alias: list), rm, inspect, connect, disconnect, and prune.

func newNetworkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "network",
		Short:   "Manage networks",
		Aliases: []string{"net"},
	}
	cmd.AddCommand(
		stubCmd("create", "Create a network"),
		stubCmd("ls", "List networks", "list"),
		stubCmd("rm", "Remove one or more networks"),
		stubCmd("inspect", "Display detailed network information"),
		stubCmd("connect", "Connect a container to a network"),
		stubCmd("disconnect", "Disconnect a container from a network"),
		stubCmd("prune", "Remove all unused networks"),
	)
	return cmd
}

// newArtifactCmd creates the "artifact" command for managing OCI artifacts (ORAS)
// and registers stub subcommands: push, pull, ls (alias "list"), attach, and discover.

func newArtifactCmd() *cobra.Command {
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

// newSystemCmd creates and returns a *cobra.Command named "system" for system-level
// operations and diagnostics. The command registers the stub subcommands
// check, info, events, df, and prune; each subcommand is a placeholder that
// returns the package error errNotImplemented.

func newSystemCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "system",
		Short: "System-level operations and diagnostics",
	}
	cmd.AddCommand(
		stubCmd("check", "Verify system prerequisites (runtime, rootless, networking)"),
		stubCmd("info", "Display system-wide information"),
		stubCmd("events", "Monitor real-time system events"),
		stubCmd("df", "Show disk usage for images, containers, volumes"),
		stubCmd("prune", "Remove all unused resources"),
	)
	return cmd
}

// newServiceCmd constructs the `service` command for managing systemd unit files for containers and registers the `generate`, `install`, and `uninstall` subcommands.

func newServiceCmd() *cobra.Command {
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

package cli

import (
	"github.com/spf13/cobra"
)

// Top-level shortcuts that delegate to subcommand group implementations.
// These mirror Docker's UX: `maestro run` === `maestro container run`.

func newRunCmd(h *Handler) *cobra.Command {
	return newContainerRunCmd(h)
}

func newExecCmd(_ *Handler) *cobra.Command {
	return &cobra.Command{
		Use:   "exec",
		Short: "Execute a command in a running container (shortcut for 'container exec')",
		RunE:  func(_ *cobra.Command, _ []string) error { return errNotImplemented },
	}
}

func newPsCmd(h *Handler) *cobra.Command {
	return newContainerLsCmd(h)
}

func newLogsCmd(h *Handler) *cobra.Command {
	return newContainerLogsCmd(h)
}

func newStopCmd(h *Handler) *cobra.Command {
	return newContainerStopCmd(h)
}

func newRmCmd(h *Handler) *cobra.Command {
	return newContainerRmCmd(h)
}

func newInspectCmd(h *Handler) *cobra.Command {
	return newContainerInspectCmd(h)
}

func newPushCmd(_ *Handler) *cobra.Command {
	return &cobra.Command{
		Use:   "push",
		Short: "Push an image to a registry (shortcut for 'image push')",
		RunE:  func(_ *cobra.Command, _ []string) error { return errNotImplemented },
	}
}

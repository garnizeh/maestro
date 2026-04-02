package cli

import (
	"github.com/spf13/cobra"
)

// Top-level shortcuts that delegate to subcommand group implementations.
// These mirror Docker's UX: `maestro run` === `maestro container run`.

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Create and start a container (shortcut for 'container run')",
		RunE:  func(_ *cobra.Command, _ []string) error { return errNotImplemented },
		// Flags will be added when container run is implemented (Milestone 1.3).
	}
}

func newExecCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "exec",
		Short: "Execute a command in a running container (shortcut for 'container exec')",
		RunE:  func(_ *cobra.Command, _ []string) error { return errNotImplemented },
	}
}

func newPsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ps",
		Short: "List containers (shortcut for 'container ls')",
		RunE:  func(_ *cobra.Command, _ []string) error { return errNotImplemented },
	}
}

func newPullCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pull",
		Short: "Pull an image from a registry (shortcut for 'image pull')",
		RunE:  func(_ *cobra.Command, _ []string) error { return errNotImplemented },
	}
}

func newPushCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "push",
		Short: "Push an image to a registry (shortcut for 'image push')",
		RunE:  func(_ *cobra.Command, _ []string) error { return errNotImplemented },
	}
}

func newImagesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "images",
		Short: "List images (shortcut for 'image ls')",
		RunE:  func(_ *cobra.Command, _ []string) error { return errNotImplemented },
	}
}

func newLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login [registry]",
		Short: "Log in to a container registry",
		RunE:  func(_ *cobra.Command, _ []string) error { return errNotImplemented },
	}
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout [registry]",
		Short: "Log out from a container registry",
		RunE:  func(_ *cobra.Command, _ []string) error { return errNotImplemented },
	}
}

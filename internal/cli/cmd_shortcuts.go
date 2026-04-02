package cli

import (
	"github.com/spf13/cobra"
)

// Top-level shortcuts that delegate to subcommand group implementations.
// newRunCmd returns a Cobra command named "run" that serves as a top-level shortcut for "container run" — creating and starting a container.
// The command's RunE handler currently returns errNotImplemented until the underlying container run implementation exists.

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Create and start a container (shortcut for 'container run')",
		RunE:  func(_ *cobra.Command, _ []string) error { return errNotImplemented },
		// Flags will be added when container run is implemented (Milestone 1.3).
	}
}

// newExecCmd creates a cobra.Command that serves as a top-level shortcut for "container exec".
// The command's RunE handler currently returns errNotImplemented.
func newExecCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "exec",
		Short: "Execute a command in a running container (shortcut for 'container exec')",
		RunE:  func(_ *cobra.Command, _ []string) error { return errNotImplemented },
	}
}

// newPsCmd creates a *cobra.Command for the top-level "ps" shortcut that lists containers (shortcut for "container ls").
func newPsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ps",
		Short: "List containers (shortcut for 'container ls')",
		RunE:  func(_ *cobra.Command, _ []string) error { return errNotImplemented },
	}
}

// newPullCmd creates a top-level `pull` command that serves as a shortcut for the
// `image pull` command and is described as "Pull an image from a registry".
// Its RunE handler currently returns errNotImplemented.
func newPullCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pull",
		Short: "Pull an image from a registry (shortcut for 'image pull')",
		RunE:  func(_ *cobra.Command, _ []string) error { return errNotImplemented },
	}
}

// newPushCmd creates a cobra.Command for the top-level "push" shortcut that pushes an image to a registry.
// The command is a thin UX shortcut for `image push`; its RunE handler currently returns errNotImplemented.
func newPushCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "push",
		Short: "Push an image to a registry (shortcut for 'image push')",
		RunE:  func(_ *cobra.Command, _ []string) error { return errNotImplemented },
	}
}

// newImagesCmd creates a *cobra.Command for the top-level "images" shortcut (alias for "image ls").
// The command's RunE currently returns errNotImplemented.
func newImagesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "images",
		Short: "List images (shortcut for 'image ls')",
		RunE:  func(_ *cobra.Command, _ []string) error { return errNotImplemented },
	}
}

// newLoginCmd returns a *cobra.Command implementing the "login [registry]" shortcut for logging in to a container registry.
// The command is a placeholder: its RunE currently returns errNotImplemented.
func newLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login [registry]",
		Short: "Log in to a container registry",
		RunE:  func(_ *cobra.Command, _ []string) error { return errNotImplemented },
	}
}

// newLogoutCmd creates a Cobra command named "logout" that logs out from a container registry.
// The command is a top-level shortcut and its RunE handler currently returns errNotImplemented.
func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout [registry]",
		Short: "Log out from a container registry",
		RunE:  func(_ *cobra.Command, _ []string) error { return errNotImplemented },
	}
}

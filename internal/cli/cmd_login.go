package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// defaultRegistry is the implicit registry when none is specified on the command line.
const defaultRegistry = "docker.io"

func newLoginCmd(h *Handler) *cobra.Command {
	var username, password string
	var passwordStdin bool

	cmd := &cobra.Command{
		Use:   "login [OPTIONS] [SERVER]",
		Short: "Log in to a container registry",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			registry := defaultRegistry
			if len(args) == 1 {
				registry = args[0]
			}
			return runLogin(h, cmd, registry, username, password, passwordStdin)
		},
	}
	cmd.Flags().StringVarP(&username, "username", "u", "", "Username")
	cmd.Flags().StringVarP(&password, "password", "p", "", "Password")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "Take the password from stdin")
	return cmd
}

func runLogin(
	h *Handler,
	cmd *cobra.Command,
	registry, username, password string,
	passwordStdin bool,
) error {
	// --password-stdin reads password from stdin; username must come from the flag
	// to avoid consuming stdin for both inputs.
	if passwordStdin && username == "" {
		return errors.New("--username is required when using --password-stdin")
	}

	// Resolve username interactively if not provided via flag.
	if username == "" {
		if _, err := fmt.Fprint(cmd.ErrOrStderr(), "Username: "); err != nil {
			return fmt.Errorf("write username prompt: %w", err)
		}
		u, err := h.LoginReadLineFn(cmd.InOrStdin())
		if err != nil {
			return fmt.Errorf("read username: %w", err)
		}
		username = strings.TrimSpace(u)
	}

	// Resolve password: flag → stdin pipe → interactive prompt.
	switch {
	case password != "":
		// provided via --password flag; use as-is
	case passwordStdin:
		p, err := h.LoginReadLineFn(cmd.InOrStdin())
		if err != nil {
			return fmt.Errorf("read password from stdin: %w", err)
		}
		password = strings.TrimSpace(p)
	default:
		if _, err := fmt.Fprint(cmd.ErrOrStderr(), "Password: "); err != nil {
			return fmt.Errorf("write password prompt: %w", err)
		}
		p, err := h.LoginReadPasswordFn()
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}
		if _, newlineErr := fmt.Fprintln(cmd.ErrOrStderr()); newlineErr != nil {
			return fmt.Errorf("write newline after password: %w", newlineErr)
		}
		password = p
	}

	if err := h.LoginSaveFn(registry, username, password, h.SigulConfig()); err != nil {
		return fmt.Errorf("save credentials for %s: %w", registry, err)
	}

	if !h.Quiet {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Login Succeeded"); err != nil {
			return fmt.Errorf("write login success: %w", err)
		}
	}
	return nil
}

func newLogoutCmd(h *Handler) *cobra.Command {
	return &cobra.Command{
		Use:   "logout [SERVER]",
		Short: "Log out from a container registry",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			registry := defaultRegistry
			if len(args) == 1 {
				registry = args[0]
			}
			return runLogout(h, cmd, registry)
		},
	}
}

func runLogout(h *Handler, cmd *cobra.Command, registry string) error {
	if err := h.LoginRemoveFn(registry, h.SigulConfig()); err != nil {
		return fmt.Errorf("remove credentials for %s: %w", registry, err)
	}
	if !h.Quiet {
		printFf(cmd.OutOrStdout(), "Removing login credentials for %s\n", registry)
	}
	return nil
}

// defaultReadPassword reads a password from the terminal without echoing.
func defaultReadPassword() (string, error) {
	//nolint:gosec // G115: fd fits in int on all supported 64-bit platforms
	b, err := term.ReadPassword(int(os.Stdin.Fd())) //coverage:ignore requires a real TTY
	if err != nil {
		return "", err //coverage:ignore requires a real TTY
	}
	return string(b), nil //coverage:ignore requires a real TTY
}

// defaultReadLine reads one line from r, stripping the trailing newline.
func defaultReadLine(r io.Reader) (string, error) {
	scanner := bufio.NewScanner(r)
	if scanner.Scan() {
		return scanner.Text(), nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", io.ErrUnexpectedEOF
}

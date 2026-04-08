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

	"github.com/rodrigo-baliza/maestro/internal/shardik"
)

// loginSaveFn is the DI point for saving credentials.
// Overridden in tests to avoid real filesystem writes.
//
//nolint:gochecknoglobals // dependency injection point: overridden in tests
var loginSaveFn = shardik.SaveCredentials

// loginRemoveFn is the DI point for removing credentials.
// Overridden in tests to avoid real filesystem writes.
//
//nolint:gochecknoglobals // dependency injection point: overridden in tests
var loginRemoveFn = shardik.RemoveCredentials

// loginReadPasswordFn reads a password without echoing to the terminal.
// Overridden in tests to avoid requiring a real TTY.
//
//nolint:gochecknoglobals // dependency injection point: overridden in tests
var loginReadPasswordFn = defaultReadPassword

// loginReadLineFn reads a single line from the given reader.
// Overridden in tests for error injection on the stdin read path.
//
//nolint:gochecknoglobals // dependency injection point: overridden in tests
var loginReadLineFn = defaultReadLine

// defaultRegistry is the implicit registry when none is specified on the command line.
const defaultRegistry = "docker.io"

func newLoginCmd() *cobra.Command {
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
			return runLogin(cmd, registry, username, password, passwordStdin)
		},
	}
	cmd.Flags().StringVarP(&username, "username", "u", "", "Username")
	cmd.Flags().StringVarP(&password, "password", "p", "", "Password")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "Take the password from stdin")
	return cmd
}

func runLogin(cmd *cobra.Command, registry, username, password string, passwordStdin bool) error {
	// --password-stdin reads password from stdin; username must come from the flag
	// to avoid consuming stdin for both inputs.
	if passwordStdin && username == "" {
		return errors.New("--username is required when using --password-stdin")
	}

	// Resolve username interactively if not provided via flag.
	if username == "" {
		_, _ = fmt.Fprint(cmd.ErrOrStderr(), "Username: ")
		u, err := loginReadLineFn(cmd.InOrStdin())
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
		p, err := loginReadLineFn(cmd.InOrStdin())
		if err != nil {
			return fmt.Errorf("read password from stdin: %w", err)
		}
		password = strings.TrimSpace(p)
	default:
		_, _ = fmt.Fprint(cmd.ErrOrStderr(), "Password: ")
		p, err := loginReadPasswordFn()
		if err != nil {
			return fmt.Errorf("read password: %w", err)
		}
		_, _ = fmt.Fprintln(cmd.ErrOrStderr()) // newline after hidden input
		password = p
	}

	if err := loginSaveFn(registry, username, password, ""); err != nil {
		return fmt.Errorf("save credentials for %s: %w", registry, err)
	}

	if !globalFlags.Quiet {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Login Succeeded")
	}
	return nil
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout [SERVER]",
		Short: "Log out from a container registry",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			registry := defaultRegistry
			if len(args) == 1 {
				registry = args[0]
			}
			return runLogout(cmd, registry)
		},
	}
}

func runLogout(cmd *cobra.Command, registry string) error {
	if err := loginRemoveFn(registry, ""); err != nil {
		return fmt.Errorf("remove credentials for %s: %w", registry, err)
	}
	if !globalFlags.Quiet {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Removing login credentials for %s\n", registry)
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

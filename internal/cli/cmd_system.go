package cli

import (
	"fmt"
	"os"
	"os/user"
	"runtime"

	"time"

	"github.com/spf13/cobra"

	"github.com/garnizeh/maestro/internal/bin"
	"github.com/garnizeh/maestro/internal/eld"
	"github.com/garnizeh/maestro/internal/prim"
	"github.com/garnizeh/maestro/internal/white"
)

func newSystemCheckCmd(_ *Handler) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Verify system prerequisites (runtime, rootless, networking)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), "Checking Maestro prerequisites...")

			// 1. OCI Runtime
			checkBinary(cmd, "OCI Runtime", []string{"crun", "runc", "youki"}, true)

			// 2. Rootless Support
			u, errUser := user.Current()
			if errUser != nil {
				return fmt.Errorf("current user: %w", errUser)
			}
			printFf(
				cmd.OutOrStdout(),
				"  - Current User: %s (UID: %s, GID: %s)\n",
				u.Username,
				u.Uid,
				u.Gid,
			)

			checkBinary(cmd, "Shadow-utils (rootless ID mapping)",
				[]string{"newuidmap", "newgidmap"}, os.Geteuid() != 0)
			checkSubIDs(cmd, u.Username)

			// 3. Networking
			checkBinary(
				cmd,
				"Rootless Networking",
				[]string{"pasta", "slirp4netns"},
				os.Geteuid() != 0,
			)

			// 4. Storage (FUSE fallback)
			checkBinary(cmd, "FUSE OverlayFS (fallback)", []string{"fuse-overlayfs"}, false)

			fmt.Fprintln(cmd.OutOrStdout(), "\nDone.")
			return nil
		},
	}
}

func newSystemInfoCmd(h *Handler) *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Display system-wide information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := h.ContainerOpsFn(cmd.Context(), h.StoreRoot())
			if err != nil {
				return fmt.Errorf("init: %w", err)
			}

			printFf(cmd.OutOrStdout(), "Maestro Container Engine\n")
			printFf(cmd.OutOrStdout(), "  OS/Arch:      %s/%s\n", runtime.GOOS, runtime.GOARCH)
			printFf(cmd.OutOrStdout(), "  Go Version:   %s\n", runtime.Version())

			// Runtime info (via gan.Ops)
			// We need to expose some of this info or use internal detection.
			pf := eld.NewPathfinder()
			if rt, rtErr := pf.Discover("", ""); rtErr == nil {
				printFf(cmd.OutOrStdout(), "  OCI Runtime:  %s (%s)\n", rt.Name, rt.Path)
			}

			// Storage info
			if snap, snapErr := prim.Detect(cmd.Context(), h.StoreRoot(), false, nil, nil); snapErr == nil {
				printFf(cmd.OutOrStdout(), "  Storage:      %s\n", snap.Driver)
				printFf(cmd.OutOrStdout(), "  Rootless:     %v\n", snap.Rootless)
			}

			return nil
		},
	}
}

func checkBinary(cmd *cobra.Command, label string, names []string, required bool) {
	printFf(cmd.OutOrStdout(), "  - %s: ", label)
	found := ""
	for _, n := range names {
		if p, err := bin.Find(n); err == nil {
			found = p
			break
		}
	}

	switch {
	case found != "":
		printFf(cmd.OutOrStdout(), "✅ Found (%s)\n", found)
	case required:
		printFf(cmd.OutOrStdout(), "❌ NOT FOUND (Required)\n")
	default:
		printFf(cmd.OutOrStdout(), "⚠️  Not found (Optional fallback)\n")
	}
}

func checkSubIDs(cmd *cobra.Command, username string) {
	printFf(cmd.OutOrStdout(), "  - SubUID/SubGID mapping: ")
	_, _, errUID := white.GetSubIDRange(username, "/etc/subuid")
	_, _, errGID := white.GetSubIDRange(username, "/etc/subgid")

	if errUID == nil && errGID == nil {
		printFf(cmd.OutOrStdout(), "✅ Configured\n")
		return
	}

	if errUID != nil {
		printFf(cmd.OutOrStdout(), "❌ Missing mapping in /etc/subuid or /etc/subgid\n")
	}
	if errGID != nil {
		printFf(cmd.OutOrStdout(), "❌ Missing mapping in /etc/subuid or /etc/subgid\n")
	}
}

func newSystemMonitorCmd(h *Handler) *cobra.Command {
	var (
		id           string
		bundle       string
		logPath      string
		pidFile      string
		exitFile     string
		launcherPath string
	)

	cmd := &cobra.Command{
		Use:    "monitor",
		Short:  "Internal: supervise a container process",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := h.ContainerOpsFn(cmd.Context(), h.StoreRoot())
			if err != nil {
				return err
			}

			rt, _, err := discoverRuntime()
			if err != nil {
				return err
			}
			monitor := eld.NewMonitor(rt)

			cfg := eld.MonitorConfig{
				ContainerID:  id,
				BundlePath:   bundle,
				LogPath:      logPath,
				PidFile:      pidFile,
				ExitFile:     exitFile,
				LauncherPath: launcherPath,
				Detach:       false,            // This IS the background monitor
				Timeout:      30 * time.Second, //nolint:mnd // default monitor timeout
			}

			_, err = monitor.Run(cmd.Context(), cfg)
			return err
		},
	}

	cmd.Flags().StringVar(&id, "id", "", "Container ID")
	cmd.Flags().StringVar(&bundle, "bundle", "", "Path to OCI bundle")
	cmd.Flags().StringVar(&logPath, "log", "", "Path to log file")
	cmd.Flags().StringVar(&pidFile, "pid-file", "", "Path to PID file")
	cmd.Flags().StringVar(&exitFile, "exit-file", "", "Path to exit file")
	cmd.Flags().StringVar(&launcherPath, "launcher", "", "Path to rootless netns holder socket")

	return cmd
}

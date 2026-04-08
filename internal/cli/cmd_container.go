package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/rodrigo-baliza/maestro/internal/eld"
	"github.com/rodrigo-baliza/maestro/internal/gan"
	"github.com/rodrigo-baliza/maestro/internal/prim"
	"github.com/rodrigo-baliza/maestro/internal/waystation"
)

// ── dependency injection points ───────────────────────────────────────────────

//nolint:gochecknoglobals // dependency injection point: overridden in tests or wiring
var containerOpsFn = defaultContainerOps

// defaultContainerOps builds a Gan Ops instance using the real stack.
func defaultContainerOps() (*gan.Ops, error) {
	dataRoot, err := containerDataRoot()
	if err != nil {
		return nil, err
	}

	// Initialise Waystation (state store).
	store := waystation.New(dataRoot)
	if initErr := store.Init(); initErr != nil {
		return nil, fmt.Errorf("init state store: %w", initErr)
	}

	// Discover OCI runtime.
	rt, rtInfo, err := discoverRuntime()
	if err != nil {
		return nil, fmt.Errorf("discover runtime: %w", err)
	}

	// Auto-detect snapshotter.
	snapResult, err := prim.Detect(dataRoot, false, nil)
	if err != nil {
		return nil, fmt.Errorf("detect snapshotter: %w", err)
	}

	manager := gan.NewManager(store, dataRoot)
	ops := gan.NewOps(manager, rt, rtInfo, snapResult.Prim, dataRoot)
	return ops, nil
}

// containerDataRoot returns the default data root directory.
func containerDataRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	root := home + "/.local/share/maestro"
	if mkdirErr := os.MkdirAll(root, 0o700); mkdirErr != nil {
		return "", fmt.Errorf("create data root: %w", mkdirErr)
	}
	return root, nil
}

// discoverRuntime finds the best OCI runtime on the system.
func discoverRuntime() (eld.Eld, eld.RuntimeInfo, error) {
	pf := eld.NewPathfinder()
	rtInfo, err := pf.Discover("", "")
	if err != nil {
		return nil, eld.RuntimeInfo{}, err
	}
	rt := eld.NewOCIRuntime(*rtInfo)
	return rt, *rtInfo, nil
}

// ── container run ─────────────────────────────────────────────────────────────

func newContainerRunCmd() *cobra.Command {
	var (
		name        string
		detach      bool
		rmAfter     bool
		env         []string
		capAdd      []string
		capDrop     []string
		networkMode string
		readOnly    bool
		entrypoint  string
	)

	cmd := &cobra.Command{
		Use:   "run IMAGE [COMMAND [ARG...]]",
		Short: "Create and start a container",
		Long: `Create a container from IMAGE and start it.

The image must already be pulled to the local store (use 'maestro image pull').
`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ops, err := containerOpsFn()
			if err != nil {
				return fmt.Errorf("init: %w", err)
			}

			image := args[0]
			var userCmd []string
			if len(args) > 1 {
				userCmd = args[1:]
			}

			var ep []string
			if entrypoint != "" {
				ep = strings.Fields(entrypoint)
			}

			opts := gan.RunOpts{
				Name:        name,
				Image:       image,
				Cmd:         userCmd,
				Entrypoint:  ep,
				Env:         env,
				CapAdd:      capAdd,
				CapDrop:     capDrop,
				NetworkMode: networkMode,
				ReadOnly:    readOnly,
				Detach:      detach,
				Timeout:     30 * time.Second, //nolint:mnd // default 30s container start timeout
			}

			ctr, err := ops.Run(cmd.Context(), opts)
			if err != nil {
				return err
			}

			if rmAfter && !detach {
				_ = ops.Rm(cmd.Context(), ctr.ID, gan.RmOpts{})
			}

			fmt.Fprintln(cmd.OutOrStdout(), ctr.ID)
			return nil
		},
	}

	cmd.Flags().StringVarP(&name, "name", "n", "", "Assign a name to the container")
	cmd.Flags().BoolVarP(&detach, "detach", "d", false, "Run container in the background")
	cmd.Flags().BoolVar(&rmAfter, "rm", false, "Automatically remove the container after it exits")
	cmd.Flags().StringArrayVarP(&env, "env", "e", nil, "Set environment variables (KEY=VALUE)")
	cmd.Flags().StringArrayVar(&capAdd, "cap-add", nil, "Add Linux capabilities")
	cmd.Flags().StringArrayVar(&capDrop, "cap-drop", nil, "Drop Linux capabilities")
	cmd.Flags().StringVar(&networkMode, "network", "private", "Set network mode (none|host|private)")
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "Mount the container's root filesystem as read only")
	cmd.Flags().StringVar(&entrypoint, "entrypoint", "", "Override the default ENTRYPOINT")

	return cmd
}

// ── container stop ────────────────────────────────────────────────────────────

func newContainerStopCmd() *cobra.Command {
	var (
		timeout int
		force   bool
	)

	cmd := &cobra.Command{
		Use:   "stop CONTAINER [CONTAINER...]",
		Short: "Stop one or more running containers",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ops, err := containerOpsFn()
			if err != nil {
				return fmt.Errorf("init: %w", err)
			}

			opts := gan.StopOpts{
				Signal:  syscall.SIGTERM,
				Timeout: time.Duration(timeout) * time.Second,
				Force:   force,
			}

			var errs []string
			for _, id := range args {
				if stopErr := ops.Stop(cmd.Context(), id, opts); stopErr != nil {
					errs = append(errs, fmt.Sprintf("%s: %v", id, stopErr))
					continue
				}
				fmt.Fprintln(cmd.OutOrStdout(), id)
			}
			if len(errs) > 0 {
				return fmt.Errorf("%s", strings.Join(errs, "; "))
			}
			return nil
		},
	}

	cmd.Flags().IntVarP(
		&timeout, "time", "t", 10, //nolint:mnd // default 10s stop timeout
		"Seconds to wait for the container to stop before sending SIGKILL",
	)
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Send SIGKILL immediately")

	return cmd
}

// ── container rm ──────────────────────────────────────────────────────────────

func newContainerRmCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "rm CONTAINER [CONTAINER...]",
		Short: "Remove one or more containers",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ops, err := containerOpsFn()
			if err != nil {
				return fmt.Errorf("init: %w", err)
			}

			opts := gan.RmOpts{Force: force}
			var errs []string
			for _, id := range args {
				if rmErr := ops.Rm(cmd.Context(), id, opts); rmErr != nil {
					errs = append(errs, fmt.Sprintf("%s: %v", id, rmErr))
					continue
				}
				fmt.Fprintln(cmd.OutOrStdout(), id)
			}
			if len(errs) > 0 {
				return fmt.Errorf("%s", strings.Join(errs, "; "))
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force removal of a running container")

	return cmd
}

// ── container ls (ps) ─────────────────────────────────────────────────────────

func newContainerLsCmd() *cobra.Command {
	var (
		all    bool
		format string
	)

	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"ps", "list"},
		Short:   "List containers",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ops, err := containerOpsFn()
			if err != nil {
				return fmt.Errorf("init: %w", err)
			}
			return runContainerLs(cmd, ops, all, format)
		},
	}

	cmd.Flags().BoolVarP(&all, "all", "a", false, "Show all containers (default shows only running)")
	cmd.Flags().StringVar(&format, "format", "", "Format output (table|json)")

	return cmd
}

func runContainerLs(cmd *cobra.Command, ops *gan.Ops, all bool, format string) error {
	ctrs, err := ops.ListContainers(cmd.Context())
	if err != nil {
		return err
	}

	var summaries []gan.Summary
	for _, c := range ctrs {
		if !all && c.Ka != gan.KaRunning {
			continue
		}
		summaries = append(summaries, gan.Summarise(c))
	}

	switch strings.ToLower(format) {
	case "json":
		return containerPrintJSON(cmd, summaries)
	default:
		return printContainerTable(cmd, summaries)
	}
}

func printContainerTable(cmd *cobra.Command, summaries []gan.Summary) error {
	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, containerTWPad, ' ', 0)
	fmt.Fprintln(tw, "CONTAINER ID\tNAME\tIMAGE\tSTATUS\tCREATED")
	for _, s := range summaries {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			s.ShortID, s.Name, s.Image, s.Ka, formatAge(s.Created))
	}
	return tw.Flush()
}

func containerPrintJSON(cmd *cobra.Command, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}

// ── container logs ────────────────────────────────────────────────────────────

func newContainerLogsCmd() *cobra.Command {
	var (
		follow     bool
		tail       int
		timestamps bool
	)

	cmd := &cobra.Command{
		Use:   "logs CONTAINER",
		Short: "Fetch the logs of a container",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ops, err := containerOpsFn()
			if err != nil {
				return fmt.Errorf("init: %w", err)
			}

			ctr, loadErr := ops.LoadContainer(cmd.Context(), args[0])
			if loadErr != nil {
				return loadErr
			}

			return eld.StreamLogs(cmd.Context(), ctr.LogPath, tail, follow, timestamps, cmd.OutOrStdout())
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	cmd.Flags().IntVar(&tail, "tail", -1, "Number of lines to show from the end (default: all)")
	cmd.Flags().BoolVarP(&timestamps, "timestamps", "t", false, "Show timestamps")

	return cmd
}

// ── constants ─────────────────────────────────────────────────────────────────

const containerTWPad = 3

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/rodrigo-baliza/maestro/internal/beam"
	"github.com/rodrigo-baliza/maestro/internal/eld"
	"github.com/rodrigo-baliza/maestro/internal/gan"
	"github.com/rodrigo-baliza/maestro/internal/maturin"
	"github.com/rodrigo-baliza/maestro/internal/prim"
	"github.com/rodrigo-baliza/maestro/internal/tower"
	"github.com/rodrigo-baliza/maestro/internal/waystation"
	"github.com/rodrigo-baliza/maestro/internal/white"
)

// ── subcommand constructors ───────────────────────────────────────────────────

func newContainerCmd(h *Handler) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "container",
		Short:   "Manage containers",
		Aliases: []string{"c"},
	}
	cmd.AddCommand(
		newContainerCreateCmd(h),
		newContainerStartCmd(h),
		newContainerStopCmd(h),
		newContainerKillCmd(h),
		newContainerRmCmd(h),
		newContainerRunCmd(h),
		stubCmd("exec", "Run a command in a running container"),
		newContainerLsCmd(h),
		newContainerInspectCmd(h),
		newContainerLogsCmd(h),
		stubCmd("stats", "Display resource usage statistics"),
		stubCmd("pause", "Pause all processes in a container"),
		stubCmd("unpause", "Resume all processes in a paused container"),
		stubCmd("cp", "Copy files between host and container"),
		stubCmd("rename", "Rename a container"),
		stubCmd("wait", "Block until a container stops"),
		stubCmd("top", "Display running processes in a container"),
		newContainerPortCmd(h),
	)
	return cmd
}

func newContainerRunCmd(h *Handler) *cobra.Command {
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
		ports       []string
		volumes     []string
	)

	cmd := &cobra.Command{
		Use:   "run IMAGE [COMMAND [ARG...]]",
		Short: "Create and start a container",
		Long: `Create a container from IMAGE and start it.

The image must already be pulled to the local store (use 'maestro image pull').
`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ops, err := h.ContainerOpsFn(cmd.Context(), h.StoreRoot())
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
				CreateOpts: gan.CreateOpts{
					Name:        name,
					Image:       image,
					Cmd:         userCmd,
					Entrypoint:  ep,
					Env:         env,
					CapAdd:      capAdd,
					CapDrop:     capDrop,
					NetworkMode: networkMode,
					ReadOnly:    readOnly,
					Ports:       ports,
					Volumes:     volumes,
				},
				StartOpts: gan.StartOpts{
					Detach:  detach,
					Stdout:  cmd.OutOrStdout(),
					Stderr:  cmd.ErrOrStderr(),
					Timeout: 30 * time.Second, //nolint:mnd // default 30s container start timeout
				},
			}
			log.Debug().
				Str("image", image).
				Interface("cmd", userCmd).
				Str("name", name).
				Str("network", networkMode).
				Interface("ports", ports).
				Interface("volumes", volumes).
				Msg("cli: container run")

			ctr, err := ops.Run(cmd.Context(), opts)
			if err != nil {
				return err
			}

			if rmAfter && !detach {
				if errRm := ops.Rm(cmd.Context(), ctr.ID, gan.RmOpts{}); errRm != nil {
					return fmt.Errorf("failed to remove container: %w", errRm)
				}
			}

			fmt.Fprintln(cmd.OutOrStdout(), ctr.ID)
			return nil
		},
	}

	registerRunFlags(
		cmd,
		&name,
		&detach,
		&rmAfter,
		&env,
		&capAdd,
		&capDrop,
		&networkMode,
		&readOnly,
		&entrypoint,
		&ports,
		&volumes,
	)
	return cmd
}

func registerRunFlags(
	cmd *cobra.Command,
	name *string,
	detach *bool,
	rmAfter *bool,
	env *[]string,
	capAdd *[]string,
	capDrop *[]string,
	networkMode *string,
	readOnly *bool,
	entrypoint *string,
	ports *[]string,
	volumes *[]string,
) {
	cmd.Flags().StringVarP(name, "name", "n", "", "Assign a name to the container")
	cmd.Flags().BoolVarP(detach, "detach", "d", false, "Run container in the background")
	cmd.Flags().BoolVar(rmAfter, "rm", false, "Automatically remove the container after it exits")
	cmd.Flags().StringArrayVarP(env, "env", "e", nil, "Set environment variables (KEY=VALUE)")
	cmd.Flags().StringArrayVar(capAdd, "cap-add", nil, "Add Linux capabilities")
	cmd.Flags().StringArrayVar(capDrop, "cap-drop", nil, "Drop Linux capabilities")
	cmd.Flags().
		StringVar(networkMode, "network", "private", "Set network mode (none|host|private)")
	cmd.Flags().StringSliceVarP(
		ports,
		"publish",
		"p",
		nil,
		"Publish a container's port(s) to the host (e.g., 8080:80)",
	)
	cmd.Flags().
		BoolVar(readOnly, "read-only", false, "Mount the container's root filesystem as read only")
	cmd.Flags().StringVar(entrypoint, "entrypoint", "", "Override the default ENTRYPOINT")
	cmd.Flags().
		StringArrayVarP(volumes, "volume", "v", nil, "Bind mount a volume (host-src:container-dest[:options])")
}

func newContainerCreateCmd(h *Handler) *cobra.Command {
	var (
		name        string
		env         []string
		capAdd      []string
		capDrop     []string
		networkMode string
		readOnly    bool
		entrypoint  string
		ports       []string
		volumes     []string
	)

	cmd := &cobra.Command{
		Use:   "create IMAGE [COMMAND [ARG...]]",
		Short: "Create a new container",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ops, err := h.ContainerOpsFn(cmd.Context(), h.StoreRoot())
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

			opts := gan.CreateOpts{
				Name:        name,
				Image:       image,
				Cmd:         userCmd,
				Entrypoint:  ep,
				Env:         env,
				CapAdd:      capAdd,
				CapDrop:     capDrop,
				NetworkMode: networkMode,
				ReadOnly:    readOnly,
				Ports:       ports,
				Volumes:     volumes,
			}

			log.Debug().
				Str("image", image).
				Interface("cmd", userCmd).
				Str("name", name).
				Str("network", networkMode).
				Msg("cli: container create")

			ctr, err := ops.Create(cmd.Context(), opts)
			if err != nil {
				return err
			}

			fmt.Fprintln(cmd.OutOrStdout(), ctr.ID)
			return nil
		},
	}

	cmd.Flags().StringVarP(&name, "name", "n", "", "Assign a name to the container")
	cmd.Flags().StringArrayVarP(&env, "env", "e", nil, "Set environment variables (KEY=VALUE)")
	cmd.Flags().StringArrayVar(&capAdd, "cap-add", nil, "Add Linux capabilities")
	cmd.Flags().StringArrayVar(&capDrop, "cap-drop", nil, "Drop Linux capabilities")
	cmd.Flags().
		StringVar(&networkMode, "network", "private", "Set network mode (none|host|private)")
	cmd.Flags().
		StringSliceVarP(&ports, "publish", "p", nil, "Publish a container's port(s) to the host")
	cmd.Flags().
		BoolVar(&readOnly, "read-only", false, "Mount the container's root filesystem as read only")
	cmd.Flags().StringVar(&entrypoint, "entrypoint", "", "Override the default ENTRYPOINT")
	cmd.Flags().
		StringArrayVarP(&volumes, "volume", "v", nil, "Bind mount a volume (host-src:container-dest[:options])")

	return cmd
}

func newContainerStartCmd(h *Handler) *cobra.Command {
	var (
		detach  bool
		timeout int
	)

	cmd := &cobra.Command{
		Use:   "start CONTAINER [CONTAINER...]",
		Short: "Start one or more stopped containers",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ops, err := h.ContainerOpsFn(cmd.Context(), h.StoreRoot())
			if err != nil {
				return fmt.Errorf("init: %w", err)
			}

			opts := gan.StartOpts{
				Detach:  detach,
				Timeout: time.Duration(timeout) * time.Second,
			}

			log.Debug().
				Interface("containers", args).
				Bool("detach", detach).
				Msg("cli: container start")

			var errs []string
			for _, id := range args {
				if _, startErr := ops.Start(cmd.Context(), id, opts); startErr != nil {
					errs = append(errs, fmt.Sprintf("%s: %v", id, startErr))
					continue
				}
				if detach {
					fmt.Fprintln(cmd.OutOrStdout(), id)
				}
			}
			if len(errs) > 0 {
				return fmt.Errorf("%s", strings.Join(errs, "; "))
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&detach, "detach", "d", false, "Run container in background")
	cmd.Flags().IntVarP(
		&timeout, "timeout", "t", 10, //nolint:mnd // 10s start timeout
		"Timeout to wait for the container to start",
	)

	return cmd
}

func newContainerStopCmd(h *Handler) *cobra.Command {
	var (
		timeout int
		force   bool
	)

	cmd := &cobra.Command{
		Use:   "stop CONTAINER [CONTAINER...]",
		Short: "Stop one or more running containers",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ops, err := h.ContainerOpsFn(cmd.Context(), h.StoreRoot())
			if err != nil {
				return fmt.Errorf("init: %w", err)
			}

			opts := gan.StopOpts{
				Signal:  syscall.SIGTERM,
				Timeout: time.Duration(timeout) * time.Second,
				Force:   force,
			}

			log.Debug().
				Interface("containers", args).
				Int("timeout", timeout).
				Bool("force", force).
				Msg("cli: container stop")

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

func newContainerKillCmd(h *Handler) *cobra.Command {
	var signal string

	cmd := &cobra.Command{
		Use:   "kill CONTAINER [CONTAINER...]",
		Short: "Kill one or more running containers",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ops, err := h.ContainerOpsFn(cmd.Context(), h.StoreRoot())
			if err != nil {
				return fmt.Errorf("init: %w", err)
			}

			sig := syscall.SIGKILL
			if signal != "" {
				parsed, parseErr := eld.ParseSignal(signal)
				if parseErr != nil {
					return parseErr
				}
				sig = parsed
			}

			log.Debug().
				Interface("containers", args).
				Str("signal", signal).
				Msg("cli: container kill")

			var errs []string
			for _, id := range args {
				if killErr := ops.Kill(cmd.Context(), id, sig); killErr != nil {
					errs = append(errs, fmt.Sprintf("%s: %v", id, killErr))
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

	cmd.Flags().StringVarP(&signal, "signal", "s", "SIGKILL", "Signal to send to the container")

	return cmd
}

func newContainerRmCmd(h *Handler) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "rm CONTAINER [CONTAINER...]",
		Short: "Remove one or more containers",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ops, err := h.ContainerOpsFn(cmd.Context(), h.StoreRoot())
			if err != nil {
				return fmt.Errorf("init: %w", err)
			}

			opts := gan.RmOpts{Force: force}
			log.Debug().
				Interface("containers", args).
				Bool("force", force).
				Msg("cli: container rm")

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

func newContainerLsCmd(h *Handler) *cobra.Command {
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
			ops, err := h.ContainerOpsFn(cmd.Context(), h.StoreRoot())
			if err != nil {
				return fmt.Errorf("init: %w", err)
			}
			return runContainerLs(h, cmd, ops, all, format)
		},
	}

	cmd.Flags().
		BoolVarP(&all, "all", "a", false, "Show all containers (default shows only running)")
	cmd.Flags().StringVar(&format, "format", "", "Format output (table|json)")

	return cmd
}

func runContainerLs(h *Handler, cmd *cobra.Command, ops *gan.Ops, all bool, format string) error {
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
	case string(FormatJSON):
		return containerPrintJSON(cmd, summaries)
	default:
		return printContainerTable(h, cmd, summaries)
	}
}

func printContainerTable(_ *Handler, cmd *cobra.Command, summaries []gan.Summary) error {
	tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, containerTWPad, ' ', 0)
	fmt.Fprintln(tw, "CONTAINER ID\tNAME\tIMAGE\tSTATUS\tCREATED")
	for _, s := range summaries {
		printFf(tw, "%s\t%s\t%s\t%s\t%s\n",
			s.ShortID, s.Name, s.Image, s.Ka,
			formatAge(s.Created),
		)
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

func newContainerInspectCmd(h *Handler) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect CONTAINER [CONTAINER...]",
		Short: "Display detailed container information",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ops, err := h.ContainerOpsFn(cmd.Context(), h.StoreRoot())
			if err != nil {
				return fmt.Errorf("init: %w", err)
			}

			var results []*gan.InspectResult
			for _, id := range args {
				res, inspErr := ops.Inspect(cmd.Context(), id)
				if inspErr != nil {
					return fmt.Errorf("%s: %w", id, inspErr)
				}
				results = append(results, res)
			}

			// Docker/Podman default to array output for inspect.
			data, err := json.MarshalIndent(results, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal inspect results: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}
	return cmd
}

func newContainerLogsCmd(h *Handler) *cobra.Command {
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
			ops, err := h.ContainerOpsFn(cmd.Context(), h.StoreRoot())
			if err != nil {
				return fmt.Errorf("init: %w", err)
			}

			ctr, loadErr := ops.LoadContainer(cmd.Context(), args[0])
			if loadErr != nil {
				return loadErr
			}

			return eld.StreamLogs(
				cmd.Context(),
				ctr.LogPath,
				tail,
				follow,
				timestamps,
				cmd.OutOrStdout(),
			)
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	cmd.Flags().IntVar(&tail, "tail", -1, "Number of lines to show from the end (default: all)")
	cmd.Flags().BoolVarP(&timestamps, "timestamps", "t", false, "Show timestamps")

	return cmd
}

// ── default implementations (DI targets) ─────────────────────────────────────

// defaultContainerOps builds a Gan Ops instance using the real stack.
func defaultContainerOps(ctx context.Context, dataRoot string) (*gan.Ops, error) {
	if dataRoot == "" {
		var err error
		dataRoot, err = containerDataRoot()
		if err != nil {
			return nil, err
		}
	}
	// Load Maestro config.
	cfg, err := tower.LoadConfig("")
	if err != nil {
		// Non-fatal, will use defaults.
		log.Warn().Err(err).Msg("failed to load maestro config; using defaults")
		cfg = &tower.Config{}
	}

	// Initialise Waystation (state store).
	store := waystation.New(dataRoot)
	if initErr := store.Init(); initErr != nil {
		return nil, fmt.Errorf("init state store: %w", initErr)
	}

	// Discover OCI runtime.
	rt, rtInfo, err := discoverRuntime()
	if err != nil {
		// Non-fatal for most ops, but will fail later if execution is needed.
		rt = nil
	} else {
		log.Debug().Str("name", rtInfo.Name).Str("path", rtInfo.Path).
			Str("version", rtInfo.Version).Msg("cli: discovered oci runtime")
	}

	// Auto-detect snapshotter.
	snapResult, err := prim.Detect(ctx, dataRoot, false, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("detect snapshotter: %w", err)
	}
	log.Debug().Str("snapshotter", string(snapResult.Driver)).
		Msg("cli: discovered snapshotter")

	// Initialise Maturin (image store).
	imageStore := maturin.New(dataRoot)

	beamInst := beam.NewBeam(
		filepath.Join(dataRoot, "cni", "net.d"),
		"",
		filepath.Join(dataRoot, "netns"),
	).WithRootless(os.Getuid() != 0)

	manager := gan.NewManager(store, dataRoot)
	ops := gan.NewOps(manager, rt, rtInfo, snapResult.Prim, beamInst, imageStore, dataRoot)
	ops.WithMounter(snapResult.Mounter)

	// Load seccomp profile if configured.
	if cfg.Security.DefaultSeccomp != "" &&
		cfg.Security.DefaultSeccomp != "builtin" &&
		cfg.Security.DefaultSeccomp != "unconfined" {
		if sp, errSeccomp := white.LoadSeccompProfile(cfg.Security.DefaultSeccomp); errSeccomp == nil {
			ops.WithSeccompProfile(sp)
		} else {
			log.Warn().Err(errSeccomp).Str("path", cfg.Security.DefaultSeccomp).Msg("failed to load seccomp profile")
		}
	} else if cfg.Security.DefaultSeccomp == "builtin" {
		// For Phase 1, we look for seccomp-default.json in the data root or current project configs.
		// A real "builtin" would be hardcoded in Go, but let's try to find our file first.
		searchPaths := []string{
			filepath.Join(dataRoot, "configs", "seccomp-default.json"),
			"configs/seccomp-default.json", // Development fallback
		}
		for _, p := range searchPaths {
			if sp, errSeccomp := white.LoadSeccompProfile(p); errSeccomp == nil {
				ops.WithSeccompProfile(sp)
				break
			}
		}
	}

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

// ── constants ─────────────────────────────────────────────────────────────────

const containerTWPad = 3

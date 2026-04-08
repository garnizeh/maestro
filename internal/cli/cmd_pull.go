package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/rodrigo-baliza/maestro/internal/maturin"
	"github.com/rodrigo-baliza/maestro/internal/shardik"
)

// pullDrawFn is the dependency injection point for the pull operation.
// Overridden in tests to avoid real registry and filesystem calls.
//
//nolint:gochecknoglobals // dependency injection point: overridden in tests
var pullDrawFn = defaultPullDraw

func newPullCmd() *cobra.Command {
	var platform string

	cmd := &cobra.Command{
		Use:   "pull [OPTIONS] IMAGE[:TAG|@DIGEST]",
		Short: "Pull an image from a registry (shortcut for 'image pull')",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPull(cmd, args[0], platform)
		},
	}
	cmd.Flags().StringVar(&platform, "platform", "",
		"Set platform if server is multi-platform capable (e.g. linux/arm64)")
	return cmd
}

func runPull(cmd *cobra.Command, refStr, platform string) error {
	root := globalFlags.Root
	if root == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return fmt.Errorf("determine home directory: %w", homeErr) //coverage:ignore requires system without $HOME
		}
		root = filepath.Join(home, ".local", "share", "maestro")
	}

	opts := maturin.DrawOptions{Platform: platform}

	var prog *pullProgress
	if !globalFlags.Quiet {
		prog = newPullProgress(cmd.OutOrStdout())
		opts.OnLayerDone = prog.OnLayerDone
	}

	if drawErr := pullDrawFn(cmd.Context(), root, refStr, opts); drawErr != nil {
		return fmt.Errorf("pull %s: %w", refStr, drawErr)
	}

	if !globalFlags.Quiet {
		prog.Summary(refStr)
	}
	return nil
}

func defaultPullDraw(ctx context.Context, root, refStr string, opts maturin.DrawOptions) error {
	store := maturin.New(root) //coverage:ignore wiring-only; exercised in integration tests, not unit tests
	client := shardik.New()    //coverage:ignore wiring-only; exercised in integration tests, not unit tests
	//coverage:ignore wiring-only; exercised in integration tests, not unit tests
	return store.Draw(ctx, client, refStr, opts)
}

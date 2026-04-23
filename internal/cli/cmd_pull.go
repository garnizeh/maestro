package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/garnizeh/maestro/internal/maturin"
	"github.com/garnizeh/maestro/internal/shardik"
)

func newPullCmd(h *Handler) *cobra.Command {
	var platform string

	cmd := &cobra.Command{
		Use:   "pull [OPTIONS] IMAGE[:TAG|@DIGEST]",
		Short: "Pull an image from a registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPull(h, cmd, args[0], platform)
		},
	}
	cmd.Flags().StringVar(&platform, "platform", "",
		"Set platform if server is multi-platform capable (e.g. linux/arm64)")
	return cmd
}

func runPull(h *Handler, cmd *cobra.Command, refStr, platform string) error {
	log.Debug().
		Str("ref", refStr).
		Str("platform", platform).
		Msg("cli: image pull")
	root := h.StoreRoot()
	if root == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return fmt.Errorf(
				"determine home directory: %w",
				homeErr,
			) //coverage:ignore requires system without $HOME
		}
		root = filepath.Join(home, ".local", "share", "maestro")
	}

	opts := maturin.DrawOptions{Platform: platform}

	var prog *pullProgress
	if !h.Quiet {
		prog = newPullProgress(cmd.OutOrStdout())
		opts.OnLayerDone = prog.OnLayerDone
	}

	if drawErr := h.PullDrawFn(cmd.Context(), root, refStr, opts); drawErr != nil {
		return fmt.Errorf("pull %s: %w", refStr, drawErr)
	}

	if !h.Quiet {
		prog.Summary(refStr)
	}
	return nil
}

func defaultPullDraw(ctx context.Context, root, refStr string, opts maturin.DrawOptions) error {
	store := maturin.New(
		root,
	) //coverage:ignore wiring-only; exercised in integration tests, not unit tests
	client := shardik.New() //coverage:ignore wiring-only; exercised in integration tests, not unit tests
	//coverage:ignore wiring-only; exercised in integration tests, not unit tests
	return store.Draw(ctx, client, refStr, opts)
}

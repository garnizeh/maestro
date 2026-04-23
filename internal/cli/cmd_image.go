package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/rodrigo-baliza/maestro/internal/maturin"
)

const (
	imageTWPad       = 3
	createdByMaxLen  = 60
	createdByTrimLen = 57
	hoursPerDay      = 24
)

// ── subcommand constructors ───────────────────────────────────────────────────

func newImageCmd(h *Handler) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "image",
		Short: "Manage images",
	}
	cmd.AddCommand(
		newImageLsCmd(h),
		newImageInspectCmd(h),
		newImageHistoryCmd(h),
		newImageRmCmd(h),
		newPullCmd(h), // image pull is also under image
	)
	return cmd
}

func newImageLsCmd(h *Handler) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List locally stored images",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runImageLs(h, cmd, format)
		},
	}
	cmd.Flags().StringVar(&format, "format", "", "Output format: table (default), json")
	return cmd
}

func newImageInspectCmd(h *Handler) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect IMAGE",
		Short: "Display detailed image information",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImageInspect(h, cmd, args[0])
		},
	}
}

func newImageHistoryCmd(h *Handler) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "history IMAGE",
		Short: "Show image layer history",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImageHistory(h, cmd, args[0], format)
		},
	}
	cmd.Flags().StringVar(&format, "format", "", "Output format: table (default), json")
	return cmd
}

func newImageRmCmd(h *Handler) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:     "rm IMAGE [IMAGE...]",
		Aliases: []string{"remove", "rmi"},
		Short:   "Remove one or more locally stored images",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImageRm(h, cmd, args, force)
		},
	}
	cmd.Flags().
		BoolVarP(&force, "force", "f", false, "Force removal (ignore active container check)")
	return cmd
}

// ── runners ───────────────────────────────────────────────────────────────────

func runImageLs(h *Handler, cmd *cobra.Command, format string) error {
	root := h.StoreRoot()
	log.Debug().Str("root", root).Msg("cli: image ls")
	summaries, err := h.ImageLsFn(cmd.Context(), root)
	if err != nil {
		return fmt.Errorf("image ls: %w", err)
	}

	if h.Quiet {
		for _, s := range summaries {
			if _, writeErr := fmt.Fprintln(cmd.OutOrStdout(), s.ShortID); writeErr != nil {
				return fmt.Errorf("failed to write image ID: %w", writeErr)
			}
		}
		return nil
	}

	switch format {
	case string(FormatJSON):
		b, jsonErr := json.MarshalIndent(summaries, "", "  ")
		if jsonErr != nil {
			return fmt.Errorf(
				"json: %w",
				jsonErr,
			) //coverage:ignore json.Marshal on a []ImageSummary never errors
		}
		if _, printErr := fmt.Fprintln(cmd.OutOrStdout(), string(b)); printErr != nil {
			return fmt.Errorf("failed to write JSON: %w", printErr)
		}

	default:
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, imageTWPad, ' ', 0)
		if _, writeErr := fmt.Fprintln(w, "REPOSITORY\tTAG\tIMAGE ID\tCREATED\tSIZE"); writeErr != nil {
			return fmt.Errorf("failed to write table header: %w", writeErr)
		}
		for _, s := range summaries {
			printFf(w, "%s\t%s\t%s\t%s\t%s\n",
				s.Repository, s.Tag, s.ShortID,
				formatAge(s.Created), formatBytes(s.Size),
			)
		}
		if flushErr := w.Flush(); flushErr != nil {
			return fmt.Errorf("failed to flush table writer: %w", flushErr)
		}
	}

	return nil
}

func runImageInspect(h *Handler, cmd *cobra.Command, refStr string) error {
	root := h.StoreRoot()
	log.Debug().Str("ref", refStr).Str("root", root).Msg("cli: image inspect")
	result, err := h.ImageInspectFn(root, refStr)
	if err != nil {
		return fmt.Errorf("image inspect: %w", err)
	}

	b, jsonErr := json.MarshalIndent(result, "", "  ")
	if jsonErr != nil {
		return fmt.Errorf(
			"json: %w",
			jsonErr,
		) //coverage:ignore json.Marshal on *InspectResult never errors
	}
	if _, writeErr := fmt.Fprintln(cmd.OutOrStdout(), string(b)); writeErr != nil {
		return fmt.Errorf("failed to write JSON: %w", writeErr)
	}
	return nil
}

func runImageHistory(h *Handler, cmd *cobra.Command, refStr, format string) error {
	root := h.StoreRoot()
	log.Debug().Str("ref", refStr).Str("root", root).Msg("cli: image history")
	entries, err := h.ImageHistoryFn(root, refStr)
	if err != nil {
		return fmt.Errorf("image history: %w", err)
	}

	switch format {
	case string(FormatJSON):
		b, jsonErr := json.MarshalIndent(entries, "", "  ")
		if jsonErr != nil {
			return fmt.Errorf(
				"json: %w",
				jsonErr,
			) //coverage:ignore json.Marshal on []HistoryEntry never errors
		}
		if _, printErr := fmt.Fprintln(cmd.OutOrStdout(), string(b)); printErr != nil {
			return fmt.Errorf("failed to write JSON: %w", printErr)
		}

	default:
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, imageTWPad, ' ', 0)
		if _, writeErr := fmt.Fprintln(w, "CREATED\tCREATED BY\tSIZE\tCOMMENT"); writeErr != nil {
			return fmt.Errorf("failed to write table header: %w", writeErr)
		}
		for _, e := range entries {
			createdBy := e.CreatedBy
			if len(createdBy) > createdByMaxLen {
				createdBy = createdBy[:createdByTrimLen] + "..."
			}
			printFf(w, "%s\t%s\t%s\t%s\n",
				formatAge(e.Created), createdBy,
				formatBytes(e.Size), e.Comment,
			)
		}
		if flushErr := w.Flush(); flushErr != nil {
			return fmt.Errorf("failed to flush table writer: %w", flushErr)
		}
	}

	return nil
}

func runImageRm(h *Handler, cmd *cobra.Command, refs []string, force bool) error {
	root := h.StoreRoot()
	var lastErr error
	log.Debug().Interface("refs", refs).Bool("force", force).Msg("cli: image rm")
	for _, ref := range refs {
		if err := h.ImageRmFn(cmd.Context(), root, ref); err != nil {
			printFf(cmd.ErrOrStderr(), "Error: %v\n", err)
			lastErr = err
			continue
		}
		if !h.Quiet {
			printFf(cmd.OutOrStdout(), "Deleted: %s\n", ref)
		}
	}
	return lastErr
}

// ── default implementations (DI targets) ─────────────────────────────────────

func defaultImageLs(ctx context.Context, root string) ([]maturin.ImageSummary, error) {
	return maturin.New(root).
		ListImages(ctx)
	//coverage:ignore wiring-only; exercised in integration tests
}

func defaultImageInspect(root, refStr string) (*maturin.InspectResult, error) {
	return maturin.New(root).
		InspectImage(refStr)
	//coverage:ignore wiring-only; exercised in integration tests
}

func defaultImageHistory(root, refStr string) ([]maturin.HistoryEntry, error) {
	return maturin.New(root).
		ImageHistory(refStr)
	//coverage:ignore wiring-only; exercised in integration tests
}

func defaultImageRm(ctx context.Context, root, refStr string) error {
	return maturin.New(root).
		RemoveImage(ctx, refStr)
	//coverage:ignore wiring-only; exercised in integration tests
}

// ── helpers ───────────────────────────────────────────────────────────────────

// formatAge returns a human-readable age string for the given time.
func formatAge(t time.Time) string {
	if t.IsZero() {
		return "N/A"
	}
	d := time.Since(t).Truncate(time.Second)
	switch {
	case d < time.Minute:
		return "Less than a second ago"
	case d < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
	case d < hoursPerDay*time.Hour:
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%d days ago", int(d.Hours()/hoursPerDay))
	}
}

// newImagesCmd (shortcut) is already covered by newImagesShortcut in shortcuts.go?
// No, it was in cmd_image.go. I'll move it to newImagesShortcut to match root.go's expectation.
// Wait, root.go called newImagesCmd but I named it newImagesShortcut. I'll align them.

func newImagesCmd(h *Handler) *cobra.Command {
	return &cobra.Command{
		Use:   "images",
		Short: "List images (shortcut for 'image ls')",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runImageLs(h, cmd, "")
		},
	}
}

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/rodrigo-baliza/maestro/internal/maturin"
)

const (
	imageTWPad       = 3
	createdByMaxLen  = 60
	createdByTrimLen = 57
	hoursPerDay      = 24
)

// ── dependency injection points ───────────────────────────────────────────────

//nolint:gochecknoglobals // dependency injection point: overridden in tests
var imageLsFn = defaultImageLs

//nolint:gochecknoglobals // dependency injection point: overridden in tests
var imageInspectFn = defaultImageInspect

//nolint:gochecknoglobals // dependency injection point: overridden in tests
var imageHistoryFn = defaultImageHistory

//nolint:gochecknoglobals // dependency injection point: overridden in tests
var imageRmFn = defaultImageRm

// ── subcommand constructors ───────────────────────────────────────────────────

func newImageLsCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List locally stored images",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runImageLs(cmd, format)
		},
	}
	cmd.Flags().StringVar(&format, "format", "", "Output format: table (default), json")
	return cmd
}

func newImageInspectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect IMAGE",
		Short: "Display detailed image information",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImageInspect(cmd, args[0])
		},
	}
}

func newImageHistoryCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "history IMAGE",
		Short: "Show image layer history",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImageHistory(cmd, args[0], format)
		},
	}
	cmd.Flags().StringVar(&format, "format", "", "Output format: table (default), json")
	return cmd
}

func newImageRmCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:     "rm IMAGE [IMAGE...]",
		Aliases: []string{"remove", "rmi"},
		Short:   "Remove one or more locally stored images",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImageRm(cmd, args, force)
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force removal (ignore active container check)")
	return cmd
}

// ── runners ───────────────────────────────────────────────────────────────────

func runImageLs(cmd *cobra.Command, format string) error {
	root := storeRoot()
	summaries, err := imageLsFn(cmd.Context(), root)
	if err != nil {
		return fmt.Errorf("image ls: %w", err)
	}

	if globalFlags.Quiet {
		for _, s := range summaries {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), s.ShortID)
		}
		return nil
	}

	switch format {
	case string(FormatJSON):
		b, jsonErr := json.MarshalIndent(summaries, "", "  ")
		if jsonErr != nil {
			return fmt.Errorf("json: %w", jsonErr) //coverage:ignore json.Marshal on a []ImageSummary never errors
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(b))

	default:
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, imageTWPad, ' ', 0)
		_, _ = fmt.Fprintln(w, "REPOSITORY\tTAG\tIMAGE ID\tCREATED\tSIZE")
		for _, s := range summaries {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				s.Repository, s.Tag, s.ShortID,
				formatAge(s.Created), formatBytes(s.Size),
			)
		}
		_ = w.Flush()
	}

	return nil
}

func runImageInspect(cmd *cobra.Command, refStr string) error {
	root := storeRoot()
	result, err := imageInspectFn(root, refStr)
	if err != nil {
		return fmt.Errorf("image inspect: %w", err)
	}

	b, jsonErr := json.MarshalIndent(result, "", "  ")
	if jsonErr != nil {
		return fmt.Errorf("json: %w", jsonErr) //coverage:ignore json.Marshal on *InspectResult never errors
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(b))
	return nil
}

func runImageHistory(cmd *cobra.Command, refStr, format string) error {
	root := storeRoot()
	entries, err := imageHistoryFn(root, refStr)
	if err != nil {
		return fmt.Errorf("image history: %w", err)
	}

	switch format {
	case string(FormatJSON):
		b, jsonErr := json.MarshalIndent(entries, "", "  ")
		if jsonErr != nil {
			return fmt.Errorf("json: %w", jsonErr) //coverage:ignore json.Marshal on []HistoryEntry never errors
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(b))

	default:
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, imageTWPad, ' ', 0)
		_, _ = fmt.Fprintln(w, "CREATED\tCREATED BY\tSIZE\tCOMMENT")
		for _, e := range entries {
			createdBy := e.CreatedBy
			if len(createdBy) > createdByMaxLen {
				createdBy = createdBy[:createdByTrimLen] + "..."
			}
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				formatAge(e.Created), createdBy,
				formatBytes(e.Size), e.Comment,
			)
		}
		_ = w.Flush()
	}

	return nil
}

func runImageRm(cmd *cobra.Command, refs []string, _ bool) error {
	root := storeRoot()
	var lastErr error
	for _, ref := range refs {
		if err := imageRmFn(cmd.Context(), root, ref); err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
			lastErr = err
			continue
		}
		if !globalFlags.Quiet {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Deleted: %s\n", ref)
		}
	}
	return lastErr
}

// ── default implementations (DI targets) ─────────────────────────────────────

func defaultImageLs(ctx context.Context, root string) ([]maturin.ImageSummary, error) {
	return maturin.New(root).ListImages(ctx) //coverage:ignore wiring-only; exercised in integration tests
}

func defaultImageInspect(root, refStr string) (*maturin.InspectResult, error) {
	return maturin.New(root).InspectImage(refStr) //coverage:ignore wiring-only; exercised in integration tests
}

func defaultImageHistory(root, refStr string) ([]maturin.HistoryEntry, error) {
	return maturin.New(root).ImageHistory(refStr) //coverage:ignore wiring-only; exercised in integration tests
}

func defaultImageRm(ctx context.Context, root, refStr string) error {
	return maturin.New(root).RemoveImage(ctx, refStr) //coverage:ignore wiring-only; exercised in integration tests
}

// ── helpers ───────────────────────────────────────────────────────────────────

// storeRoot returns the Maturin store root, falling back to the default path.
func storeRoot() string {
	if globalFlags.Root != "" {
		return globalFlags.Root
	}
	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		return "" //coverage:ignore requires system without $HOME
	}
	return filepath.Join(home, ".local", "share", "maestro")
}

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

// newImagesShortcut returns the `images` top-level shortcut that delegates to
// `image ls`. Used by [NewRootCommand] to wire the convenience alias.
func newImagesShortcut() *cobra.Command {
	return &cobra.Command{
		Use:   "images",
		Short: "List images (shortcut for 'image ls')",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runImageLs(cmd, "")
		},
	}
}

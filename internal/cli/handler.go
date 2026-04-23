package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/mattn/go-isatty"

	"github.com/garnizeh/maestro/internal/gan"
	"github.com/garnizeh/maestro/internal/maturin"
	"github.com/garnizeh/maestro/internal/shardik"
)

// Handler encapsulates all CLI command dependencies and global configuration.
// It replaces global variables to enable thread-safe testing and clean DI.
type Handler struct {
	// Global settings
	Config        string
	LogLevel      string
	Runtime       string
	StorageDriver string
	Root          string
	Host          string
	Format        string
	NoColor       bool
	Quiet         bool
	IsTerminalFn  func(fd uintptr) bool

	// Image dependencies
	ImageLsFn      func(context.Context, string) ([]maturin.ImageSummary, error)
	ImageInspectFn func(string, string) (*maturin.InspectResult, error)
	ImageHistoryFn func(string, string) ([]maturin.HistoryEntry, error)
	ImageRmFn      func(context.Context, string, string) error

	// Pull dependencies
	PullDrawFn func(context.Context, string, string, maturin.DrawOptions) error

	// Login dependencies
	LoginSaveFn         func(string, string, string, shardik.SigulConfig) error
	LoginRemoveFn       func(string, shardik.SigulConfig) error
	LoginReadPasswordFn func() (string, error)
	LoginReadLineFn     func(io.Reader) (string, error)

	// Container dependencies
	ContainerOpsFn func(context.Context, string) (*gan.Ops, error)
}

// NewHandler returns a Handler with production defaults.
func NewHandler() *Handler {
	return &Handler{
		LogLevel:      "warn",
		Runtime:       "auto",
		StorageDriver: "auto",
		Format:        "table",
		IsTerminalFn:  isatty.IsTerminal,

		ImageLsFn:      defaultImageLs,
		ImageInspectFn: defaultImageInspect,
		ImageHistoryFn: defaultImageHistory,
		ImageRmFn:      defaultImageRm,

		PullDrawFn: defaultPullDraw,

		LoginSaveFn:         shardik.SaveCredentials,
		LoginRemoveFn:       shardik.RemoveCredentials,
		LoginReadPasswordFn: defaultReadPassword,
		LoginReadLineFn:     defaultReadLine,

		ContainerOpsFn: defaultContainerOps,
	}
}

// StoreRoot returns the effective storage root path.
func (h *Handler) StoreRoot() string {
	if h.Root != "" {
		return h.Root
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "maestro")
}

// SigulConfig returns a shardik.SigulConfig based on handler settings.
func (h *Handler) SigulConfig() shardik.SigulConfig {
	return shardik.SigulConfig{
		HomeDir: os.UserHomeDir,
	}
}

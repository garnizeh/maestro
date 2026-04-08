package cli

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/rodrigo-baliza/maestro/internal/maturin"
)

// pullProgress accumulates per-layer events and renders them with lipgloss.
// All methods are safe to call from multiple goroutines concurrently.
type pullProgress struct {
	mu      sync.Mutex
	r       *lipgloss.Renderer
	w       io.Writer
	start   time.Time
	pulled  int
	skipped int
	bytes   int64
}

// newPullProgress creates a [pullProgress] writing to w. lipgloss auto-detects
// whether w is a TTY and disables ANSI codes on plain pipes/buffers.
func newPullProgress(w io.Writer) *pullProgress {
	return &pullProgress{
		r:     lipgloss.NewRenderer(w),
		w:     w,
		start: time.Now(),
	}
}

// OnLayerDone satisfies [maturin.ProgressFunc] and may be called concurrently.
func (p *pullProgress) OnLayerDone(ev maturin.LayerEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if ev.Skipped {
		p.skipped++
		line := p.r.NewStyle().Faint(true).Render(fmt.Sprintf("  layer %s: already present", ev.Digest))
		_, _ = fmt.Fprintln(p.w, line)
	} else {
		p.pulled++
		p.bytes += ev.Size
		line := p.r.NewStyle().Foreground(lipgloss.Color("2")).
			Render(fmt.Sprintf("  layer %s: pulled %s", ev.Digest, formatBytes(ev.Size)))
		_, _ = fmt.Fprintln(p.w, line)
	}
}

// Summary prints the pull completion line. Call once after [maturin.Store.Draw] returns.
func (p *pullProgress) Summary(refStr string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	elapsed := time.Since(p.start).Truncate(time.Millisecond)
	msg := fmt.Sprintf(
		"%s: Pull complete — %d layer(s) pulled, %d cached, %s in %s",
		refStr, p.pulled, p.skipped, formatBytes(p.bytes), elapsed,
	)
	_, _ = fmt.Fprintln(p.w, p.r.NewStyle().Bold(true).Render(msg))
}

// formatBytes converts b bytes to a human-readable string (e.g. "1.4 MB").
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

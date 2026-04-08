package maturin

import "time"

// LayerEvent is emitted once per layer when it finishes processing. Fields are
// populated for both pulled and skipped layers so callers can compute totals.
type LayerEvent struct {
	Digest   string        // 12-char short digest hex
	Skipped  bool          // true = blob already present in local CAS
	Size     int64         // compressed size in bytes; 0 if Skipped or unavailable
	Duration time.Duration // wall time spent on this layer
}

// ProgressFunc is called once per layer as it completes. Implementations must
// be safe to call from multiple goroutines concurrently.
type ProgressFunc func(LayerEvent)

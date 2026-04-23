package shardik

import (
	"context"
	"errors"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/rs/zerolog/log"
)

// Circuit breaker states.
const (
	stateClosed   = "closed"   // normal operation
	stateOpen     = "open"     // blocking requests after threshold failures
	stateHalfOpen = "halfopen" // allowing a probe request
)

// Default Horn configuration values.
const (
	defaultHornMaxRetries       = 3
	defaultHornBaseDelayMS      = 200
	defaultHornMaxDelaySec      = 5
	defaultHornFailureThreshold = 3
	defaultHornHalfOpenSec      = 60
	backoffBase                 = 2 // exponential base for retry back-off
)

// HornConfig configures the Horn retry + circuit breaker transport.
type HornConfig struct {
	// MaxRetries is the maximum number of retry attempts after the initial try.
	MaxRetries int
	// BaseDelay is the delay before the first retry; doubles on each attempt.
	BaseDelay time.Duration
	// MaxDelay caps the inter-retry wait.
	MaxDelay time.Duration
	// FailureThreshold is the number of consecutive failures that open the breaker.
	FailureThreshold int
	// HalfOpenTimeout is how long the circuit stays open before allowing a probe.
	HalfOpenTimeout time.Duration
}

// DefaultHornConfig returns a conservative production-ready configuration.
func DefaultHornConfig() HornConfig {
	return HornConfig{
		MaxRetries:       defaultHornMaxRetries,
		BaseDelay:        defaultHornBaseDelayMS * time.Millisecond,
		MaxDelay:         defaultHornMaxDelaySec * time.Second,
		FailureThreshold: defaultHornFailureThreshold,
		HalfOpenTimeout:  defaultHornHalfOpenSec * time.Second,
	}
}

// Horn is an [http.RoundTripper] that wraps another transport with retry logic
// and a circuit breaker. It is named after the Horn of Eld from The Dark Tower.
type Horn struct {
	inner  http.RoundTripper
	cfg    HornConfig
	mu     sync.Mutex
	state  string
	fails  int
	openAt time.Time
}

// NewHorn wraps inner with retry + circuit-breaker behaviour.
func NewHorn(inner http.RoundTripper, cfg HornConfig) *Horn {
	if inner == nil {
		inner = http.DefaultTransport
	}
	return &Horn{
		inner: inner,
		cfg:   cfg,
		state: stateClosed,
	}
}

// RoundTrip implements [http.RoundTripper].
func (h *Horn) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := h.checkBreaker(); err != nil {
		return nil, err
	}

	var (
		resp *http.Response
		err  error
	)
	for attempt := 0; attempt <= h.cfg.MaxRetries; attempt++ {
		if errWait := h.waitBackoff(req.Context(), attempt); errWait != nil {
			return nil, errWait
		}

		resp, err = h.doRoundTrip(req)
		if err != nil {
			continue
		}

		if !h.handleResponse(resp) {
			return resp, nil
		}
	}

	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (h *Horn) waitBackoff(ctx context.Context, attempt int) error {
	if attempt == 0 {
		return nil
	}
	delay := h.backoff(attempt)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

func (h *Horn) doRoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	resp, err := h.inner.RoundTrip(cloned)
	if err != nil {
		h.recordFailure()
		return nil, err
	}
	return resp, nil
}

func (h *Horn) handleResponse(resp *http.Response) bool {
	if isRetryableStatus(resp.StatusCode) {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Error().Err(closeErr).Msg("failed to close response body")
		}
		h.recordFailure()
		return true
	}

	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		h.recordFailure()
		return false
	}

	h.recordSuccess()
	return false
}

// checkBreaker returns an error if the circuit is open.
func (h *Horn) checkBreaker() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	switch h.state {
	case stateOpen:
		if time.Since(h.openAt) >= h.cfg.HalfOpenTimeout {
			h.state = stateHalfOpen
			return nil
		}
		return errors.New("circuit breaker open: registry temporarily unavailable")
	default:
		return nil
	}
}

func (h *Horn) recordSuccess() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.fails = 0
	h.state = stateClosed
}

func (h *Horn) recordFailure() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.fails++
	if h.fails >= h.cfg.FailureThreshold && h.state == stateClosed {
		h.state = stateOpen
		h.openAt = time.Now().UTC()
	}
}

// backoff computes the exponential back-off duration for the given attempt (1-based).
func (h *Horn) backoff(attempt int) time.Duration {
	d := float64(h.cfg.BaseDelay) * math.Pow(backoffBase, float64(attempt-1))
	if d > float64(h.cfg.MaxDelay) {
		d = float64(h.cfg.MaxDelay)
	}
	return time.Duration(d)
}

// isRetryableStatus returns true for HTTP status codes that warrant a retry.
func isRetryableStatus(code int) bool {
	switch code {
	case http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
		http.StatusTooManyRequests,
		http.StatusRequestTimeout:
		return true
	}
	return false
}

// WithHorn returns an [Option] that wraps the transport with Horn retry logic.
func WithHorn(cfg HornConfig) Option {
	return func(c *Client) {
		h := NewHorn(http.DefaultTransport, cfg)
		c.opts = append(c.opts, remote.WithTransport(h))
	}
}

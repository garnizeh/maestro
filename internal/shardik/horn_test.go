package shardik_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rodrigo-baliza/maestro/internal/shardik"
)

// ── Task #26 — Horn retry + circuit breaker ───────────────────────────────────

func TestHorn_SuccessOnFirstAttempt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := shardik.NewHorn(http.DefaultTransport, shardik.HornConfig{
		MaxRetries:       2,
		BaseDelay:        1 * time.Millisecond,
		MaxDelay:         10 * time.Millisecond,
		FailureThreshold: 3,
		HalfOpenTimeout:  100 * time.Millisecond,
	})
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := h.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Fatalf("resp.Body.Close: %v", closeErr)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestHorn_RetriesOnServerError(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := shardik.NewHorn(http.DefaultTransport, shardik.HornConfig{
		MaxRetries:       3,
		BaseDelay:        1 * time.Millisecond,
		MaxDelay:         5 * time.Millisecond,
		FailureThreshold: 5,
		HalfOpenTimeout:  1 * time.Second,
	})

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := h.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Fatalf("resp.Body.Close: %v", closeErr)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("final status = %d, want 200", resp.StatusCode)
	}
	if attempts < 3 {
		t.Errorf("attempts = %d, want >= 3", attempts)
	}
}

func TestHorn_CircuitBreakerOpensAfterThreshold(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	cfg := shardik.HornConfig{
		MaxRetries:       0,
		BaseDelay:        1 * time.Millisecond,
		MaxDelay:         1 * time.Millisecond,
		FailureThreshold: 2,
		HalfOpenTimeout:  10 * time.Second, // long enough that breaker stays open
	}
	h := shardik.NewHorn(http.DefaultTransport, cfg)

	// Trip the breaker.
	for range 3 {
		req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		resp, err := h.RoundTrip(req)
		if err == nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				t.Fatalf("resp.Body.Close: %v", closeErr)
			}
		}
	}

	// Next request should be rejected immediately by the open breaker.
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	_, err = h.RoundTrip(req)
	if err == nil {
		t.Error("expected circuit breaker error, got nil")
	}
}

func TestHorn_CircuitBreakerHalfOpenAfterTimeout(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := shardik.HornConfig{
		MaxRetries:       0,
		BaseDelay:        1 * time.Millisecond,
		MaxDelay:         1 * time.Millisecond,
		FailureThreshold: 1,
		HalfOpenTimeout:  10 * time.Millisecond,
	}
	h := shardik.NewHorn(http.DefaultTransport, cfg)

	// Trip the breaker with a server error.
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failSrv.Close()

	req, err := http.NewRequest(http.MethodGet, failSrv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := h.RoundTrip(req)
	if err == nil {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Fatalf("resp.Body.Close: %v", closeErr)
		}
	}

	// Wait for half-open window.
	time.Sleep(20 * time.Millisecond)

	// Now probe should go through.
	req2, err2 := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err2 != nil {
		t.Fatalf("NewRequest: %v", err2)
	}
	resp2, err2 := h.RoundTrip(req2)
	if err2 != nil {
		t.Fatalf("half-open probe: %v", err2)
	}
	defer func() {
		if closeErr := resp2.Body.Close(); closeErr != nil {
			t.Fatalf("resp2.Body.Close: %v", closeErr)
		}
	}()
	if calls == 0 {
		t.Error("expected probe request to reach server")
	}
}

func TestHorn_DoesNotRetry4xx(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	h := shardik.NewHorn(http.DefaultTransport, shardik.HornConfig{
		MaxRetries:       3,
		BaseDelay:        1 * time.Millisecond,
		MaxDelay:         5 * time.Millisecond,
		FailureThreshold: 10,
		HalfOpenTimeout:  1 * time.Second,
	})

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := h.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Fatalf("resp.Body.Close: %v", closeErr)
		}
	}()
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (404 should not be retried)", attempts)
	}
}

func TestHorn_DefaultHornConfig(t *testing.T) {
	cfg := shardik.DefaultHornConfig()
	if cfg.MaxRetries <= 0 {
		t.Error("MaxRetries should be > 0")
	}
	if cfg.FailureThreshold <= 0 {
		t.Error("FailureThreshold should be > 0")
	}
	if cfg.HalfOpenTimeout <= 0 {
		t.Error("HalfOpenTimeout should be > 0")
	}
}

func TestHorn_NilInnerUsesDefaultTransport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// nil inner should default to http.DefaultTransport without panic.
	h := shardik.NewHorn(nil, shardik.HornConfig{
		MaxRetries:       0,
		BaseDelay:        1 * time.Millisecond,
		MaxDelay:         5 * time.Millisecond,
		FailureThreshold: 3,
		HalfOpenTimeout:  1 * time.Second,
	})
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := h.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip with nil inner: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Fatalf("resp.Body.Close: %v", closeErr)
		}
	}()
}

func TestHorn_ContextCancelledDuringBackoff(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	h := shardik.NewHorn(http.DefaultTransport, shardik.HornConfig{
		MaxRetries:       5,
		BaseDelay:        500 * time.Millisecond, // long enough to be cancelled
		MaxDelay:         2 * time.Second,
		FailureThreshold: 10,
		HalfOpenTimeout:  10 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	// Cancel after the first attempt fires.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err = h.RoundTrip(req)
	if err == nil {
		t.Fatal("expected error after context cancellation")
	}
}

func TestHorn_TransportError(t *testing.T) {
	// Use a port that refuses connections to trigger a transport-level error.
	h := shardik.NewHorn(http.DefaultTransport, shardik.HornConfig{
		MaxRetries:       1,
		BaseDelay:        1 * time.Millisecond,
		MaxDelay:         1 * time.Millisecond,
		FailureThreshold: 10,
		HalfOpenTimeout:  10 * time.Second,
	})
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:1/", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	_, err = h.RoundTrip(req)
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestHorn_BackoffCapsAtMaxDelay(t *testing.T) {
	// Use a large number of retries with a very small MaxDelay to hit the cap.
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < 4 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := shardik.NewHorn(http.DefaultTransport, shardik.HornConfig{
		MaxRetries:       5,
		BaseDelay:        10 * time.Millisecond,
		MaxDelay:         10 * time.Millisecond, // cap equals base → always capped after first retry
		FailureThreshold: 10,
		HalfOpenTimeout:  10 * time.Second,
	})

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := h.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Fatalf("resp.Body.Close: %v", closeErr)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

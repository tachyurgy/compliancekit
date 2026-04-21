package integration

import (
	"net/http"
	"testing"

	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
)

// TestRateLimit_AuthBurstReturns429 constructs a harness with a TIGHT
// token bucket (5 capacity, 0.1 refill/s ≈ 1 token per 10s). The 6th
// /api/auth/signin request within a burst should be 429.
func TestRateLimit_AuthBurstReturns429(t *testing.T) {
	bucket := mw.NewTokenBucket(5, 0.1)
	h := NewHarnessWithOpts(t, HarnessOpts{RateLimit: bucket})

	seen429 := false
	for i := 0; i < 15; i++ {
		resp, err := http.Post(h.URL("/api/auth/signin"), "application/json", stringReader(`{"email":"nobody@example.com"}`))
		if err != nil {
			t.Fatalf("POST #%d: %v", i, err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			seen429 = true
			// Retry-After header should be set per the rate-limiter.
			if retryAfter := resp.Header.Get("Retry-After"); retryAfter == "" {
				t.Fatalf("request #%d got 429 but no Retry-After header", i)
			}
			break
		}
	}
	if !seen429 {
		t.Fatalf("expected at least one 429 across 15 rapid signin calls; none observed")
	}
}

// TestRateLimit_NonAuthRoutesNotThrottled: rate limiting only wraps the
// /api/auth/* group in router.go. A burst against /healthz should never
// 429 even with a tiny bucket (healthz is outside the rate-limited group).
func TestRateLimit_NonAuthRoutesNotThrottled(t *testing.T) {
	bucket := mw.NewTokenBucket(1, 0.01)
	h := NewHarnessWithOpts(t, HarnessOpts{RateLimit: bucket})

	for i := 0; i < 10; i++ {
		resp, err := http.Get(h.URL("/healthz"))
		if err != nil {
			t.Fatalf("GET #%d: %v", i, err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			t.Fatalf("unexpected 429 on /healthz at request #%d — rate limiter applied outside auth group", i)
		}
	}
}

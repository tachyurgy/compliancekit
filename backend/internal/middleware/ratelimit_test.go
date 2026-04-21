package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/middleware"
)

func TestTokenBucket_Burst_AllowsCapacityRequests(t *testing.T) {
	t.Parallel()
	// capacity=5, refill=0/sec — only the initial burst is allowed.
	tb := middleware.NewTokenBucket(5, 0)
	h := tb.Limit("test")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	allowed := 0
	denied := 0
	for i := 0; i < 20; i++ {
		req := httptest.NewRequest("GET", "/x", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code == http.StatusOK {
			allowed++
		} else if rec.Code == http.StatusTooManyRequests {
			denied++
		}
	}
	if allowed != 5 {
		t.Fatalf("allowed = %d, want 5", allowed)
	}
	if denied != 15 {
		t.Fatalf("denied = %d, want 15", denied)
	}
}

func TestTokenBucket_Refill_RestoresTokens(t *testing.T) {
	t.Parallel()
	// capacity=1, refill=100/sec → one token every 10ms.
	tb := middleware.NewTokenBucket(1, 100)
	h := tb.Limit("refill")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	fire := func() int {
		req := httptest.NewRequest("GET", "/x", nil)
		req.RemoteAddr = "1.2.3.4:9"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	// First call allowed (uses 1-token burst).
	if got := fire(); got != http.StatusOK {
		t.Fatalf("first = %d, want 200", got)
	}
	// Immediate second call denied.
	if got := fire(); got != http.StatusTooManyRequests {
		t.Fatalf("second = %d, want 429", got)
	}
	// After sleeping long enough to refill, allowed again.
	time.Sleep(50 * time.Millisecond)
	if got := fire(); got != http.StatusOK {
		t.Fatalf("after refill = %d, want 200", got)
	}
}

func TestTokenBucket_PerKeyIsolation(t *testing.T) {
	t.Parallel()
	tb := middleware.NewTokenBucket(1, 0)
	h := tb.Limit("scope1")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	// IP A exhausts its single token.
	reqA := httptest.NewRequest("GET", "/x", nil)
	reqA.RemoteAddr = "10.0.0.1:1"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, reqA)
	if rec.Code != http.StatusOK {
		t.Fatalf("A first = %d", rec.Code)
	}
	// IP B should still have its own fresh bucket.
	reqB := httptest.NewRequest("GET", "/x", nil)
	reqB.RemoteAddr = "10.0.0.2:1"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, reqB)
	if rec.Code != http.StatusOK {
		t.Fatalf("B first = %d", rec.Code)
	}
	// IP A second call now denied.
	reqA2 := httptest.NewRequest("GET", "/x", nil)
	reqA2.RemoteAddr = "10.0.0.1:2"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, reqA2)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("A second = %d, want 429", rec.Code)
	}
}

func TestTokenBucket_DifferentScopesDontShareBucket(t *testing.T) {
	t.Parallel()
	tb := middleware.NewTokenBucket(1, 0)
	h1 := tb.Limit("scopeA")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	h2 := tb.Limit("scopeB")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, h := range []http.Handler{h1, h2} {
		req := httptest.NewRequest("GET", "/x", nil)
		req.RemoteAddr = "127.0.0.1:1"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("scope-first call = %d", rec.Code)
		}
	}
}

func TestTokenBucket_ConcurrentAccess_Safe(t *testing.T) {
	t.Parallel()
	tb := middleware.NewTokenBucket(100, 1000)
	h := tb.Limit("concurrent")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	var ok, not int64
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				req := httptest.NewRequest("GET", "/x", nil)
				req.RemoteAddr = "10.0.0.1:1" // same key → hit same bucket
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				if rec.Code == http.StatusOK {
					atomic.AddInt64(&ok, 1)
				} else {
					atomic.AddInt64(&not, 1)
				}
			}
		}(i)
	}
	wg.Wait()

	// Just ensure we didn't race-crash and each request got a verdict.
	if ok+not != 400 {
		t.Fatalf("ok+not = %d, want 400", ok+not)
	}
}

func TestTokenBucket_UsesXForwardedFor(t *testing.T) {
	t.Parallel()
	tb := middleware.NewTokenBucket(1, 0)
	h := tb.Limit("xff")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	// Two requests from different X-Forwarded-For IPs but same RemoteAddr:
	// each should get its own bucket.
	fire := func(xff string) int {
		req := httptest.NewRequest("GET", "/x", nil)
		req.RemoteAddr = "127.0.0.1:9999" // same for both
		req.Header.Set("X-Forwarded-For", xff)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if got := fire("1.1.1.1"); got != http.StatusOK {
		t.Fatalf("first = %d", got)
	}
	if got := fire("2.2.2.2"); got != http.StatusOK {
		t.Fatalf("second = %d", got)
	}
	// A repeat of the first IP should now be limited.
	if got := fire("1.1.1.1"); got != http.StatusTooManyRequests {
		t.Fatalf("repeat = %d, want 429", got)
	}
}

func TestTokenBucket_RetryAfterHeaderSetOnDeny(t *testing.T) {
	t.Parallel()
	tb := middleware.NewTokenBucket(1, 0)
	h := tb.Limit("ra")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	// consume
	req := httptest.NewRequest("GET", "/x", nil)
	req.RemoteAddr = "3.3.3.3:1"
	h.ServeHTTP(httptest.NewRecorder(), req)
	// deny
	req = httptest.NewRequest("GET", "/x", nil)
	req.RemoteAddr = "3.3.3.3:2"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("code = %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After header missing")
	}
}

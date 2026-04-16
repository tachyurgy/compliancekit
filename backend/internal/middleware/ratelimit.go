package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
)

type bucket struct {
	tokens    float64
	updatedAt time.Time
}

// TokenBucket is a simple in-memory rate limiter keyed by IP+scope.
// Not for multi-node deployments — swap for Redis when horizontally scaled.
type TokenBucket struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	capacity float64
	refill   float64 // tokens per second
}

func NewTokenBucket(capacity int, refillPerSec float64) *TokenBucket {
	return &TokenBucket{
		buckets:  make(map[string]*bucket),
		capacity: float64(capacity),
		refill:   refillPerSec,
	}
}

func (t *TokenBucket) allow(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	b, ok := t.buckets[key]
	if !ok {
		b = &bucket{tokens: t.capacity, updatedAt: now}
		t.buckets[key] = b
	}
	elapsed := now.Sub(b.updatedAt).Seconds()
	b.tokens = min(t.capacity, b.tokens+elapsed*t.refill)
	b.updatedAt = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Limit returns middleware enforcing the given scope. Scope disambiguates per-endpoint buckets.
func (t *TokenBucket) Limit(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := scope + "|" + clientIP(r)
			if !t.allow(key) {
				w.Header().Set("Retry-After", "30")
				httpx.RenderError(w, r, &httpx.APIError{
					Status: http.StatusTooManyRequests, Code: "rate_limited",
					Message: "too many requests",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if comma := strings.Index(xff, ","); comma > 0 {
			return strings.TrimSpace(xff[:comma])
		}
		return strings.TrimSpace(xff)
	}
	if xr := r.Header.Get("X-Real-IP"); xr != "" {
		return strings.TrimSpace(xr)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

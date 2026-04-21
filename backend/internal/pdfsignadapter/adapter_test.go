package pdfsignadapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
)

// TestSessionAuth_ProviderIDFromContext verifies the adapter reads the same
// context key that middleware.RequireProviderSession writes. This is the one
// non-trivial thing the adapter does; the rest is composition.
func TestSessionAuth_ProviderIDFromContext(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/foo", nil)
	ctx := context.WithValue(req.Context(), mw.CtxKeyProviderID, "prov_123")
	req = req.WithContext(ctx)

	auth := sessionAuth{}
	id, ok := auth.ProviderID(req)
	if !ok || id != "prov_123" {
		t.Fatalf("expected (prov_123, true), got (%q, %v)", id, ok)
	}
}

// TestSessionAuth_NoProviderID_ReturnsFalse ensures a request without a
// session context value is treated as unauthenticated, not as the empty
// string provider.
func TestSessionAuth_NoProviderID_ReturnsFalse(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/foo", nil)
	auth := sessionAuth{}
	id, ok := auth.ProviderID(req)
	if ok {
		t.Fatalf("expected ok=false for missing context, got true with id=%q", id)
	}
	if id != "" {
		t.Fatalf("expected empty id for missing context, got %q", id)
	}
}

// TestSessionAuth_EmptyStringProviderID_TreatedAsMissing rejects the edge
// case where the context key is set but to the empty string — we must not
// hand pdfsign an empty provider ID as if authenticated.
func TestSessionAuth_EmptyStringProviderID_TreatedAsMissing(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/foo", nil)
	ctx := context.WithValue(req.Context(), mw.CtxKeyProviderID, "")
	req = req.WithContext(ctx)

	auth := sessionAuth{}
	id, ok := auth.ProviderID(req)
	if ok {
		t.Fatalf("empty string must not pass auth; got (%q, true)", id)
	}
}

// TestMountPublicRoutes_IsNoOp verifies the public-routes hook doesn't alter
// the router. Regression guard: if someone adds a real implementation there
// without updating the caller, the test flags it.
func TestMountPublicRoutes_IsNoOp(t *testing.T) {
	t.Parallel()
	a := &Adapter{} // nil handlers fine; MountPublicRoutes doesn't touch them
	r := chi.NewRouter()
	a.MountPublicRoutes(r)
	// chi doesn't expose a direct route count; instead, hit a made-up path and
	// confirm it 404s (because no route was ever added).
	srv := httptest.NewServer(r)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/anything")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 on untouched router, got %d", resp.StatusCode)
	}
}

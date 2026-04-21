package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/tachyurgy/compliancekit/backend/internal/middleware"
	"github.com/tachyurgy/compliancekit/backend/internal/testhelp"
)

type fakeRoleLookup struct {
	role  string
	err   error
	calls int32
}

func (f *fakeRoleLookup) LookupUserRole(ctx context.Context, userID string) (string, error) {
	atomic.AddInt32(&f.calls, 1)
	return f.role, f.err
}

func TestRequireRole_Match_CallsNext(t *testing.T) {
	t.Parallel()
	lookup := &fakeRoleLookup{role: middleware.RoleProviderAdmin}
	var reached bool
	h := middleware.RequireRole(lookup, middleware.RoleProviderAdmin)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		if r := middleware.UserRoleFrom(r.Context()); r != middleware.RoleProviderAdmin {
			t.Errorf("UserRoleFrom=%q", r)
		}
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	ctx := context.WithValue(req.Context(), middleware.CtxKeyUserID, "u1")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !reached {
		t.Fatalf("next handler not reached; status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestRequireRole_Mismatch_Returns403(t *testing.T) {
	t.Parallel()
	lookup := &fakeRoleLookup{role: middleware.RoleProviderStaff}
	h := middleware.RequireRole(lookup, middleware.RoleProviderAdmin)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not run")
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	ctx := context.WithValue(req.Context(), middleware.CtxKeyUserID, "u1")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestRequireRole_NoUserOnCtx_Returns401(t *testing.T) {
	t.Parallel()
	lookup := &fakeRoleLookup{role: middleware.RoleProviderAdmin}
	h := middleware.RequireRole(lookup, middleware.RoleProviderAdmin)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not run")
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRequireRole_LookupErr_Returns500(t *testing.T) {
	t.Parallel()
	lookup := &fakeRoleLookup{err: errors.New("boom")}
	h := middleware.RequireRole(lookup, middleware.RoleProviderAdmin)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not run")
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	ctx := context.WithValue(req.Context(), middleware.CtxKeyUserID, "u1")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestRequireRole_UnknownUser_Returns403(t *testing.T) {
	t.Parallel()
	lookup := &fakeRoleLookup{role: ""} // user row not found
	h := middleware.RequireRole(lookup, middleware.RoleProviderAdmin)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not run")
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	ctx := context.WithValue(req.Context(), middleware.CtxKeyUserID, "u1")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestRequireRole_RoleCached_NoSecondLookup(t *testing.T) {
	t.Parallel()
	lookup := &fakeRoleLookup{role: middleware.RoleProviderAdmin}

	// Chain two RequireRole middlewares. The second should read the cached role
	// without re-querying.
	mw1 := middleware.RequireRole(lookup, middleware.RoleProviderAdmin)
	mw2 := middleware.RequireRole(lookup, middleware.RoleProviderAdmin)
	h := mw1(mw2(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest("GET", "/x", nil)
	ctx := context.WithValue(req.Context(), middleware.CtxKeyUserID, "u1")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// Only one lookup should have fired.
	if got := atomic.LoadInt32(&lookup.calls); got != 1 {
		t.Fatalf("lookup called %d times, want 1 (cached)", got)
	}
}

// ---- PoolRoleLookup tests ----

func TestPoolRoleLookup_EmptyUserID_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	pool := testhelp.OpenDB(t)
	lookup := middleware.PoolRoleLookup{DB: pool}
	role, err := lookup.LookupUserRole(context.Background(), "")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if role != "" {
		t.Fatalf("role = %q, want empty", role)
	}
}

func TestPoolRoleLookup_NonexistentUser_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	pool := testhelp.OpenDB(t)
	lookup := middleware.PoolRoleLookup{DB: pool}
	role, err := lookup.LookupUserRole(context.Background(), "nobody")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if role != "" {
		t.Fatalf("role = %q, want empty", role)
	}
}

func TestPoolRoleLookup_FindsRealRole(t *testing.T) {
	t.Parallel()
	pool := testhelp.OpenDB(t)
	_, err := pool.Exec(`
INSERT INTO providers (id, legal_name, state) VALUES ('p1','T','CA');
INSERT INTO users (id, provider_id, email, full_name, role) VALUES ('u1','p1','a@b.com','Admin','provider_admin');
`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	lookup := middleware.PoolRoleLookup{DB: pool}
	role, err := lookup.LookupUserRole(context.Background(), "u1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if role != middleware.RoleProviderAdmin {
		t.Fatalf("role = %q, want provider_admin", role)
	}
}

func TestPoolRoleLookup_IgnoresDeletedUsers(t *testing.T) {
	t.Parallel()
	pool := testhelp.OpenDB(t)
	_, err := pool.Exec(`
INSERT INTO providers (id, legal_name, state) VALUES ('p1','T','CA');
INSERT INTO users (id, provider_id, email, full_name, role, deleted_at) VALUES ('u1','p1','a@b.com','Admin','provider_admin', CURRENT_TIMESTAMP);
`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	lookup := middleware.PoolRoleLookup{DB: pool}
	role, err := lookup.LookupUserRole(context.Background(), "u1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if role != "" {
		t.Fatalf("role = %q, want empty for deleted user", role)
	}
}

func TestUserRoleFrom_EmptyCtx(t *testing.T) {
	t.Parallel()
	if got := middleware.UserRoleFrom(context.Background()); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

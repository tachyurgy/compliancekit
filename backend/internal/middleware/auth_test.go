package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tachyurgy/compliancekit/backend/internal/magiclink"
	"github.com/tachyurgy/compliancekit/backend/internal/middleware"
)

type fakeSession struct {
	pid, uid string
	err      error
}

func (f fakeSession) LookupSession(ctx context.Context, token string) (string, string, error) {
	return f.pid, f.uid, f.err
}

func handlerThatReadsCtx(t *testing.T, wantPID, wantUID string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pid := middleware.ProviderIDFrom(r.Context())
		uid := middleware.UserIDFrom(r.Context())
		if pid != wantPID {
			t.Errorf("ProviderIDFrom = %q, want %q", pid, wantPID)
		}
		if uid != wantUID {
			t.Errorf("UserIDFrom = %q, want %q", uid, wantUID)
		}
		w.WriteHeader(http.StatusOK)
	})
}

func TestRequireProviderSession_NoCookie_Returns401(t *testing.T) {
	t.Parallel()
	mw := middleware.RequireProviderSession(fakeSession{pid: "p1", uid: "u1"})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRequireProviderSession_EmptyCookie_Returns401(t *testing.T) {
	t.Parallel()
	mw := middleware.RequireProviderSession(fakeSession{pid: "p1", uid: "u1"})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	req.AddCookie(&http.Cookie{Name: "ck_sess", Value: ""})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRequireProviderSession_InvalidSession_Returns401(t *testing.T) {
	t.Parallel()
	mw := middleware.RequireProviderSession(fakeSession{err: errors.New("not found")})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	req.AddCookie(&http.Cookie{Name: "ck_sess", Value: "bogus"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRequireProviderSession_EmptyProviderID_Returns401(t *testing.T) {
	t.Parallel()
	mw := middleware.RequireProviderSession(fakeSession{pid: "", uid: "u1"})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	req.AddCookie(&http.Cookie{Name: "ck_sess", Value: "valid-looking"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRequireProviderSession_ValidSession_InjectsContext(t *testing.T) {
	t.Parallel()
	mw := middleware.RequireProviderSession(fakeSession{pid: "prov-1", uid: "user-1"})
	h := mw(handlerThatReadsCtx(t, "prov-1", "user-1"))
	req := httptest.NewRequest("GET", "/x", nil)
	req.AddCookie(&http.Cookie{Name: "ck_sess", Value: "good"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

// ---- RequireIndividualMagicLink tests ----

type fakeMagicConsumer struct {
	claim *magiclink.Claim
	err   error
	got   string
}

func (f *fakeMagicConsumer) Consume(ctx context.Context, token string) (*magiclink.Claim, error) {
	f.got = token
	return f.claim, f.err
}

func TestRequireMagicLink_MissingToken_Returns401(t *testing.T) {
	t.Parallel()
	mc := &fakeMagicConsumer{}
	h := middleware.RequireIndividualMagicLink(mc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not run")
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRequireMagicLink_BadToken_Returns401(t *testing.T) {
	t.Parallel()
	mc := &fakeMagicConsumer{err: errors.New("not found")}
	h := middleware.RequireIndividualMagicLink(mc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not run")
	}))
	req := httptest.NewRequest("GET", "/x?t=nope", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRequireMagicLink_ClaimOnCtx(t *testing.T) {
	t.Parallel()
	mc := &fakeMagicConsumer{claim: &magiclink.Claim{Kind: magiclink.KindParentUpload, ProviderID: "prov-1", SubjectID: "c-1"}}
	var got *magiclink.Claim
	h := middleware.RequireIndividualMagicLink(mc, magiclink.KindParentUpload)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = middleware.MagicClaimFrom(r.Context())
		if pid := middleware.ProviderIDFrom(r.Context()); pid != "prov-1" {
			t.Errorf("provider id not on ctx: %q", pid)
		}
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/x?t=good", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got == nil {
		t.Fatal("claim not on ctx")
	}
	if got.Kind != magiclink.KindParentUpload {
		t.Fatalf("got.Kind = %s", got.Kind)
	}
}

func TestRequireMagicLink_DisallowedKind_Returns403(t *testing.T) {
	t.Parallel()
	mc := &fakeMagicConsumer{claim: &magiclink.Claim{Kind: magiclink.KindProviderSignin}}
	h := middleware.RequireIndividualMagicLink(mc, magiclink.KindParentUpload)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not run")
	}))
	req := httptest.NewRequest("GET", "/x?t=good", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestRequireMagicLink_BearerAuth(t *testing.T) {
	t.Parallel()
	mc := &fakeMagicConsumer{claim: &magiclink.Claim{Kind: magiclink.KindDocumentSign, ProviderID: "p1"}}
	h := middleware.RequireIndividualMagicLink(mc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer abcdef")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if mc.got != "abcdef" {
		t.Fatalf("consumer got %q, want abcdef", mc.got)
	}
}

// ---- RequireStripeCustomer tests ----

type fakeBilling struct {
	ok  bool
	err error
}

func (f fakeBilling) HasActiveSubscription(ctx context.Context, providerID string) (bool, error) {
	return f.ok, f.err
}

func TestRequireStripeCustomer_NoProvider_Returns401(t *testing.T) {
	t.Parallel()
	h := middleware.RequireStripeCustomer(fakeBilling{ok: true})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not run")
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRequireStripeCustomer_NoSubscription_Returns402(t *testing.T) {
	t.Parallel()
	h := middleware.RequireStripeCustomer(fakeBilling{ok: false})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not run")
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	ctx := context.WithValue(req.Context(), middleware.CtxKeyProviderID, "p1")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402", rec.Code)
	}
}

func TestRequireStripeCustomer_Active_Passes(t *testing.T) {
	t.Parallel()
	h := middleware.RequireStripeCustomer(fakeBilling{ok: true})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	ctx := context.WithValue(req.Context(), middleware.CtxKeyProviderID, "p1")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

// ---- Context helpers ----

func TestCtxHelpers_ReturnEmptyForMissing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if got := middleware.ProviderIDFrom(ctx); got != "" {
		t.Fatalf("ProviderIDFrom = %q, want empty", got)
	}
	if got := middleware.UserIDFrom(ctx); got != "" {
		t.Fatalf("UserIDFrom = %q, want empty", got)
	}
	if got := middleware.MagicClaimFrom(ctx); got != nil {
		t.Fatalf("MagicClaimFrom = %v, want nil", got)
	}
}

func TestCtxHelpers_ReturnValueWhenSet(t *testing.T) {
	t.Parallel()
	ctx := context.WithValue(context.Background(), middleware.CtxKeyProviderID, "p1")
	ctx = context.WithValue(ctx, middleware.CtxKeyUserID, "u1")
	claim := &magiclink.Claim{Kind: magiclink.KindStaffUpload}
	ctx = context.WithValue(ctx, middleware.CtxKeyMagicClaim, claim)
	if got := middleware.ProviderIDFrom(ctx); got != "p1" {
		t.Fatalf("got %q", got)
	}
	if got := middleware.UserIDFrom(ctx); got != "u1" {
		t.Fatalf("got %q", got)
	}
	if got := middleware.MagicClaimFrom(ctx); got != claim {
		t.Fatalf("got %v", got)
	}
}

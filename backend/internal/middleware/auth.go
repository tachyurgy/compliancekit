package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	"github.com/tachyurgy/compliancekit/backend/internal/magiclink"
)

type ctxKey string

const (
	CtxKeyProviderID ctxKey = "provider_id"
	CtxKeyUserID     ctxKey = "user_id"
	CtxKeyMagicClaim ctxKey = "magic_claim"
)

// SessionReader knows how to resolve a provider session from a cookie.
type SessionReader interface {
	LookupSession(ctx context.Context, token string) (providerID, userID string, err error)
}

// BillingChecker confirms a provider has an active Stripe subscription.
type BillingChecker interface {
	HasActiveSubscription(ctx context.Context, providerID string) (bool, error)
}

// MagicConsumer wraps magiclink.Service's Consume to avoid import cycles when we want to mock.
type MagicConsumer interface {
	Consume(ctx context.Context, token string) (*magiclink.Claim, error)
}

// RequireProviderSession reads the ck_sess cookie and loads provider/user IDs into ctx.
func RequireProviderSession(sr SessionReader) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie("ck_sess")
			if err != nil || c.Value == "" {
				httpx.RenderError(w, r, httpx.ErrUnauthorized)
				return
			}
			pid, uid, err := sr.LookupSession(r.Context(), c.Value)
			if err != nil || pid == "" {
				httpx.RenderError(w, r, httpx.ErrUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), CtxKeyProviderID, pid)
			ctx = context.WithValue(ctx, CtxKeyUserID, uid)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireIndividualMagicLink extracts a token from ?t=... or path and resolves it.
// Stores the Claim on ctx under CtxKeyMagicClaim.
func RequireIndividualMagicLink(mc MagicConsumer, allowed ...magiclink.Kind) func(http.Handler) http.Handler {
	allow := map[magiclink.Kind]struct{}{}
	for _, k := range allowed {
		allow[k] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.URL.Query().Get("t")
			if token == "" {
				// allow Authorization: Bearer
				h := r.Header.Get("Authorization")
				if strings.HasPrefix(h, "Bearer ") {
					token = strings.TrimPrefix(h, "Bearer ")
				}
			}
			if token == "" {
				httpx.RenderError(w, r, httpx.ErrUnauthorized)
				return
			}
			claim, err := mc.Consume(r.Context(), token)
			if err != nil || claim == nil {
				httpx.RenderError(w, r, httpx.Wrap(httpx.ErrUnauthorized, err))
				return
			}
			if len(allow) > 0 {
				if _, ok := allow[claim.Kind]; !ok {
					httpx.RenderError(w, r, httpx.ErrForbidden)
					return
				}
			}
			ctx := context.WithValue(r.Context(), CtxKeyMagicClaim, claim)
			ctx = context.WithValue(ctx, CtxKeyProviderID, claim.ProviderID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireStripeCustomer ensures the session-authed provider has an active subscription.
// Must run AFTER RequireProviderSession.
func RequireStripeCustomer(bc BillingChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pid, _ := r.Context().Value(CtxKeyProviderID).(string)
			if pid == "" {
				httpx.RenderError(w, r, httpx.ErrUnauthorized)
				return
			}
			ok, err := bc.HasActiveSubscription(r.Context(), pid)
			if err != nil {
				httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
				return
			}
			if !ok {
				httpx.RenderError(w, r, &httpx.APIError{
					Status: http.StatusPaymentRequired, Code: "payment_required", Message: "active subscription required",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ProviderIDFrom returns the provider ID stored on ctx, or "".
func ProviderIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(CtxKeyProviderID).(string); ok {
		return v
	}
	return ""
}

// UserIDFrom returns the user ID stored on ctx, or "".
func UserIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(CtxKeyUserID).(string); ok {
		return v
	}
	return ""
}

// MagicClaimFrom returns the magic-link claim stored on ctx, or nil.
func MagicClaimFrom(ctx context.Context) *magiclink.Claim {
	if v, ok := ctx.Value(CtxKeyMagicClaim).(*magiclink.Claim); ok {
		return v
	}
	return nil
}

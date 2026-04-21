package middleware

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
)

// Role constants for the users.role column. The CHECK constraint in migration
// 000001 enforces these two exact values; keep them in sync.
const (
	RoleProviderAdmin = "provider_admin"
	RoleProviderStaff = "provider_staff"
)

// CtxKeyUserRole is the context key under which the authenticated user's role
// is cached after the first RequireRole lookup, so subsequent middleware /
// handlers can read the role without re-querying.
const CtxKeyUserRole ctxKey = "user_role"

// RoleLookuper loads a user's role from the store. Kept as an interface so
// tests can inject a fake; the wired implementation just queries the users
// table.
type RoleLookuper interface {
	LookupUserRole(ctx context.Context, userID string) (string, error)
}

// PoolRoleLookup is the standard RoleLookuper backed by a *sql.DB over the
// users table.
type PoolRoleLookup struct{ DB *sql.DB }

// LookupUserRole returns the users.role for the given id. Returns an empty
// string if the user does not exist.
func (p PoolRoleLookup) LookupUserRole(ctx context.Context, userID string) (string, error) {
	if userID == "" {
		return "", nil
	}
	var role string
	err := p.DB.QueryRowContext(ctx,
		`SELECT role FROM users WHERE id = ? AND deleted_at IS NULL`, userID).Scan(&role)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return role, nil
}

// RequireRole enforces a specific users.role on the authenticated session.
// Must run AFTER RequireProviderSession. Returns 403 if the role doesn't match.
// The resolved role is cached on ctx under CtxKeyUserRole so dependent
// middleware/handlers can read it without an additional DB round-trip.
func RequireRole(lookup RoleLookuper, required string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// If a prior middleware already cached the role, reuse it.
			role, _ := r.Context().Value(CtxKeyUserRole).(string)
			if role == "" {
				uid, _ := r.Context().Value(CtxKeyUserID).(string)
				if uid == "" {
					// No authenticated user on ctx — middleware mis-wired or
					// session middleware didn't run. Treat as unauthorized.
					httpx.RenderError(w, r, httpx.ErrUnauthorized)
					return
				}
				resolved, err := lookup.LookupUserRole(r.Context(), uid)
				if err != nil {
					httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
					return
				}
				if resolved == "" {
					httpx.RenderError(w, r, httpx.ErrForbidden)
					return
				}
				role = resolved
			}
			if role != required {
				httpx.RenderError(w, r, httpx.ErrForbidden)
				return
			}
			ctx := context.WithValue(r.Context(), CtxKeyUserRole, role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserRoleFrom returns the role stored on ctx, or "" if none.
func UserRoleFrom(ctx context.Context) string {
	if v, ok := ctx.Value(CtxKeyUserRole).(string); ok {
		return v
	}
	return ""
}

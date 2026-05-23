// Package handlers — demo.go
//
// REQ-024 originally seeded a brand-new operator's REAL provider with
// is_demo=1 rows so the post-checkout tour had something to point at.
// REQ-038 split the demo into its own provider row — see workspaces.go +
// demoseed.SeedDemoFacility. These endpoints stay on the same routes for
// back-compat with the frontend tour glue, but their behavior changes:
//
//   GET  /api/demo/status  reports whether the CURRENT active provider is
//                          the demo one (providers.is_demo=1). The old
//                          per-row counts are still emitted for the few
//                          legacy tenants who haven't switched workspaces
//                          yet.
//   POST /api/demo/purge   only acts when the active provider is is_demo=1.
//                          When the user is on their real facility the
//                          endpoint is a no-op — we can never delete real
//                          rows from this entry point.
package handlers

import (
	"database/sql"
	"log/slog"
	"net/http"

	"github.com/tachyurgy/compliancekit/backend/internal/auditlog"
	"github.com/tachyurgy/compliancekit/backend/internal/demoseed"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
)

type DemoHandler struct {
	Pool *sql.DB
	Log  *slog.Logger
}

// GET /api/demo/status
//
// Returns whether the caller's CURRENT active provider is the demo one.
// The dashboard uses this to decide whether to mount the Shayna tour and
// the yellow demo banner. We keep emitting the per-row counts because
// the tour's "magic-link" step asserts that demo children exist before
// rendering, and a few pre-REQ-038 tenants still have those rows on
// their real provider.
func (h *DemoHandler) Status(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	var isDemo int
	if err := h.Pool.QueryRowContext(r.Context(),
		`SELECT COALESCE(is_demo, 0) FROM providers WHERE id = ?`, pid).Scan(&isDemo); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	var demoStaff, demoChildren, demoRooms int
	if err := h.Pool.QueryRowContext(r.Context(), `
		SELECT
		  (SELECT COUNT(*) FROM staff    WHERE provider_id = ? AND is_demo = 1 AND deleted_at IS NULL),
		  (SELECT COUNT(*) FROM children WHERE provider_id = ? AND is_demo = 1 AND deleted_at IS NULL),
		  (SELECT COUNT(*) FROM rooms    WHERE provider_id = ? AND is_demo = 1 AND deleted_at IS NULL)
	`, pid, pid, pid).Scan(&demoStaff, &demoChildren, &demoRooms); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"is_demo_workspace": isDemo == 1,
		"has_demo_data":     isDemo == 1 || demoStaff+demoChildren+demoRooms > 0,
		"demo_staff":        demoStaff,
		"demo_children":     demoChildren,
		"demo_rooms":        demoRooms,
	})
}

// POST /api/demo/purge
//
// Deletes every is_demo=1 row inside the CURRENTLY ACTIVE provider. If
// the active provider is the real one we no-op — we can never let this
// route touch real data from a misclick. Idempotent.
func (h *DemoHandler) Purge(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	var isDemo int
	if err := h.Pool.QueryRowContext(r.Context(),
		`SELECT COALESCE(is_demo, 0) FROM providers WHERE id = ?`, pid).Scan(&isDemo); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	if isDemo != 1 {
		// REQ-038 safety net: never delete from the real provider via
		// this endpoint. Return 200 so the old frontend doesn't blow up
		// on a 4xx.
		httpx.RenderJSON(w, http.StatusOK, map[string]any{
			"status":             "ok",
			"is_demo_workspace":  false,
			"has_demo_data":      false,
			"skipped":            "not_demo_workspace",
		})
		return
	}
	if err := demoseed.Purge(r.Context(), h.Pool, pid); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	auditlog.Emit(r.Context(), h.Pool, auditlog.Entry{
		ProviderID: pid,
		ActorKind:  auditlog.ActorKindProviderAdmin,
		ActorID:    mw.UserIDFrom(r.Context()),
		Action:     "demo.purge",
		TargetKind: auditlog.TargetKindProvider,
		TargetID:   pid,
		IP:         auditlog.ClientIP(r),
		UserAgent:  r.UserAgent(),
	})
	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"status":             "ok",
		"is_demo_workspace":  true,
		"has_demo_data":      false,
	})
}

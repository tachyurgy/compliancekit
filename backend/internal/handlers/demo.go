// Package handlers — demo.go
//
// REQ-024 (Shayna tour). Endpoints that back the post-checkout onboarding
// tour: "/api/demo/status" tells the dashboard whether the current facility
// is showing seeded data so the tour can resume mid-stream; "/api/demo/purge"
// is what the "Make this mine" button at the end of the tour calls to wipe
// the seed and let the operator start with their own data.
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
// Returns whether this facility currently has any is_demo=1 rows. The
// dashboard uses this to decide whether to show the Shayna tour (the
// magic-link "send" beat assumes a demo child exists). Cheap — three
// boolean existence probes, all hitting the partial indexes added in
// migration 000038.
func (h *DemoHandler) Status(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
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
		"has_demo_data":  demoStaff+demoChildren+demoRooms > 0,
		"demo_staff":     demoStaff,
		"demo_children":  demoChildren,
		"demo_rooms":     demoRooms,
	})
}

// POST /api/demo/purge
//
// Deletes every is_demo=1 row for this provider — staff, children,
// documents, rooms. Used by the "Make this mine" button at the end of
// the Shayna tour. Returns 200 with the next state so the frontend can
// invalidate its caches and re-fetch with one round-trip.
//
// Idempotent: hitting it twice on an empty facility is a no-op.
func (h *DemoHandler) Purge(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
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
		"status":        "ok",
		"has_demo_data": false,
	})
}

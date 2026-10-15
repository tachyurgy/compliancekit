// Package handlers — workspaces.go
//
// REQ-038 (demo as second facility): users can belong to more than one
// provider (today: their real facility + a seeded demo one). The session
// row carries which one is currently active. These endpoints back the
// top-left workspace switcher in the React app.
//
//	GET  /api/me/workspaces        list every provider the user can open
//	POST /api/me/active-workspace  flip the active provider on the session
package handlers

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"

	"github.com/tachyurgy/compliancekit/backend/internal/auditlog"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
)

// WorkspaceHandler is intentionally tiny — it owns the read + write paths
// for the active workspace and nothing else.
type WorkspaceHandler struct {
	Pool *sql.DB
	Log  *slog.Logger
}

// Workspace is the JSON shape returned to the frontend.
type Workspace struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	IsDemo   bool   `json:"isDemo"`
	IsActive bool   `json:"isActive"`
}

// listWorkspacesFor is the shared read path: returns every provider the
// user belongs to, marking which one is currently active on the session.
// Used by both /api/me (so the frontend has it on first paint) and the
// dedicated /api/me/workspaces endpoint (for refresh after switching).
func listWorkspacesFor(ctx context.Context, pool *sql.DB, userID, sessionID string) ([]Workspace, string, error) {
	// Active provider lives on the session row. NULL means "fall back to
	// sessions.provider_id" — which is set at signin time and is the user's
	// real provider.
	var active sql.NullString
	var defaultPID string
	if err := pool.QueryRowContext(ctx, `
		SELECT COALESCE(active_provider_id, ''), provider_id
		  FROM sessions
		 WHERE id = ?`, sessionID).Scan(&active, &defaultPID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, "", err
	}
	activeID := active.String
	if activeID == "" {
		activeID = defaultPID
	}

	rows, err := pool.QueryContext(ctx, `
		SELECT p.id,
		       COALESCE(NULLIF(p.name, ''), p.legal_name, '(unnamed facility)') AS name,
		       COALESCE(p.is_demo, 0)
		  FROM provider_memberships pm
		  JOIN providers p ON p.id = pm.provider_id
		 WHERE pm.user_id = ? AND p.deleted_at IS NULL
		 ORDER BY p.is_demo ASC, p.created_at ASC`, userID)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	out := []Workspace{}
	for rows.Next() {
		var w Workspace
		var demoInt int
		if err := rows.Scan(&w.ID, &w.Name, &demoInt); err != nil {
			return nil, "", err
		}
		w.IsDemo = demoInt == 1
		w.IsActive = w.ID == activeID
		out = append(out, w)
	}
	return out, activeID, nil
}

// List handles GET /api/me/workspaces.
func (h *WorkspaceHandler) List(w http.ResponseWriter, r *http.Request) {
	uid := mw.UserIDFrom(r.Context())
	if uid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	sessID := ""
	if c, err := r.Cookie("ck_sess"); err == nil {
		sessID = c.Value
	}
	workspaces, active, err := listWorkspacesFor(r.Context(), h.Pool, uid, sessID)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"workspaces":         workspaces,
		"active_provider_id": active,
	})
}

// SetActive handles POST /api/me/active-workspace { providerId }.
//
// Verifies the user actually has a membership row on the requested
// provider before flipping the session — without this we'd let any
// signed-in user yank themselves into someone else's facility just by
// guessing an ID.
func (h *WorkspaceHandler) SetActive(w http.ResponseWriter, r *http.Request) {
	uid := mw.UserIDFrom(r.Context())
	if uid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	var in struct {
		ProviderID string `json:"providerId"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	if in.ProviderID == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("providerId required"))
		return
	}

	// Verify membership. A row in provider_memberships is the green light.
	var n int
	if err := h.Pool.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM provider_memberships pm
		   JOIN providers p ON p.id = pm.provider_id
		  WHERE pm.user_id = ? AND pm.provider_id = ? AND p.deleted_at IS NULL`,
		uid, in.ProviderID).Scan(&n); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	if n == 0 {
		httpx.RenderError(w, r, httpx.ErrForbidden)
		return
	}

	sessID := ""
	if c, err := r.Cookie("ck_sess"); err == nil {
		sessID = c.Value
	}
	if sessID == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	if _, err := h.Pool.ExecContext(r.Context(),
		`UPDATE sessions SET active_provider_id = ? WHERE id = ? AND revoked_at IS NULL`,
		in.ProviderID, sessID); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	auditlog.Emit(r.Context(), h.Pool, auditlog.Entry{
		ProviderID: in.ProviderID,
		ActorKind:  auditlog.ActorKindProviderAdmin,
		ActorID:    uid,
		Action:     "workspace.switch",
		TargetKind: auditlog.TargetKindProvider,
		TargetID:   in.ProviderID,
		IP:         auditlog.ClientIP(r),
		UserAgent:  r.UserAgent(),
	})

	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"active_provider_id": in.ProviderID,
	})
}

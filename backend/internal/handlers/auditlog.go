package handlers

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
)

// AuditLogHandler exposes GET /api/audit-log to admins. Results are scoped to
// the caller's provider_id. Role enforcement lives in the router via
// middleware.RequireRole(RoleProviderAdmin).
type AuditLogHandler struct {
	Pool *sql.DB
	Log  *slog.Logger
}

// auditLogItem is the JSON shape returned per row. metadata is decoded into a
// JSON object so the frontend can render it without a second json.parse step.
type auditLogItem struct {
	ID         string         `json:"id"`
	ProviderID string         `json:"provider_id"`
	ActorKind  string         `json:"actor_kind"`
	ActorID    string         `json:"actor_id,omitempty"`
	ActorEmail string         `json:"actor_email,omitempty"`
	Action     string         `json:"action"`
	TargetKind string         `json:"target_kind,omitempty"`
	TargetID   string         `json:"target_id,omitempty"`
	Metadata   map[string]any `json:"metadata"`
	IP         string         `json:"ip,omitempty"`
	UserAgent  string         `json:"user_agent,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

type auditLogResponse struct {
	Items      []auditLogItem `json:"items"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

// GET /api/audit-log?limit=&offset=&action=&target_kind=&since=&until=
func (h *AuditLogHandler) List(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}

	q := r.URL.Query()
	limit := parseIntDefault(q.Get("limit"), 50)
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	offset := parseIntDefault(q.Get("offset"), 0)
	if offset < 0 {
		offset = 0
	}

	where := "al.provider_id = ?"
	args := []any{pid}

	if action := q.Get("action"); action != "" {
		where += " AND al.action = ?"
		args = append(args, action)
	}
	if tk := q.Get("target_kind"); tk != "" {
		where += " AND al.target_kind = ?"
		args = append(args, tk)
	}
	if since := q.Get("since"); since != "" {
		t, err := parseTimeFlexible(since)
		if err != nil {
			httpx.RenderError(w, r, httpx.BadRequestf("invalid 'since' timestamp"))
			return
		}
		where += " AND al.created_at >= ?"
		args = append(args, t.UTC().Format(time.RFC3339))
	}
	if until := q.Get("until"); until != "" {
		t, err := parseTimeFlexible(until)
		if err != nil {
			httpx.RenderError(w, r, httpx.BadRequestf("invalid 'until' timestamp"))
			return
		}
		where += " AND al.created_at <= ?"
		args = append(args, t.UTC().Format(time.RFC3339))
	}

	// LEFT JOIN users so we can show actor_email when the actor is a user.
	// Audit rows with system/webhook actors won't match; those columns come
	// back NULL and we surface actor_kind in the UI instead.
	args = append(args, limit, offset)
	sqlText := `
		SELECT al.id, COALESCE(al.provider_id, ''), al.actor_kind, COALESCE(al.actor_id, ''),
		       COALESCE(u.email, ''), al.action,
		       COALESCE(al.target_kind, ''), COALESCE(al.target_id, ''),
		       al.metadata, COALESCE(al.ip, ''), COALESCE(al.user_agent, ''), al.created_at
		FROM audit_log al
		LEFT JOIN users u ON u.id = al.actor_id
		WHERE ` + where + `
		ORDER BY al.created_at DESC
		LIMIT ? OFFSET ?`

	rows, err := h.Pool.QueryContext(r.Context(), sqlText, args...)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	defer rows.Close()

	items := make([]auditLogItem, 0, limit)
	for rows.Next() {
		var (
			it       auditLogItem
			metaText string
			created  string
		)
		if err := rows.Scan(&it.ID, &it.ProviderID, &it.ActorKind, &it.ActorID,
			&it.ActorEmail, &it.Action, &it.TargetKind, &it.TargetID,
			&metaText, &it.IP, &it.UserAgent, &created); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		// Decode the metadata JSON blob; default to {} on any parse failure so
		// a single corrupt row doesn't break the viewer.
		it.Metadata = map[string]any{}
		if metaText != "" {
			if err := json.Unmarshal([]byte(metaText), &it.Metadata); err != nil {
				if h.Log != nil {
					h.Log.WarnContext(r.Context(), "audit-log: metadata unparseable",
						"id", it.ID, "err", err)
				}
				it.Metadata = map[string]any{"_raw": metaText}
			}
		}
		// SQLite stores created_at as ISO-8601 text; parse into a real
		// time.Time for JSON encoding. Accept either RFC3339 or the common
		// "2006-01-02 15:04:05" form that CURRENT_TIMESTAMP yields.
		if t, err := parseTimeFlexible(created); err == nil {
			it.CreatedAt = t.UTC()
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	resp := auditLogResponse{Items: items}
	// Offset-based "next_cursor": only set when we filled the page, implying
	// there may be more. Frontend uses this to enable "Load more".
	if len(items) == limit {
		resp.NextCursor = strconv.Itoa(offset + limit)
	}
	httpx.RenderJSON(w, http.StatusOK, resp)
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// parseTimeFlexible accepts RFC3339, "2006-01-02 15:04:05" (SQLite default
// CURRENT_TIMESTAMP format), or a plain date "2006-01-02". Returns UTC.
func parseTimeFlexible(s string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	var lastErr error
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		} else {
			lastErr = err
		}
	}
	return time.Time{}, lastErr
}

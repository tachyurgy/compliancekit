package handlers

import (
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/tachyurgy/compliancekit/backend/internal/auditlog"
	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/dbtime"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
)

// DrillHandler owns CRUD for the drill_logs table. See migration 000009 for
// the schema and 000010 for the soft-delete column.
type DrillHandler struct {
	Pool *sql.DB
	Log  *slog.Logger
}

// DrillLog is the JSON representation of a drill_logs row.
type DrillLog struct {
	ID                   string    `json:"id"`
	ProviderID           string    `json:"provider_id"`
	DrillKind            string    `json:"drill_kind"`
	DrillDate            time.Time `json:"drill_date"`
	DurationSeconds      int       `json:"duration_seconds,omitempty"`
	Notes                string    `json:"notes,omitempty"`
	AttachmentDocumentID string    `json:"attachment_document_id,omitempty"`
	LoggedByUserID       string    `json:"logged_by_user_id,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

var validDrillKinds = map[string]struct{}{
	"fire": {}, "tornado": {}, "lockdown": {}, "earthquake": {}, "evacuation": {}, "other": {},
}

// POST /api/drills
func (h *DrillHandler) Create(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	uid := mw.UserIDFrom(r.Context())

	var in struct {
		DrillKind            string    `json:"drill_kind"`
		DrillDate            time.Time `json:"drill_date"`
		DurationSeconds      int       `json:"duration_seconds"`
		Notes                string    `json:"notes"`
		AttachmentDocumentID string    `json:"attachment_document_id"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	if _, ok := validDrillKinds[in.DrillKind]; !ok {
		httpx.RenderError(w, r, httpx.BadRequestf("drill_kind must be one of fire|tornado|lockdown|earthquake|evacuation|other"))
		return
	}
	if in.DrillDate.IsZero() {
		in.DrillDate = time.Now().UTC()
	}

	out := DrillLog{
		ID:                   base62.NewID()[:22],
		ProviderID:           pid,
		DrillKind:            in.DrillKind,
		DrillDate:            in.DrillDate,
		DurationSeconds:      in.DurationSeconds,
		Notes:                in.Notes,
		AttachmentDocumentID: in.AttachmentDocumentID,
		LoggedByUserID:       uid,
	}

	// NULL-out empty optional FKs so we don't violate REFERENCES.
	var attachmentArg any
	if in.AttachmentDocumentID != "" {
		attachmentArg = in.AttachmentDocumentID
	}
	var userArg any
	if uid != "" {
		userArg = uid
	}
	var durArg any
	if in.DurationSeconds > 0 {
		durArg = in.DurationSeconds
	}
	var notesArg any
	if in.Notes != "" {
		notesArg = in.Notes
	}

	if _, err := h.Pool.ExecContext(r.Context(), `
		INSERT INTO drill_logs (id, provider_id, drill_kind, drill_date, logged_by_user_id,
		                       duration_seconds, notes, attachment_document_id, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		out.ID, out.ProviderID, out.DrillKind, out.DrillDate, userArg,
		durArg, notesArg, attachmentArg); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	// Hydrate created_at/updated_at from the DB so the client sees canonical values.
	_ = h.Pool.QueryRowContext(r.Context(),
		`SELECT created_at, updated_at FROM drill_logs WHERE id = ?`, out.ID).
		Scan(dbtime.Into(&out.CreatedAt), dbtime.Into(&out.UpdatedAt))

	auditlog.EmitDrillCreate(r.Context(), h.Pool, pid, uid, out.ID, map[string]any{
		"drill_kind": out.DrillKind,
		"drill_date": out.DrillDate,
	}, r)

	httpx.RenderJSON(w, http.StatusCreated, out)
}

// GET /api/drills?kind=&from=&to=
func (h *DrillHandler) List(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	q := r.URL.Query()

	args := []any{pid}
	sqlText := `SELECT id, provider_id, drill_kind, drill_date, COALESCE(logged_by_user_id,''),
		       COALESCE(duration_seconds,0), COALESCE(notes,''), COALESCE(attachment_document_id,''),
		       created_at, updated_at
		FROM drill_logs WHERE provider_id = ? AND deleted_at IS NULL`

	if kind := q.Get("kind"); kind != "" {
		if _, ok := validDrillKinds[kind]; !ok {
			httpx.RenderError(w, r, httpx.BadRequestf("invalid kind"))
			return
		}
		sqlText += ` AND drill_kind = ?`
		args = append(args, kind)
	}
	if from := q.Get("from"); from != "" {
		t, err := time.Parse(time.RFC3339, from)
		if err != nil {
			t, err = time.Parse("2006-01-02", from)
		}
		if err != nil {
			httpx.RenderError(w, r, httpx.BadRequestf("invalid from date"))
			return
		}
		sqlText += ` AND drill_date >= ?`
		args = append(args, t)
	}
	if to := q.Get("to"); to != "" {
		t, err := time.Parse(time.RFC3339, to)
		if err != nil {
			t, err = time.Parse("2006-01-02", to)
		}
		if err != nil {
			httpx.RenderError(w, r, httpx.BadRequestf("invalid to date"))
			return
		}
		sqlText += ` AND drill_date <= ?`
		args = append(args, t)
	}
	sqlText += ` ORDER BY drill_date DESC, created_at DESC LIMIT 500`

	rows, err := h.Pool.QueryContext(r.Context(), sqlText, args...)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	defer rows.Close()

	out := make([]DrillLog, 0)
	for rows.Next() {
		var d DrillLog
		// SQLite's modernc.org/sqlite driver returns TEXT DATETIME columns as Go
		// strings — database/sql cannot auto-convert into *time.Time. Scan the
		// three timestamp columns into intermediate strings and parse them.
		var drillDateRaw, createdAtRaw, updatedAtRaw string
		if err := rows.Scan(&d.ID, &d.ProviderID, &d.DrillKind, &drillDateRaw, &d.LoggedByUserID,
			&d.DurationSeconds, &d.Notes, &d.AttachmentDocumentID,
			&createdAtRaw, &updatedAtRaw); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		d.DrillDate = parseSQLiteTime(drillDateRaw)
		d.CreatedAt = parseSQLiteTime(createdAtRaw)
		d.UpdatedAt = parseSQLiteTime(updatedAtRaw)
		out = append(out, d)
	}
	httpx.RenderJSON(w, http.StatusOK, out)
}

// DELETE /api/drills/{id} — soft delete.
func (h *DrillHandler) Delete(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	res, err := h.Pool.ExecContext(r.Context(),
		`UPDATE drill_logs SET deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND provider_id = ? AND deleted_at IS NULL`, id, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		httpx.RenderError(w, r, httpx.ErrNotFound)
		return
	}
	auditlog.EmitDrillDelete(r.Context(), h.Pool, pid, mw.UserIDFrom(r.Context()), id, r)
	w.WriteHeader(http.StatusNoContent)
}

// parseSQLiteTime parses a TEXT timestamp from SQLite into a time.Time.
// SQLite emits two formats depending on how the value was inserted:
//   - "2006-01-02 15:04:05"           (CURRENT_TIMESTAMP default)
//   - "2006-01-02T15:04:05Z"          (Go time.Time encoded via driver)
//
// Falls back to zero time on parse failure rather than erroring the whole
// request — the UI can tolerate a blank timestamp; 500 on a list endpoint
// cannot.
func parseSQLiteTime(s string) time.Time {
	// Delegate to dbtime so every TEXT timestamp format the app writes
	// (CURRENT_TIMESTAMP, RFC 3339, and the driver's time.Time.String()
	// encoding) parses the same way everywhere.
	return dbtime.MustParse(s)
}

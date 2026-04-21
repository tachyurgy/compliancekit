package handlers

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/dataexport"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
	"github.com/tachyurgy/compliancekit/backend/internal/notify"
	"github.com/tachyurgy/compliancekit/backend/internal/storage"
)

// DataExportHandler implements POST /api/exports, GET /api/exports, and
// GET /api/exports/{id}/download. Exports are produced asynchronously by a
// background goroutine (no job queue at MVP); the data_exports table is the
// durable status record so a reload of the Settings page shows the right
// state even across server restarts.
type DataExportHandler struct {
	Pool    *sql.DB
	Storage *storage.Client
	Emailer *notify.Emailer
	// DownloadTTL is how long each presigned GET URL we mint is valid.
	// Defaults to 15 minutes when zero.
	DownloadTTL time.Duration
	Log         *slog.Logger
}

// POST /api/exports — enqueue a new export.
//
// Returns 202 with { export_id, status } immediately. The ZIP is assembled in
// the background; the user receives an email with a download link when the
// export completes (or a failure notification otherwise). The Settings UI can
// also poll GET /api/exports to watch progress.
func (h *DataExportHandler) Create(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	uid := mw.UserIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}

	// Insert the tracking row synchronously so the UI can immediately list it.
	exportID := base62.NewID()[:22]
	var userIDArg any
	if uid != "" {
		userIDArg = uid
	}
	if _, err := h.Pool.ExecContext(r.Context(), `
INSERT INTO data_exports (id, provider_id, requested_by_user_id, status, started_at)
VALUES (?, ?, ?, 'requested', CURRENT_TIMESTAMP)`, exportID, pid, userIDArg); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	// Capture the owner email for the completion notification BEFORE we
	// background the goroutine, so we don't race a purge of the providers row.
	ownerEmail := h.lookupOwnerEmail(r.Context(), pid)

	// Decouple from the request ctx — we don't want a client disconnect to
	// cancel a multi-minute export. Use a background ctx with a generous
	// upper bound so a runaway export doesn't hang forever either.
	go h.runExport(exportID, pid, ownerEmail)

	httpx.RenderJSON(w, http.StatusAccepted, map[string]any{
		"export_id": exportID,
		"status":    "requested",
		"message":   "We'll email you when the export is ready.",
	})
}

// runExport is the background job. It updates the data_exports row at each
// state transition so the UI can render accurate progress.
func (h *DataExportHandler) runExport(exportID, providerID, ownerEmail string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	_, _ = h.Pool.ExecContext(ctx, `UPDATE data_exports SET status = 'running' WHERE id = ?`, exportID)
	log := h.Log
	if log == nil {
		log = slog.Default()
	}
	log.Info("data export: starting", "export_id", exportID, "provider_id", providerID)

	key, err := dataexport.ExportProvider(ctx, h.Pool, h.Storage, providerID)
	if err != nil {
		log.Error("data export: failed", "export_id", exportID, "err", err)
		_, _ = h.Pool.ExecContext(ctx, `
UPDATE data_exports SET status = 'failed', error_text = ?, finished_at = CURRENT_TIMESTAMP
 WHERE id = ?`, err.Error(), exportID)
		return
	}

	_, _ = h.Pool.ExecContext(ctx, `
UPDATE data_exports SET status = 'completed', s3_key = ?, finished_at = CURRENT_TIMESTAMP
 WHERE id = ?`, key, exportID)
	log.Info("data export: completed", "export_id", exportID, "s3_key", key)

	// Email the requester a fresh short-lived link (the UI can always mint
	// another via GET /api/exports/{id}/download).
	if h.Emailer != nil && h.Storage != nil && ownerEmail != "" {
		url, perr := h.presign(ctx, key)
		if perr != nil {
			log.Warn("data export: presign for email failed", "err", perr)
			return
		}
		subject := "Your ComplianceKit data export is ready"
		html := `<p>Your data export is ready. The download link below is valid for 15 minutes.</p>` +
			`<p><a href="` + url + `">Download your export (ZIP)</a></p>`
		text := "Your ComplianceKit data export is ready. Download (valid 15 minutes): " + url
		if err := h.Emailer.Send(ctx, notify.EmailMessage{
			To: ownerEmail, Subject: subject, HTMLBody: html, PlainBody: text,
			ReferenceID: exportID,
		}); err != nil {
			log.Warn("data export: email send failed", "err", err)
		}
	}
}

// GET /api/exports — list this provider's export history (newest first).
func (h *DataExportHandler) List(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	rows, err := h.Pool.QueryContext(r.Context(), `
SELECT id, provider_id, COALESCE(requested_by_user_id,''), status,
       COALESCE(s3_key,''), COALESCE(error_text,''),
       started_at, COALESCE(finished_at,'')
  FROM data_exports WHERE provider_id = ? ORDER BY started_at DESC`, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	defer rows.Close()
	type row struct {
		ID            string `json:"id"`
		ProviderID    string `json:"provider_id"`
		RequestedByID string `json:"requested_by_user_id,omitempty"`
		Status        string `json:"status"`
		S3Key         string `json:"s3_key,omitempty"`
		ErrorText     string `json:"error_text,omitempty"`
		StartedAt     string `json:"started_at"`
		FinishedAt    string `json:"finished_at,omitempty"`
	}
	out := make([]row, 0)
	for rows.Next() {
		var x row
		if err := rows.Scan(&x.ID, &x.ProviderID, &x.RequestedByID, &x.Status,
			&x.S3Key, &x.ErrorText, &x.StartedAt, &x.FinishedAt); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		out = append(out, x)
	}
	httpx.RenderJSON(w, http.StatusOK, out)
}

// GET /api/exports/{id}/download — mint a fresh presigned URL for a completed
// export. We re-presign on every call (rather than returning a long-lived URL
// in the email) so a leaked link expires quickly.
func (h *DataExportHandler) Download(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	if pid == "" || id == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	var status, key string
	err := h.Pool.QueryRowContext(r.Context(),
		`SELECT status, COALESCE(s3_key,'') FROM data_exports WHERE id = ? AND provider_id = ?`,
		id, pid).Scan(&status, &key)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.RenderError(w, r, httpx.ErrNotFound)
			return
		}
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	if status != "completed" || key == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("export not ready (status=%s)", status))
		return
	}
	if h.Storage == nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, errors.New("storage not configured")))
		return
	}
	url, err := h.presign(r.Context(), key)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	httpx.RenderJSON(w, http.StatusOK, map[string]string{"url": url})
}

func (h *DataExportHandler) presign(ctx context.Context, key string) (string, error) {
	ttl := h.DownloadTTL
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	bucket := h.Storage.Buckets().AuditTrail
	return h.Storage.PresignGetURL(ctx, bucket, key, ttl)
}

func (h *DataExportHandler) lookupOwnerEmail(ctx context.Context, providerID string) string {
	var email string
	_ = h.Pool.QueryRowContext(ctx,
		`SELECT COALESCE(owner_email,'') FROM providers WHERE id = ?`, providerID).Scan(&email)
	return email
}

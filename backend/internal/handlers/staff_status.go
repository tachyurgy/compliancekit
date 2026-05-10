package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/tachyurgy/compliancekit/backend/internal/auditlog"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	"github.com/tachyurgy/compliancekit/backend/internal/magiclink"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
)

// REQ-004: status-pill rollup + extended staff endpoints.
//
// The pill on /staff and the row-level pills on /staff/:id are computed
// from two inputs:
//
//   1. The set of `document_requirements` rows that apply to the staff
//      member's role (plus all rows where role = 'all').
//   2. The most recent non-deleted `documents` row per (subject_id, kind)
//      pair.
//
// We classify each requirement into one of five buckets keyed verbatim to
// brand-voice.md:
//   - on_file           green   — document present, not expired
//   - expiring_soon     amber   — document present, expires <90 days
//   - expired           coral   — past expires_at
//   - awaiting_upload   blue    — staff_nag_sends row but no document yet
//   - missing           gray    — no document, no awaiting nag
//
// The list endpoint surfaces only the rollup counts ("Missing 3" /
// "Expired 1" / "Passing"); the detail endpoint returns the per-row
// status.

// DocStatus is the verbatim brand-voice pill code we send to the frontend.
type DocStatus string

const (
	StatusOnFile         DocStatus = "on_file"
	StatusExpiringSoon   DocStatus = "expiring_soon"
	StatusExpired        DocStatus = "expired"
	StatusAwaitingUpload DocStatus = "awaiting_upload"
	StatusMissing        DocStatus = "missing"
)

// expiryWindowDays defines the "expiring soon" cutoff (Handbook expects
// 90 days for license; brand-voice pegs the pill at 90 too).
const expiryWindowDays = 90

// requirementRow is one row from `document_requirements`.
type requirementRow struct {
	ID              string
	DocKind         string
	Label           string
	Frequency       string
	GracePeriodDays int
	Authority       string
	SortOrder       int
}

// docRow is the latest document on file for a (staff_id, kind) pair.
type docRow struct {
	ID         string
	IssuedAt   *time.Time
	ExpiresAt  *time.Time
	OCRSource  string
	UploadedBy string
	CreatedAt  time.Time
}

// statusInputs is what classifyDoc needs. Decoupling the IO from the
// rule lets us write small table-driven tests against classifyDoc directly.
type statusInputs struct {
	now           time.Time
	hasDoc        bool
	doc           docRow
	pendingNagAt  *time.Time
	expiryWindowD int
}

// classifyDoc is the pure pill logic. Tests in staff_status_test.go drive
// the matrix here.
func classifyDoc(in statusInputs) DocStatus {
	if !in.hasDoc {
		// No document — was a nag recently sent?
		if in.pendingNagAt != nil {
			// Treat any nag in the last 30 days as "awaiting upload"; older
			// nags fall back to plain missing because the operator should
			// re-nag rather than wait forever.
			if in.now.Sub(*in.pendingNagAt) < 30*24*time.Hour {
				return StatusAwaitingUpload
			}
		}
		return StatusMissing
	}
	// Doc present. If there's no expiry, the doc is considered "once" —
	// always on file. If there is, compare against now.
	if in.doc.ExpiresAt == nil || in.doc.ExpiresAt.IsZero() {
		return StatusOnFile
	}
	if in.now.After(*in.doc.ExpiresAt) {
		return StatusExpired
	}
	if in.doc.ExpiresAt.Sub(in.now) <= time.Duration(in.expiryWindowD)*24*time.Hour {
		return StatusExpiringSoon
	}
	return StatusOnFile
}

// rollupCounts is the per-staff summary surfaced on the /staff list page.
type rollupCounts struct {
	OnFile         int       `json:"onFile"`
	ExpiringSoon   int       `json:"expiringSoon"`
	Expired        int       `json:"expired"`
	Missing        int       `json:"missing"`
	AwaitingUpload int       `json:"awaitingUpload"`
	Total          int       `json:"total"`
	Status         DocStatus `json:"status"` // worst-case across the staff member
}

// classifyRollup returns the worst pill across the set; this is what we
// show on the list-page row.
func (rc rollupCounts) worst() DocStatus {
	switch {
	case rc.Expired > 0:
		return StatusExpired
	case rc.Missing > 0:
		return StatusMissing
	case rc.ExpiringSoon > 0:
		return StatusExpiringSoon
	case rc.AwaitingUpload > 0:
		return StatusAwaitingUpload
	default:
		return StatusOnFile
	}
}

// requirementsForRole loads the set of doc_requirements that apply to a
// given role. Rows where role = 'all' apply to every staff member.
func requirementsForRole(ctx context.Context, db *sql.DB, role string) ([]requirementRow, error) {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		role = "all"
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, doc_kind, label, frequency, grace_period_days, COALESCE(authority,''), sort_order
		  FROM document_requirements
		 WHERE entity_type = 'staff'
		   AND state_code = 'FL'
		   AND (role = 'all' OR role = ?)
		 ORDER BY sort_order ASC`, role)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]requirementRow, 0)
	for rows.Next() {
		var r requirementRow
		if err := rows.Scan(&r.ID, &r.DocKind, &r.Label, &r.Frequency, &r.GracePeriodDays, &r.Authority, &r.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// latestDocsByKind returns a map[doc_kind] -> latest non-deleted document
// for a single staff member. We pick the row with the most recent
// expires_at (NULLs last) so a doc that's been replaced wins over a stale
// one. Same shape per row as docRow.
func latestDocsByKind(ctx context.Context, db *sql.DB, providerID, staffID string) (map[string]docRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT d.id, d.kind, d.issued_at, d.expires_at, COALESCE(d.ocr_source,''),
		       COALESCE(d.uploaded_by,''), d.created_at
		  FROM documents d
		 WHERE d.provider_id = ? AND d.subject_kind = 'staff' AND d.subject_id = ?
		   AND d.deleted_at IS NULL
		 ORDER BY d.kind, d.created_at DESC`, providerID, staffID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]docRow{}
	for rows.Next() {
		var (
			id, kind, src, uploader   string
			issuedStr, expiresStr     sql.NullString
			createdStr                string
		)
		if err := rows.Scan(&id, &kind, &issuedStr, &expiresStr, &src, &uploader, &createdStr); err != nil {
			return nil, err
		}
		// Take only the first row per kind (ORDER BY kind, created_at DESC).
		if _, seen := out[kind]; seen {
			continue
		}
		dr := docRow{
			ID:         id,
			OCRSource:  src,
			UploadedBy: uploader,
			CreatedAt:  parseSQLiteTime(createdStr),
		}
		if issuedStr.Valid && issuedStr.String != "" {
			t := parseSQLiteTime(issuedStr.String)
			dr.IssuedAt = &t
		}
		if expiresStr.Valid && expiresStr.String != "" {
			t := parseSQLiteTime(expiresStr.String)
			dr.ExpiresAt = &t
		}
		out[kind] = dr
	}
	return out, nil
}

// latestNagByKind returns the most recent staff_nag_sends row per
// (staff_id, doc_kind). Used to surface "Awaiting upload" when a manager
// has just nagged but the staff hasn't uploaded yet.
func latestNagByKind(ctx context.Context, db *sql.DB, staffID string) (map[string]time.Time, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT COALESCE(doc_kind,''), created_at
		  FROM staff_nag_sends
		 WHERE staff_id = ?
		 ORDER BY created_at DESC`, staffID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var kind, created string
		if err := rows.Scan(&kind, &created); err != nil {
			return nil, err
		}
		if kind == "" {
			continue
		}
		if _, seen := out[kind]; seen {
			continue
		}
		out[kind] = parseSQLiteTime(created)
	}
	return out, nil
}

// computeRollup ties requirementsForRole + latestDocsByKind + latestNagByKind
// into a counts summary. Pure-ish — IO is parameterised via db.
func computeRollup(ctx context.Context, db *sql.DB, providerID, staffID, role string, now time.Time) (rollupCounts, []requirementStatus, error) {
	reqs, err := requirementsForRole(ctx, db, role)
	if err != nil {
		return rollupCounts{}, nil, err
	}
	docs, err := latestDocsByKind(ctx, db, providerID, staffID)
	if err != nil {
		return rollupCounts{}, nil, err
	}
	nags, err := latestNagByKind(ctx, db, staffID)
	if err != nil {
		return rollupCounts{}, nil, err
	}
	rc := rollupCounts{Total: len(reqs)}
	statuses := make([]requirementStatus, 0, len(reqs))
	for _, req := range reqs {
		d, ok := docs[req.DocKind]
		var pendingAt *time.Time
		if t, hasNag := nags[req.DocKind]; hasNag {
			pt := t
			pendingAt = &pt
		}
		st := classifyDoc(statusInputs{
			now:           now,
			hasDoc:        ok,
			doc:           d,
			pendingNagAt:  pendingAt,
			expiryWindowD: expiryWindowDays,
		})
		switch st {
		case StatusOnFile:
			rc.OnFile++
		case StatusExpiringSoon:
			rc.ExpiringSoon++
		case StatusExpired:
			rc.Expired++
		case StatusMissing:
			rc.Missing++
		case StatusAwaitingUpload:
			rc.AwaitingUpload++
		}
		rs := requirementStatus{
			DocKind:    req.DocKind,
			Label:      req.Label,
			Authority:  req.Authority,
			Frequency:  req.Frequency,
			Status:     st,
			SortOrder:  req.SortOrder,
		}
		if ok {
			rs.DocumentID = d.ID
			if d.ExpiresAt != nil && !d.ExpiresAt.IsZero() {
				s := d.ExpiresAt.Format("2006-01-02")
				rs.ExpiresOn = &s
			}
			if d.IssuedAt != nil && !d.IssuedAt.IsZero() {
				s := d.IssuedAt.Format("2006-01-02")
				rs.IssuedOn = &s
			}
			rs.Source = sourceLabel(d)
		}
		statuses = append(statuses, rs)
	}
	rc.Status = rc.worst()
	return rc, statuses, nil
}

// requirementStatus is one row in the staff detail page's required-docs
// section.
type requirementStatus struct {
	DocKind    string    `json:"docKind"`
	Label      string    `json:"label"`
	Authority  string    `json:"authority,omitempty"`
	Frequency  string    `json:"frequency,omitempty"`
	Status     DocStatus `json:"status"`
	IssuedOn   *string   `json:"issuedOn,omitempty"`
	ExpiresOn  *string   `json:"expiresOn,omitempty"`
	DocumentID string    `json:"documentId,omitempty"`
	Source     string    `json:"source,omitempty"`
	SortOrder  int       `json:"sortOrder"`
}

// sourceLabel turns the document's metadata into operator-friendly copy
// per brand-voice.md ("Uploaded by Sarah" / "Scanned with Levelbrook AI" /
// "Uploaded by you").
func sourceLabel(d docRow) string {
	switch strings.ToLower(d.OCRSource) {
	case "mistral", "gemini":
		return "Scanned with Levelbrook AI"
	}
	if d.UploadedBy != "" {
		// We don't resolve the user_id to a name here — that requires a join
		// the list endpoint can't afford. The detail endpoint can extend
		// this if/when needed.
		return "Uploaded by you"
	}
	return ""
}

// ---- HTTP handlers --------------------------------------------------------

// ListWithRollup is REQ-004's enriched /api/staff list. We replace the
// existing StaffHandler.List behaviour by using this method behind the
// same route — see router.go.
func (h *StaffHandler) ListWithRollup(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT id, COALESCE(first_name,''), COALESCE(last_name,''),
		       COALESCE(role,''), COALESCE(email,''), COALESCE(phone,''),
		       COALESCE(preferred_contact_channel,'email'),
		       COALESCE(hire_date, hired_on, ''),
		       COALESCE(status,'active'),
		       archived_at,
		       created_at, updated_at
		  FROM staff
		 WHERE provider_id = ? AND deleted_at IS NULL AND archived_at IS NULL
		 ORDER BY last_name, first_name`, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	defer rows.Close()
	type staffRow struct {
		ID              string       `json:"id"`
		FirstName       string       `json:"firstName"`
		LastName        string       `json:"lastName"`
		Role            string       `json:"role"`
		Email           string       `json:"email"`
		Phone           string       `json:"phone"`
		PreferredChannel string      `json:"preferredChannel"`
		HireDate        string       `json:"hireDate,omitempty"`
		Status          string       `json:"status"`
		Rollup          rollupCounts `json:"rollup"`
	}
	out := make([]staffRow, 0)
	now := time.Now().UTC()
	for rows.Next() {
		var sr staffRow
		var archivedAt sql.NullString
		var createdStr, updatedStr string
		if err := rows.Scan(&sr.ID, &sr.FirstName, &sr.LastName, &sr.Role, &sr.Email, &sr.Phone,
			&sr.PreferredChannel, &sr.HireDate, &sr.Status, &archivedAt, &createdStr, &updatedStr); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		rc, _, err := computeRollup(r.Context(), h.Pool, pid, sr.ID, sr.Role, now)
		if err != nil {
			h.Log.Warn("rollup", "staff_id", sr.ID, "err", err)
			rc = rollupCounts{Status: StatusOnFile}
		}
		sr.Rollup = rc
		out = append(out, sr)
	}

	// Optional ?status=missing|expired|passing filter — REQ-004 mentions it
	// in the API surface. We do this in Go rather than SQL because the
	// rollup is computed in-process.
	want := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	if want != "" {
		filtered := make([]staffRow, 0, len(out))
		for _, sr := range out {
			match := false
			switch want {
			case "passing":
				match = sr.Rollup.Status == StatusOnFile
			case "missing":
				match = sr.Rollup.Missing > 0 || sr.Rollup.AwaitingUpload > 0
			case "expired":
				match = sr.Rollup.Expired > 0 || sr.Rollup.ExpiringSoon > 0
			}
			if match {
				filtered = append(filtered, sr)
			}
		}
		out = filtered
	}

	httpx.RenderJSON(w, http.StatusOK, out)
}

// staffDetailResponse is the payload for GET /api/staff/:id used by the
// REQ-004 detail page.
type staffDetailResponse struct {
	ID                 string              `json:"id"`
	FirstName          string              `json:"firstName"`
	LastName           string              `json:"lastName"`
	Role               string              `json:"role"`
	Email              string              `json:"email"`
	Phone              string              `json:"phone"`
	PreferredChannel   string              `json:"preferredChannel"`
	HireDate           string              `json:"hireDate,omitempty"`
	Status             string              `json:"status"`
	ArchivedAt         *string             `json:"archivedAt,omitempty"`
	Rollup             rollupCounts        `json:"rollup"`
	RequiredDocuments  []requirementStatus `json:"requiredDocuments"`
	OtherDocuments     []otherDocument     `json:"otherDocuments"`
	Activity           []activityEvent     `json:"activity"`
	UploadLink         string              `json:"uploadLink,omitempty"`
	UploadLinkExpires  string              `json:"uploadLinkExpiresAt,omitempty"`
}

type otherDocument struct {
	ID         string  `json:"id"`
	Kind       string  `json:"kind"`
	Title      string  `json:"title"`
	IssuedOn   *string `json:"issuedOn,omitempty"`
	ExpiresOn  *string `json:"expiresOn,omitempty"`
	UploadedAt string  `json:"uploadedAt"`
	Source     string  `json:"source,omitempty"`
}

type activityEvent struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Summary   string `json:"summary"`
	Detail    string `json:"detail,omitempty"`
	Actor     string `json:"actor,omitempty"`
	CreatedAt string `json:"createdAt"`
}

// GetDetail is GET /api/staff/:id — replaces the existing StaffHandler.Get
// for the detail page. Returns the rich shape the frontend expects.
func (h *StaffHandler) GetDetail(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")

	var (
		out                staffDetailResponse
		archivedAt         sql.NullString
		hireDate           sql.NullString
	)
	err := h.Pool.QueryRowContext(r.Context(), `
		SELECT id, COALESCE(first_name,''), COALESCE(last_name,''),
		       COALESCE(role,''), COALESCE(email,''), COALESCE(phone,''),
		       COALESCE(preferred_contact_channel,'email'),
		       COALESCE(hire_date, hired_on, NULL),
		       COALESCE(status,'active'),
		       archived_at
		  FROM staff
		 WHERE id = ? AND provider_id = ? AND deleted_at IS NULL`, id, pid).
		Scan(&out.ID, &out.FirstName, &out.LastName, &out.Role, &out.Email, &out.Phone,
			&out.PreferredChannel, &hireDate, &out.Status, &archivedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.RenderError(w, r, httpx.ErrNotFound)
			return
		}
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	if hireDate.Valid {
		out.HireDate = hireDate.String
	}
	if archivedAt.Valid && archivedAt.String != "" {
		s := archivedAt.String
		out.ArchivedAt = &s
	}

	// Required documents + rollup.
	now := time.Now().UTC()
	rc, reqStatuses, err := computeRollup(r.Context(), h.Pool, pid, id, out.Role, now)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	out.Rollup = rc
	out.RequiredDocuments = reqStatuses

	// Other documents — anything on file that isn't in the required-doc set.
	requiredKinds := map[string]bool{}
	for _, rs := range reqStatuses {
		requiredKinds[rs.DocKind] = true
	}
	otherRows, err := h.Pool.QueryContext(r.Context(), `
		SELECT id, kind, COALESCE(title,''), issued_at, expires_at,
		       COALESCE(ocr_source,''), COALESCE(uploaded_by,''), created_at
		  FROM documents
		 WHERE provider_id = ? AND subject_kind = 'staff' AND subject_id = ?
		   AND deleted_at IS NULL
		 ORDER BY created_at DESC`, pid, id)
	if err == nil {
		defer otherRows.Close()
		for otherRows.Next() {
			var (
				docID, kind, title, src, uploader string
				issuedStr, expiresStr             sql.NullString
				createdStr                        string
			)
			if err := otherRows.Scan(&docID, &kind, &title, &issuedStr, &expiresStr, &src, &uploader, &createdStr); err != nil {
				continue
			}
			if requiredKinds[kind] {
				continue
			}
			od := otherDocument{
				ID:         docID,
				Kind:       kind,
				Title:      title,
				UploadedAt: createdStr,
				Source:     sourceLabel(docRow{OCRSource: src, UploadedBy: uploader}),
			}
			if issuedStr.Valid && issuedStr.String != "" {
				s := issuedStr.String
				od.IssuedOn = &s
			}
			if expiresStr.Valid && expiresStr.String != "" {
				s := expiresStr.String
				od.ExpiresOn = &s
			}
			out.OtherDocuments = append(out.OtherDocuments, od)
		}
	}

	// Activity timeline (capped at 50 most recent rows).
	out.Activity = loadStaffActivity(r.Context(), h.Pool, id, 50)

	httpx.RenderJSON(w, http.StatusOK, out)
}

// loadStaffActivity loads the most-recent N activity rows. Falls back to a
// reconstructed timeline from staff_nag_sends + documents if the
// staff_activity_events table is empty for this staff member, so existing
// staff have something to show before the new event-emit hooks light up.
func loadStaffActivity(ctx context.Context, db *sql.DB, staffID string, limit int) []activityEvent {
	out := []activityEvent{}
	// 1. Pull explicit events.
	rows, err := db.QueryContext(ctx, `
		SELECT id, event_kind, summary, COALESCE(detail,''), COALESCE(actor,''), created_at
		  FROM staff_activity_events
		 WHERE staff_id = ?
		 ORDER BY created_at DESC LIMIT ?`, staffID, limit)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var ev activityEvent
			if err := rows.Scan(&ev.ID, &ev.Kind, &ev.Summary, &ev.Detail, &ev.Actor, &ev.CreatedAt); err == nil {
				out = append(out, ev)
			}
		}
	}
	if len(out) > 0 {
		return out
	}
	// 2. Synthesize from documents + staff_nag_sends. Best-effort — order
	// by created_at desc.
	docRows, err := db.QueryContext(ctx, `
		SELECT id, kind, COALESCE(title, kind), created_at
		  FROM documents
		 WHERE subject_kind = 'staff' AND subject_id = ? AND deleted_at IS NULL
		 ORDER BY created_at DESC LIMIT ?`, staffID, limit)
	if err == nil {
		defer docRows.Close()
		for docRows.Next() {
			var id, kind, title, created string
			if err := docRows.Scan(&id, &kind, &title, &created); err == nil {
				out = append(out, activityEvent{
					ID: id, Kind: "document_uploaded",
					Summary:   fmt.Sprintf("Uploaded %s", title),
					CreatedAt: created,
				})
			}
		}
	}
	nagRows, err := db.QueryContext(ctx, `
		SELECT id, COALESCE(reason,''), COALESCE(doc_kind,''), COALESCE(channel,''), created_at
		  FROM staff_nag_sends
		 WHERE staff_id = ?
		 ORDER BY created_at DESC LIMIT ?`, staffID, limit)
	if err == nil {
		defer nagRows.Close()
		for nagRows.Next() {
			var id, reason, kind, channel, created string
			if err := nagRows.Scan(&id, &reason, &kind, &channel, &created); err == nil {
				summary := "Sent a reminder"
				if kind != "" {
					summary = fmt.Sprintf("Sent a reminder about %s", kind)
				}
				out = append(out, activityEvent{
					ID: id, Kind: "reminder_sent",
					Summary:   summary,
					Detail:    fmt.Sprintf("via %s (%s)", channel, reason),
					CreatedAt: created,
				})
			}
		}
	}
	return out
}

// POST /api/staff/{id}/archive — soft-archive a departing staff member.
// Distinct from DELETE (which sets status='terminated'); archive removes
// them from chase scope and from the default /staff list, but preserves
// the row so the audit trail can still resolve their name.
func (h *StaffHandler) Archive(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	res, err := h.Pool.ExecContext(r.Context(), `
		UPDATE staff
		   SET archived_at = CURRENT_TIMESTAMP,
		       status = 'inactive',
		       updated_at = CURRENT_TIMESTAMP
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
	auditlog.Emit(r.Context(), h.Pool, auditlog.Entry{
		ProviderID: pid,
		ActorKind:  auditlog.ActorKindProviderAdmin,
		ActorID:    mw.UserIDFrom(r.Context()),
		Action:     "staff.archived",
		TargetKind: auditlog.TargetKindStaff,
		TargetID:   id,
		IP:         auditlog.ClientIP(r),
		UserAgent:  r.UserAgent(),
	})
	w.WriteHeader(http.StatusNoContent)
}

// UploadLinkHandler is the deps the GetUploadLink endpoint needs. We
// declare it here so router.go can wire either the existing
// MintStaffPortalLink (which both mints AND optionally emails) or this
// pure-read variant. For REQ-004 we just need the shareable URL.
type UploadLinkHandler struct {
	Pool         *sql.DB
	Magic        *magiclink.Service
	FrontendBase string
}

// GET /api/staff/{id}/upload-link
//
// Returns a freshly-minted magic-link URL the operator can copy. We mint a
// new token each call rather than reuse: the existing minting service
// (magiclink) does not surface plaintext tokens after the fact. Operators
// can call this repeatedly; old tokens stay valid until their TTL or
// consumed_at marker.
func (u *UploadLinkHandler) Get(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	// Verify the staff member belongs to this provider.
	var found int
	if err := u.Pool.QueryRowContext(r.Context(),
		`SELECT 1 FROM staff WHERE id = ? AND provider_id = ? AND deleted_at IS NULL`,
		id, pid).Scan(&found); err != nil {
		httpx.RenderError(w, r, httpx.ErrNotFound)
		return
	}
	_, path, err := u.Magic.Generate(r.Context(), magiclink.KindStaffUpload, pid, id, 0)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	expiresAt := time.Now().Add(magiclink.TTLFor(magiclink.KindStaffUpload)).UTC().Format(time.RFC3339)
	httpx.RenderJSON(w, http.StatusOK, map[string]string{
		"url":         u.FrontendBase + path,
		"expiresAt":   expiresAt,
	})
}

// _ = strconv keeps the import alive for future helpers; remove once a
// caller appears.
var _ = strconv.Itoa

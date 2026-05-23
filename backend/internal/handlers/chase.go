package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
)

// ChaseHandler powers the operator-facing "Chase Mode" dashboard
// (REQ-042). It reads from the existing chase-related tables and exposes
// three endpoints:
//
//   GET  /api/chase/state    — outstanding chases + stuck items
//   GET  /api/chase/activity — recent nag log (defaults to last 30 days)
//   POST /api/chase/trigger  — fires a fresh nag, reusing NagHandler
//
// No new background workers. No new templates. The handler is a thin
// read layer on top of the existing chase tables plus a trigger that
// dispatches to NagHandler.Trigger / TriggerChild.
type ChaseHandler struct {
	Pool *sql.DB
	Nag  *NagHandler
	Log  *slog.Logger
}

// ChaseRow is one outstanding chase line item (staff cert or child doc).
type ChaseRow struct {
	RecipientKind  string `json:"recipientKind"` // "parent" | "staff"
	RecipientID    string `json:"recipientId"`
	RecipientName  string `json:"recipientName"`
	RecipientRole  string `json:"recipientRole"`
	RecipientPhone string `json:"recipientPhone,omitempty"`
	RecipientEmail string `json:"recipientEmail,omitempty"`
	ItemKind       string `json:"itemKind"`             // "doc_required"
	ItemLabel      string `json:"itemLabel"`            // e.g. "DH-680"
	DueOn          string `json:"dueOn,omitempty"`      // ISO date or empty
	Channel        string `json:"channel"`              // recipient's preferred channel
	LastNagAt      string `json:"lastNagAt,omitempty"`  // ISO timestamp
	LastNagChannel string `json:"lastNagChannel,omitempty"`
	LastNagStatus  string `json:"lastNagStatus,omitempty"`
	NagCount       int    `json:"nagCount"`
	Status         string `json:"status"` // "missing" | "expired" | "expiring_soon"
}

type chaseStateResponse struct {
	Outstanding []ChaseRow `json:"outstanding"`
	Stuck       []ChaseRow `json:"stuck"`
}

type chaseActivityItem struct {
	ID            string `json:"id"`
	RecipientKind string `json:"recipientKind"`
	RecipientID   string `json:"recipientId"`
	RecipientName string `json:"recipientName"`
	ItemLabel     string `json:"itemLabel"`
	Channel       string `json:"channel"`
	EmailStatus   string `json:"emailStatus,omitempty"`
	SMSStatus     string `json:"smsStatus,omitempty"`
	CreatedAt     string `json:"createdAt"`
}

// GET /api/chase/state — admin or staff role. Returns outstanding chases
// (every open missing/expired/expiring-soon item, joined with the most
// recent nag for context) plus a separate "stuck" list for items where
// 3+ nags went out in the last 60 days.
func (h *ChaseHandler) State(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}

	staffRows, err := h.loadStaffOutstanding(r, pid)
	if err != nil {
		h.Log.Warn("chase: staff load failed", "err", err)
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	parentRows, err := h.loadParentOutstanding(r, pid)
	if err != nil {
		h.Log.Warn("chase: parent load failed", "err", err)
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	all := append(staffRows, parentRows...)

	// Split into outstanding + stuck. "Stuck" = 3+ nags in 60d. The same
	// row appears in `outstanding` (the global table) and in `stuck` so
	// the operator sees it in both contexts (full list + escalation queue).
	stuck := make([]ChaseRow, 0)
	for _, row := range all {
		if row.NagCount >= 3 {
			stuck = append(stuck, row)
		}
	}

	httpx.RenderJSON(w, http.StatusOK, chaseStateResponse{
		Outstanding: all,
		Stuck:       stuck,
	})
}

// GET /api/chase/activity?from=YYYY-MM-DD&to=YYYY-MM-DD
// Returns the chronological nag log for the tenant. Defaults to the last
// 30 days when params are omitted. Cap is 500 rows so the page doesn't
// have to deal with pagination yet.
func (h *ChaseHandler) Activity(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	to := time.Now().UTC()
	from := to.Add(-30 * 24 * time.Hour)
	if q := strings.TrimSpace(r.URL.Query().Get("from")); q != "" {
		if t, err := time.Parse("2006-01-02", q); err == nil {
			from = t
		}
	}
	if q := strings.TrimSpace(r.URL.Query().Get("to")); q != "" {
		if t, err := time.Parse("2006-01-02", q); err == nil {
			// Include the whole "to" day.
			to = t.Add(24 * time.Hour)
		}
	}

	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT sns.id,
		       sns.staff_id,
		       COALESCE(sns.doc_kind, ''),
		       sns.channel,
		       COALESCE(sns.email_status, ''),
		       COALESCE(sns.sms_status, ''),
		       sns.created_at,
		       COALESCE(st.first_name, '') AS staff_first,
		       COALESCE(st.last_name, '')  AS staff_last,
		       COALESCE(ch.first_name, '') AS child_first,
		       COALESCE(ch.last_name, '')  AS child_last,
		       COALESCE(ch.parent_name, '') AS parent_name
		  FROM staff_nag_sends sns
		  LEFT JOIN staff st    ON st.id = sns.staff_id
		  LEFT JOIN children ch ON ch.id = sns.staff_id
		 WHERE sns.provider_id = ?
		   AND sns.created_at >= ?
		   AND sns.created_at <  ?
		 ORDER BY sns.created_at DESC
		 LIMIT 500`,
		pid, from.Format("2006-01-02 15:04:05"), to.Format("2006-01-02 15:04:05"))
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	defer rows.Close()

	out := make([]chaseActivityItem, 0, 64)
	for rows.Next() {
		var (
			id, sid, dk, ch, es, ss, createdAt string
			staffFirst, staffLast              string
			childFirst, childLast, parentName  string
		)
		if err := rows.Scan(&id, &sid, &dk, &ch, &es, &ss, &createdAt,
			&staffFirst, &staffLast, &childFirst, &childLast, &parentName); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		// Resolve the subject:
		//   - If we found a staff row, it's a staff nag.
		//   - Otherwise the child join hit; show parent + child name.
		kind := "staff"
		name := strings.TrimSpace(staffFirst + " " + staffLast)
		if name == "" && (childFirst != "" || childLast != "") {
			kind = "parent"
			childFull := strings.TrimSpace(childFirst + " " + childLast)
			if parentName != "" {
				name = parentName + " (parent of " + childFull + ")"
			} else {
				name = "Parent of " + childFull
			}
		}
		out = append(out, chaseActivityItem{
			ID:            id,
			RecipientKind: kind,
			RecipientID:   sid,
			RecipientName: name,
			ItemLabel:     humanItemLabel(dk),
			Channel:       ch,
			EmailStatus:   es,
			SMSStatus:     ss,
			CreatedAt:     createdAt,
		})
	}

	httpx.RenderJSON(w, http.StatusOK, map[string]any{"items": out})
}

// POST /api/chase/trigger — admin only. Reuses NagHandler.Trigger /
// TriggerChild so we don't duplicate the SES + Twilio + audit-log code.
//
// Body: { recipientKind, recipientId, docKind?, note? }
func (h *ChaseHandler) Trigger(w http.ResponseWriter, r *http.Request) {
	if h.Nag == nil {
		httpx.RenderError(w, r, httpx.BadRequestf("nag pipeline not configured"))
		return
	}
	var in struct {
		RecipientKind string `json:"recipientKind"`
		RecipientID   string `json:"recipientId"`
		DocKind       string `json:"docKind"`
		Note          string `json:"note"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&in)
	}
	if in.RecipientID == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("recipientId required"))
		return
	}
	// Re-pack the nag handler's expected URL param + body. The cleanest path
	// is to mutate the chi route context. We avoid that by calling the same
	// SQL the nag handler would, but to stay DRY we just re-issue the request
	// to the underlying handler via http.ServeMux-style dispatch: we rebuild
	// the body and put the id on the chi context.
	body, _ := json.Marshal(map[string]string{
		"reason":   "manual",
		"doc_kind": in.DocKind,
		"note":     in.Note,
	})
	// Build a clone of the request whose Body re-serves the new JSON and
	// whose chi RouteContext carries {id} = recipientId. NagHandler.Trigger /
	// TriggerChild read both via chi.URLParam + JSON-decode the body.
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", in.RecipientID)
	ctx := context.WithValue(r.Context(), chi.RouteCtxKey, rctx)
	r2 := r.Clone(ctx)
	r2.Body = io.NopCloser(bytes.NewReader(body))

	switch strings.ToLower(in.RecipientKind) {
	case "staff":
		h.Nag.Trigger(w, r2)
	case "parent", "child":
		h.Nag.TriggerChild(w, r2)
	default:
		httpx.RenderError(w, r, httpx.BadRequestf("recipientKind must be 'staff' or 'parent'"))
	}
}

// loadStaffOutstanding pulls the outstanding staff certification rows for
// the tenant, joined with the most recent nag (and total nag count in the
// last 60 days) for that (staff_id, cert_type) pair.
func (h *ChaseHandler) loadStaffOutstanding(r *http.Request, pid string) ([]ChaseRow, error) {
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT scr.staff_id,
		       scr.cert_type,
		       scr.status,
		       COALESCE(scr.required_by, ''),
		       COALESCE(st.first_name, ''),
		       COALESCE(st.last_name, ''),
		       COALESCE(st.role, ''),
		       COALESCE(st.email, ''),
		       COALESCE(st.phone, ''),
		       COALESCE(st.preferred_contact_channel, 'email'),
		       (
		         SELECT created_at FROM staff_nag_sends sns
		          WHERE sns.staff_id = scr.staff_id
		            AND COALESCE(sns.doc_kind, '') = scr.cert_type
		            AND sns.provider_id = ?
		          ORDER BY created_at DESC LIMIT 1
		       ) AS last_nag_at,
		       (
		         SELECT channel FROM staff_nag_sends sns
		          WHERE sns.staff_id = scr.staff_id
		            AND COALESCE(sns.doc_kind, '') = scr.cert_type
		            AND sns.provider_id = ?
		          ORDER BY created_at DESC LIMIT 1
		       ) AS last_nag_channel,
		       (
		         SELECT COALESCE(email_status, sms_status) FROM staff_nag_sends sns
		          WHERE sns.staff_id = scr.staff_id
		            AND COALESCE(sns.doc_kind, '') = scr.cert_type
		            AND sns.provider_id = ?
		          ORDER BY created_at DESC LIMIT 1
		       ) AS last_nag_status,
		       (
		         SELECT COUNT(*) FROM staff_nag_sends sns
		          WHERE sns.staff_id = scr.staff_id
		            AND COALESCE(sns.doc_kind, '') = scr.cert_type
		            AND sns.provider_id = ?
		            AND sns.created_at >= datetime('now', '-60 days')
		       ) AS nag_count
		  FROM staff_certifications_required scr
		  JOIN staff st ON st.id = scr.staff_id
		 WHERE st.provider_id = ?
		   AND st.deleted_at IS NULL
		   AND scr.status IN ('missing','expired','expiring_soon')
		 ORDER BY scr.required_by IS NULL, scr.required_by ASC, st.last_name ASC`,
		pid, pid, pid, pid, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ChaseRow, 0)
	for rows.Next() {
		var (
			sid, ct, st, dueOn string
			first, last, role  string
			email, phone, ch   string
			lastAt, lastCh, lastSt sql.NullString
			nagCount           int
		)
		if err := rows.Scan(&sid, &ct, &st, &dueOn, &first, &last, &role,
			&email, &phone, &ch, &lastAt, &lastCh, &lastSt, &nagCount); err != nil {
			return nil, err
		}
		row := ChaseRow{
			RecipientKind:  "staff",
			RecipientID:    sid,
			RecipientName:  strings.TrimSpace(first + " " + last),
			RecipientRole:  role,
			RecipientEmail: email,
			RecipientPhone: phone,
			ItemKind:       "doc_required",
			ItemLabel:      humanItemLabel(ct),
			DueOn:          dueOn,
			Channel:        ch,
			NagCount:       nagCount,
			Status:         st,
		}
		if lastAt.Valid {
			row.LastNagAt = lastAt.String
		}
		if lastCh.Valid {
			row.LastNagChannel = lastCh.String
		}
		if lastSt.Valid {
			row.LastNagStatus = lastSt.String
		}
		out = append(out, row)
	}
	return out, nil
}

// loadParentOutstanding mirrors loadStaffOutstanding for child docs.
// staff_nag_sends.staff_id holds the child id on the parent path (see
// nag.go TriggerChild for the rationale).
func (h *ChaseHandler) loadParentOutstanding(r *http.Request, pid string) ([]ChaseRow, error) {
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT cdr.child_id,
		       cdr.doc_type,
		       cdr.status,
		       COALESCE(cdr.required_by, ''),
		       COALESCE(ch.first_name, ''),
		       COALESCE(ch.last_name, ''),
		       COALESCE(ch.parent_name, ''),
		       COALESCE(ch.parent_email, ''),
		       COALESCE(ch.parent_phone, ''),
		       COALESCE(ch.preferred_contact_channel, 'email'),
		       (
		         SELECT created_at FROM staff_nag_sends sns
		          WHERE sns.staff_id = cdr.child_id
		            AND COALESCE(sns.doc_kind, '') = cdr.doc_type
		            AND sns.provider_id = ?
		          ORDER BY created_at DESC LIMIT 1
		       ) AS last_nag_at,
		       (
		         SELECT channel FROM staff_nag_sends sns
		          WHERE sns.staff_id = cdr.child_id
		            AND COALESCE(sns.doc_kind, '') = cdr.doc_type
		            AND sns.provider_id = ?
		          ORDER BY created_at DESC LIMIT 1
		       ) AS last_nag_channel,
		       (
		         SELECT COALESCE(email_status, sms_status) FROM staff_nag_sends sns
		          WHERE sns.staff_id = cdr.child_id
		            AND COALESCE(sns.doc_kind, '') = cdr.doc_type
		            AND sns.provider_id = ?
		          ORDER BY created_at DESC LIMIT 1
		       ) AS last_nag_status,
		       (
		         SELECT COUNT(*) FROM staff_nag_sends sns
		          WHERE sns.staff_id = cdr.child_id
		            AND COALESCE(sns.doc_kind, '') = cdr.doc_type
		            AND sns.provider_id = ?
		            AND sns.created_at >= datetime('now', '-60 days')
		       ) AS nag_count
		  FROM child_documents_required cdr
		  JOIN children ch ON ch.id = cdr.child_id
		 WHERE ch.provider_id = ?
		   AND ch.deleted_at IS NULL
		   AND COALESCE(ch.withdrawn_at, '') = ''
		   AND cdr.status IN ('missing','expired','expiring_soon')
		 ORDER BY cdr.required_by IS NULL, cdr.required_by ASC, ch.last_name ASC`,
		pid, pid, pid, pid, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ChaseRow, 0)
	for rows.Next() {
		var (
			cid, dt, st, dueOn string
			childFirst, childLast, parentName string
			email, phone, ch   string
			lastAt, lastCh, lastSt sql.NullString
			nagCount           int
		)
		if err := rows.Scan(&cid, &dt, &st, &dueOn, &childFirst, &childLast,
			&parentName, &email, &phone, &ch, &lastAt, &lastCh, &lastSt, &nagCount); err != nil {
			return nil, err
		}
		childFull := strings.TrimSpace(childFirst + " " + childLast)
		recipName := parentName
		if recipName == "" {
			recipName = "Parent of " + childFull
		} else {
			recipName = parentName + " (parent of " + childFull + ")"
		}
		row := ChaseRow{
			RecipientKind:  "parent",
			RecipientID:    cid,
			RecipientName:  recipName,
			RecipientRole:  "Parent",
			RecipientEmail: email,
			RecipientPhone: phone,
			ItemKind:       "doc_required",
			ItemLabel:      humanItemLabel(dt),
			DueOn:          dueOn,
			Channel:        ch,
			NagCount:       nagCount,
			Status:         st,
		}
		if lastAt.Valid {
			row.LastNagAt = lastAt.String
		}
		if lastCh.Valid {
			row.LastNagChannel = lastCh.String
		}
		if lastSt.Valid {
			row.LastNagStatus = lastSt.String
		}
		out = append(out, row)
	}
	return out, nil
}

// humanItemLabel turns "cpr_cert" into "CPR certification", and falls
// back to title-casing the slug otherwise. Mirrors humanDocKind in nag.go
// but pulls one level higher (full label, not phrase fragment).
func humanItemLabel(slug string) string {
	if slug == "" {
		return "Missing item"
	}
	switch strings.ToLower(slug) {
	case "cpr_cert":
		return "CPR certification"
	case "first_aid_cert":
		return "First Aid certification"
	case "tb_test":
		return "TB test"
	case "background_check":
		return "Background check"
	case "immunization_record", "dh_680", "dh680":
		return "DH-680 immunization record"
	case "physical_exam", "dh_3040", "dh3040":
		return "Physical exam (DH 3040)"
	case "religious_exemption", "dh_681", "dh681":
		return "Religious exemption (DH 681)"
	case "i9", "i_9":
		return "I-9 form"
	case "w4", "w_4":
		return "W-4 form"
	}
	cleaned := strings.ReplaceAll(slug, "_", " ")
	return strings.ToUpper(cleaned[:1]) + cleaned[1:]
}

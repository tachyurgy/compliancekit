package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	"github.com/tachyurgy/compliancekit/backend/internal/magiclink"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
	"github.com/tachyurgy/compliancekit/backend/internal/notify"
)

// NagHandler exposes operator-triggered "nag this staff member" actions.
// Distinct from notify.ChaseService — chase runs daily and finds expiring
// docs; this is the manual lever an operator pulls when someone hasn't
// uploaded what they were supposed to.
type NagHandler struct {
	Pool         *sql.DB
	Magic        *magiclink.Service
	Emailer      *notify.Emailer
	SMS          *notify.SMSSender
	FrontendBase string
	Log          *slog.Logger
}

type nagRequest struct {
	Reason  string `json:"reason"`   // "missing" | "manual"
	DocKind string `json:"doc_kind"` // optional: cpr_cert, tb_test, etc.
	Note    string `json:"note"`     // optional free-text included in the message
}

type nagResult struct {
	StaffID     string `json:"staff_id"`
	Channel     string `json:"channel"`
	EmailStatus string `json:"email_status,omitempty"`
	SMSStatus   string `json:"sms_status,omitempty"`
	UploadURL   string `json:"upload_url"`
}

// POST /api/staff/{id}/nag — admin only.
func (h *NagHandler) Trigger(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	uid := mw.UserIDFrom(r.Context())
	staffID := chi.URLParam(r, "id")
	if pid == "" || staffID == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}

	var in nagRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&in)
	}
	in.Reason = strings.TrimSpace(strings.ToLower(in.Reason))
	if in.Reason == "" {
		in.Reason = "manual"
	}
	if in.Reason != "missing" && in.Reason != "manual" {
		httpx.RenderError(w, r, httpx.BadRequestf("reason must be missing or manual"))
		return
	}

	var firstName, lastName, email, phone, channel string
	err := h.Pool.QueryRowContext(r.Context(), `
		SELECT first_name, last_name, COALESCE(email,''), COALESCE(phone,''),
		       COALESCE(preferred_contact_channel,'email')
		FROM staff WHERE id = ? AND provider_id = ? AND deleted_at IS NULL`,
		staffID, pid).Scan(&firstName, &lastName, &email, &phone, &channel)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.RenderError(w, r, httpx.ErrNotFound)
			return
		}
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	var providerName string
	_ = h.Pool.QueryRowContext(r.Context(),
		`SELECT COALESCE(name, legal_name, '') FROM providers WHERE id = ?`, pid).Scan(&providerName)

	_, path, err := h.Magic.Generate(r.Context(), magiclink.KindStaffUpload, pid, staffID, 0)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	uploadURL := h.FrontendBase + path

	emailStatus := "skipped"
	smsStatus := "skipped"
	wantEmail := channel == "email" || channel == "both"
	wantSMS := channel == "sms" || channel == "both"

	if wantEmail {
		if email == "" {
			emailStatus = "failed:no_email_on_file"
		} else if h.Emailer == nil {
			emailStatus = "failed:emailer_disabled"
		} else {
			subject, html, text := renderNagEmail(nagEmailData{
				RecipientFirst: firstName,
				ProviderName:   providerName,
				DocKind:        in.DocKind,
				Reason:         in.Reason,
				Note:           in.Note,
				UploadURL:      uploadURL,
			})
			if err := h.Emailer.Send(r.Context(), notify.EmailMessage{
				To: email, Subject: subject, HTMLBody: html, PlainBody: text,
			}); err != nil {
				h.Log.Warn("nag: email send failed", "err", err, "staff", staffID)
				emailStatus = "failed:" + truncate(err.Error(), 100)
			} else {
				emailStatus = "sent"
			}
		}
	}

	if wantSMS {
		if phone == "" {
			smsStatus = "failed:no_phone_on_file"
		} else if h.SMS == nil {
			smsStatus = "failed:sms_disabled"
		} else {
			body := renderNagSMS(in.DocKind, providerName, uploadURL)
			if _, err := h.SMS.Send(phone, body); err != nil {
				h.Log.Warn("nag: sms send failed", "err", err, "staff", staffID)
				smsStatus = "failed:" + truncate(err.Error(), 100)
			} else {
				smsStatus = "sent"
			}
		}
	}

	if _, err := h.Pool.ExecContext(r.Context(), `
		INSERT INTO staff_nag_sends (id, provider_id, staff_id, reason, doc_kind,
		                             channel, email_status, sms_status, triggered_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		base62.NewID()[:22], pid, staffID, in.Reason, in.DocKind,
		channel, emailStatus, smsStatus, uid); err != nil {
		h.Log.Warn("nag: insert log failed", "err", err)
	}

	httpx.RenderJSON(w, http.StatusOK, nagResult{
		StaffID: staffID, Channel: channel,
		EmailStatus: emailStatus, SMSStatus: smsStatus,
		UploadURL: uploadURL,
	})
}

// GET /api/staff/{id}/nag-history — admin only. Returns recent nags for one staff member.
func (h *NagHandler) History(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	staffID := chi.URLParam(r, "id")
	if pid == "" || staffID == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT id, reason, COALESCE(doc_kind,''), channel,
		       COALESCE(email_status,''), COALESCE(sms_status,''),
		       COALESCE(triggered_by,''), created_at
		FROM staff_nag_sends
		WHERE provider_id = ? AND staff_id = ?
		ORDER BY created_at DESC LIMIT 50`, pid, staffID)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	defer rows.Close()
	type item struct {
		ID          string `json:"id"`
		Reason      string `json:"reason"`
		DocKind     string `json:"doc_kind,omitempty"`
		Channel     string `json:"channel"`
		EmailStatus string `json:"email_status,omitempty"`
		SMSStatus   string `json:"sms_status,omitempty"`
		TriggeredBy string `json:"triggered_by,omitempty"`
		CreatedAt   string `json:"created_at"`
	}
	out := make([]item, 0)
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ID, &it.Reason, &it.DocKind, &it.Channel,
			&it.EmailStatus, &it.SMSStatus, &it.TriggeredBy, &it.CreatedAt); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		out = append(out, it)
	}
	httpx.RenderJSON(w, http.StatusOK, map[string]any{"items": out})
}

type nagEmailData struct {
	RecipientFirst string
	ProviderName   string
	DocKind        string
	Reason         string
	Note           string
	UploadURL      string
}

func renderNagEmail(d nagEmailData) (subject, html, text string) {
	docPhrase := "your missing documents"
	if d.DocKind != "" {
		docPhrase = humanDocKind(d.DocKind)
	}
	subject = fmt.Sprintf("Action needed: upload %s", docPhrase)
	intro := fmt.Sprintf("Hi %s,", strings.TrimSpace(d.RecipientFirst))
	if d.RecipientFirst == "" {
		intro = "Hi,"
	}
	noteBlock := ""
	if d.Note != "" {
		noteBlock = fmt.Sprintf("\n\n%s\n", d.Note)
	}
	text = fmt.Sprintf(`%s

%s needs you to upload %s before your next compliance check.%s

Use this private link to upload (it expires in 7 days):
%s

If you've already uploaded this, just ignore the message — it'll stop on its own.

Thanks,
%s
`, intro, d.ProviderName, docPhrase, noteBlock, d.UploadURL, d.ProviderName)

	html = fmt.Sprintf(`<p>%s</p>
<p>%s needs you to upload <strong>%s</strong> before your next compliance check.</p>
%s
<p><a href="%s" style="display:inline-block;padding:10px 16px;border-radius:6px;background:#0d6f5e;color:#fff;text-decoration:none">Upload here</a></p>
<p style="color:#666;font-size:12px">Link expires in 7 days. Already uploaded? Ignore this — the reminder will stop.</p>
<p>— %s</p>`,
		htmlEscape(intro), htmlEscape(d.ProviderName), htmlEscape(docPhrase),
		func() string {
			if d.Note == "" {
				return ""
			}
			return "<p>" + htmlEscape(d.Note) + "</p>"
		}(),
		d.UploadURL, htmlEscape(d.ProviderName))
	return
}

func renderNagSMS(docKind, providerName, url string) string {
	doc := "your missing documents"
	if docKind != "" {
		doc = humanDocKind(docKind)
	}
	msg := fmt.Sprintf("%s: please upload %s. %s (expires in 7 days)", providerName, doc, url)
	if len(msg) > 320 {
		msg = msg[:319] + "…"
	}
	return msg
}

// humanDocKind makes "cpr_cert" → "your CPR certification" for the message body.
func humanDocKind(k string) string {
	k = strings.ReplaceAll(k, "_", " ")
	switch strings.ToLower(k) {
	case "cpr cert":
		return "your CPR certification"
	case "first aid cert":
		return "your First Aid certification"
	case "tb test":
		return "your TB test result"
	case "background check":
		return "your background check"
	case "i 9", "i9":
		return "your I-9 form"
	case "w 4", "w4":
		return "your W-4 form"
	default:
		return "your " + k
	}
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;")
	return r.Replace(s)
}

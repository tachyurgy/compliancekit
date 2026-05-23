package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
	"github.com/tachyurgy/compliancekit/backend/internal/notify"
)

// MessagesHandler powers the in-app message composer (REQ-043). It
// fans out free-form messages across email + SMS using the existing
// notify.Emailer / notify.SMSSender, and writes one row per (channel,
// recipient) into outgoing_messages so the per-recipient history tab
// (REQ-044) has something to read.
type MessagesHandler struct {
	Pool    *sql.DB
	Emailer *notify.Emailer
	SMS     *notify.SMSSender
	Log     *slog.Logger
}

type messageRecipient struct {
	Kind string `json:"kind"` // "staff" | "parent"
	ID   string `json:"id"`
}

type sendRequest struct {
	Recipients []messageRecipient `json:"recipients"`
	Channel    string             `json:"channel"` // "email" | "sms" | "both"
	Subject    string             `json:"subject"`
	Body       string             `json:"body"`
}

type sendResult struct {
	RecipientKind string `json:"recipientKind"`
	RecipientID   string `json:"recipientId"`
	RecipientName string `json:"recipientName"`
	Channel       string `json:"channel"`
	Status        string `json:"status"`          // "sent" | "failed" | "skipped"
	Error         string `json:"error,omitempty"` // populated on failed
}

// POST /api/messages/send — admin only.
func (h *MessagesHandler) Send(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	uid := mw.UserIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	var in sendRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpx.RenderError(w, r, httpx.BadRequestf("invalid JSON body"))
		return
	}
	in.Channel = strings.TrimSpace(strings.ToLower(in.Channel))
	if in.Channel != "email" && in.Channel != "sms" && in.Channel != "both" {
		httpx.RenderError(w, r, httpx.BadRequestf("channel must be email, sms, or both"))
		return
	}
	if strings.TrimSpace(in.Body) == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("body required"))
		return
	}
	if len(in.Recipients) == 0 {
		httpx.RenderError(w, r, httpx.BadRequestf("recipients required"))
		return
	}
	// Cap fan-out so a bug in the picker can't blast 10k SMS.
	if len(in.Recipients) > 200 {
		httpx.RenderError(w, r, httpx.BadRequestf("at most 200 recipients per send"))
		return
	}

	var providerName string
	_ = h.Pool.QueryRowContext(r.Context(),
		`SELECT COALESCE(name, legal_name, '') FROM providers WHERE id = ?`, pid).
		Scan(&providerName)

	wantEmail := in.Channel == "email" || in.Channel == "both"
	wantSMS := in.Channel == "sms" || in.Channel == "both"

	out := make([]sendResult, 0, len(in.Recipients)*2)
	for _, rec := range in.Recipients {
		rec.Kind = strings.ToLower(strings.TrimSpace(rec.Kind))
		if rec.Kind != "staff" && rec.Kind != "parent" {
			out = append(out, sendResult{
				RecipientKind: rec.Kind, RecipientID: rec.ID,
				Status: "failed", Error: "unknown recipientKind",
			})
			continue
		}
		name, email, phone, err := h.resolveRecipient(r, pid, rec)
		if err != nil {
			out = append(out, sendResult{
				RecipientKind: rec.Kind, RecipientID: rec.ID,
				Status: "failed", Error: err.Error(),
			})
			continue
		}

		if wantEmail {
			result := h.deliverEmail(r, pid, uid, providerName, rec, name, email, in.Subject, in.Body)
			out = append(out, result)
		}
		if wantSMS {
			result := h.deliverSMS(r, pid, uid, providerName, rec, name, phone, in.Body)
			out = append(out, result)
		}
	}

	httpx.RenderJSON(w, http.StatusOK, map[string]any{"results": out})
}

func (h *MessagesHandler) resolveRecipient(r *http.Request, pid string, rec messageRecipient) (name, email, phone string, err error) {
	switch rec.Kind {
	case "staff":
		var first, last string
		err = h.Pool.QueryRowContext(r.Context(), `
			SELECT first_name, last_name, COALESCE(email, ''), COALESCE(phone, '')
			  FROM staff WHERE id = ? AND provider_id = ? AND deleted_at IS NULL`,
			rec.ID, pid).Scan(&first, &last, &email, &phone)
		if err != nil {
			return "", "", "", fmt.Errorf("staff not found")
		}
		name = strings.TrimSpace(first + " " + last)
	case "parent":
		var childFirst, childLast, parentName string
		err = h.Pool.QueryRowContext(r.Context(), `
			SELECT first_name, last_name, COALESCE(parent_name, ''),
			       COALESCE(parent_email, ''), COALESCE(parent_phone, '')
			  FROM children WHERE id = ? AND provider_id = ? AND deleted_at IS NULL`,
			rec.ID, pid).Scan(&childFirst, &childLast, &parentName, &email, &phone)
		if err != nil {
			return "", "", "", fmt.Errorf("child not found")
		}
		childFull := strings.TrimSpace(childFirst + " " + childLast)
		if parentName != "" {
			name = parentName + " (parent of " + childFull + ")"
		} else {
			name = "Parent of " + childFull
		}
	}
	return name, email, phone, nil
}

func (h *MessagesHandler) deliverEmail(r *http.Request, pid, uid, providerName string, rec messageRecipient, name, email, subject, body string) sendResult {
	res := sendResult{RecipientKind: rec.Kind, RecipientID: rec.ID, RecipientName: name, Channel: "email"}
	if email == "" {
		res.Status = "failed"
		res.Error = "no email on file"
		h.insert(r, pid, uid, rec, "email", subject, body, "failed", res.Error)
		return res
	}
	if h.Emailer == nil {
		res.Status = "failed"
		res.Error = "emailer disabled"
		h.insert(r, pid, uid, rec, "email", subject, body, "failed", res.Error)
		return res
	}
	subj := strings.TrimSpace(subject)
	if subj == "" {
		subj = providerName + " — a quick note"
	}
	html := renderComposerHTML(providerName, body)
	err := h.Emailer.Send(r.Context(), notify.EmailMessage{
		To: email, Subject: subj, HTMLBody: html, PlainBody: body,
	})
	if err != nil {
		res.Status = "failed"
		res.Error = err.Error()
		h.insert(r, pid, uid, rec, "email", subj, body, "failed", err.Error())
		return res
	}
	res.Status = "sent"
	h.insert(r, pid, uid, rec, "email", subj, body, "sent", "")
	return res
}

func (h *MessagesHandler) deliverSMS(r *http.Request, pid, uid, providerName string, rec messageRecipient, name, phone, body string) sendResult {
	res := sendResult{RecipientKind: rec.Kind, RecipientID: rec.ID, RecipientName: name, Channel: "sms"}
	if phone == "" {
		res.Status = "failed"
		res.Error = "no phone on file"
		h.insert(r, pid, uid, rec, "sms", "", body, "failed", res.Error)
		return res
	}
	if h.SMS == nil {
		res.Status = "failed"
		res.Error = "sms disabled"
		h.insert(r, pid, uid, rec, "sms", "", body, "failed", res.Error)
		return res
	}
	// Twilio supports concatenated SMS up to ~1600 chars; trim defensively.
	smsBody := body
	if providerName != "" {
		smsBody = providerName + ": " + smsBody
	}
	if len(smsBody) > 1500 {
		smsBody = smsBody[:1499] + "…"
	}
	if _, err := h.SMS.Send(phone, smsBody); err != nil {
		res.Status = "failed"
		res.Error = err.Error()
		h.insert(r, pid, uid, rec, "sms", "", body, "failed", err.Error())
		return res
	}
	res.Status = "sent"
	h.insert(r, pid, uid, rec, "sms", "", body, "sent", "")
	return res
}

func (h *MessagesHandler) insert(r *http.Request, pid, uid string, rec messageRecipient, channel, subject, body, status, errMsg string) {
	id := base62.NewID()[:22]
	var sender any = uid
	if uid == "" {
		sender = nil
	}
	var subj any = subject
	if subject == "" {
		subj = nil
	}
	var sentAt any
	if status == "sent" {
		sentAt = "CURRENT_TIMESTAMP_REPLACE"
	}
	var errVal any = errMsg
	if errMsg == "" {
		errVal = nil
	}
	// Use plain Exec with parameters; sent_at goes in via SQL function
	// when status is "sent" (datetime('now') for SQLite parity).
	if sentAt == nil {
		_, err := h.Pool.ExecContext(r.Context(), `
			INSERT INTO outgoing_messages
			  (id, provider_id, sender_user_id, recipient_kind, recipient_id,
			   channel, subject, body, status, error, sent_at, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, CURRENT_TIMESTAMP)`,
			id, pid, sender, rec.Kind, rec.ID, channel, subj, body, status, errVal)
		if err != nil {
			h.Log.Warn("messages: insert failed", "err", err)
		}
		return
	}
	_, err := h.Pool.ExecContext(r.Context(), `
		INSERT INTO outgoing_messages
		  (id, provider_id, sender_user_id, recipient_kind, recipient_id,
		   channel, subject, body, status, error, sent_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		id, pid, sender, rec.Kind, rec.ID, channel, subj, body, status, errVal)
	if err != nil {
		h.Log.Warn("messages: insert failed", "err", err)
	}
}

type messageItem struct {
	ID         string `json:"id"`
	Channel    string `json:"channel"`
	Subject    string `json:"subject,omitempty"`
	Body       string `json:"body"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
	SentAt     string `json:"sentAt,omitempty"`
	CreatedAt  string `json:"createdAt"`
	SenderName string `json:"senderName,omitempty"`
}

// GET /api/messages?recipientKind=&recipientId=&limit=
func (h *MessagesHandler) History(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	kind := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("recipientKind")))
	id := strings.TrimSpace(r.URL.Query().Get("recipientId"))
	if kind != "staff" && kind != "parent" {
		httpx.RenderError(w, r, httpx.BadRequestf("recipientKind must be staff or parent"))
		return
	}
	if id == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("recipientId required"))
		return
	}
	limit := 50
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT om.id, om.channel, COALESCE(om.subject, ''), om.body,
		       om.status, COALESCE(om.error, ''),
		       COALESCE(om.sent_at, ''), om.created_at,
		       COALESCE(u.full_name, '')
		  FROM outgoing_messages om
		  LEFT JOIN users u ON u.id = om.sender_user_id
		 WHERE om.provider_id = ?
		   AND om.recipient_kind = ?
		   AND om.recipient_id = ?
		 ORDER BY om.created_at DESC
		 LIMIT ?`, pid, kind, id, limit)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	defer rows.Close()
	out := make([]messageItem, 0)
	for rows.Next() {
		var it messageItem
		if err := rows.Scan(&it.ID, &it.Channel, &it.Subject, &it.Body,
			&it.Status, &it.Error, &it.SentAt, &it.CreatedAt, &it.SenderName); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		out = append(out, it)
	}
	httpx.RenderJSON(w, http.StatusOK, map[string]any{"items": out})
}

type recipientOption struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Role      string `json:"role,omitempty"`
	ChildName string `json:"childName,omitempty"`
	Email     string `json:"email,omitempty"`
	Phone     string `json:"phone,omitempty"`
}

// GET /api/messages/recipients — flat picker list for the composer UI.
func (h *MessagesHandler) Recipients(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	staff := make([]recipientOption, 0)
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT id, first_name, last_name, COALESCE(role, ''), COALESCE(email, ''), COALESCE(phone, '')
		  FROM staff
		 WHERE provider_id = ? AND deleted_at IS NULL AND status = 'active'
		 ORDER BY last_name, first_name`, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	for rows.Next() {
		var o recipientOption
		var first, last string
		if err := rows.Scan(&o.ID, &first, &last, &o.Role, &o.Email, &o.Phone); err != nil {
			rows.Close()
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		o.Kind = "staff"
		o.Name = strings.TrimSpace(first + " " + last)
		staff = append(staff, o)
	}
	rows.Close()

	parents := make([]recipientOption, 0)
	rows, err = h.Pool.QueryContext(r.Context(), `
		SELECT id, first_name, last_name, COALESCE(parent_name, ''),
		       COALESCE(parent_email, ''), COALESCE(parent_phone, '')
		  FROM children
		 WHERE provider_id = ? AND deleted_at IS NULL AND COALESCE(withdrawn_at, '') = ''
		 ORDER BY last_name, first_name`, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	defer rows.Close()
	for rows.Next() {
		var o recipientOption
		var first, last, parentName string
		if err := rows.Scan(&o.ID, &first, &last, &parentName, &o.Email, &o.Phone); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		o.Kind = "parent"
		o.ChildName = strings.TrimSpace(first + " " + last)
		if parentName != "" {
			o.Name = parentName
		} else {
			o.Name = "Parent of " + o.ChildName
		}
		parents = append(parents, o)
	}

	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"staff":   staff,
		"parents": parents,
	})
}

// renderComposerHTML wraps the operator's plaintext body in a minimal
// Levelbrook-branded email frame. URLs in the body are auto-linked and
// newlines become <br>.
func renderComposerHTML(providerName, body string) string {
	escaped := htmlEscape(body)
	// Auto-link http/https tokens.
	escaped = autolinkHTTP(escaped)
	escaped = strings.ReplaceAll(escaped, "\n", "<br>")
	footer := ""
	if providerName != "" {
		footer = fmt.Sprintf(`<p style="color:#666;font-size:12px;margin-top:24px">— %s</p>`, htmlEscape(providerName))
	}
	return fmt.Sprintf(`<div style="font-family:system-ui,Segoe UI,Arial,sans-serif;color:#222;max-width:600px;margin:auto;padding:16px">
<div style="font-size:15px;line-height:1.5">%s</div>
%s
</div>`, escaped, footer)
}

// autolinkHTTP wraps bare http/https URLs in <a> tags. Conservative —
// stops at whitespace or HTML-escape sentinels so we never link across
// elements.
func autolinkHTTP(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		switch {
		case strings.HasPrefix(s[i:], "https://"), strings.HasPrefix(s[i:], "http://"):
			j := i
			for j < len(s) && !isURLBreak(s[j]) {
				j++
			}
			url := s[i:j]
			b.WriteString(`<a href="` + url + `" style="color:#0d6f5e">` + url + `</a>`)
			i = j
		default:
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String()
}

func isURLBreak(c byte) bool {
	switch c {
	case ' ', '\n', '\t', '<', '"', '\'', ')', ']':
		return true
	}
	return false
}

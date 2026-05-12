package handlers

// REQ-007: Twilio inbound-SMS webhook. Twilio posts every inbound SMS to
// the configured "Messaging Service > Inbound URL" or per-number webhook.
// We only care about the STOP keyword family — RFC-style opt-out is
// TCPA-mandatory and Twilio also auto-blocks future sends to a number that
// replied STOP, but we mirror the row in our own unsubscribes table so the
// chase code can short-circuit before calling Twilio at all (and so the
// operator can see the opt-out in the UI).
//
// The webhook receives application/x-www-form-urlencoded with at minimum:
//   From         — E.164 phone, e.g. +13055551212
//   To           — our Twilio number
//   Body         — message text
//   MessageSid   — Twilio message id
//
// Reply body is TwiML (<Response><Message>…</Message></Response>). Twilio
// expects 200 with the TwiML payload; an empty 200 also works but the
// user-visible reply is friendlier with the explicit confirmation.

import (
	"database/sql"
	"log/slog"
	"net/http"
	"strings"
)

type TwilioWebhookHandler struct {
	Pool *sql.DB
	Log  *slog.Logger
}

// stopKeywords are the case-insensitive opt-out keywords Twilio recognizes
// and which most carriers honor without question. We deliberately do NOT
// also act on "HELP" or "INFO" — those are help queries, not opt-outs.
var stopKeywords = map[string]bool{
	"STOP":        true,
	"STOPALL":     true,
	"UNSUBSCRIBE": true,
	"END":         true,
	"QUIT":        true,
	"CANCEL":      true,
	"REVOKE":      true,
	"OPTOUT":      true,
}

// startKeywords reverse the opt-out (TCPA also mandates supporting this).
// We DELETE the unsubscribes row so chase resumes sending.
var startKeywords = map[string]bool{
	"START":     true,
	"YES":       true,
	"UNSTOP":    true,
	"SUBSCRIBE": true,
}

// Handle mounts at POST /webhooks/twilio/sms. We respond with TwiML
// (Content-Type: text/xml) so Twilio relays the confirmation back to the
// sender. Even non-STOP inbound messages get a 200 so Twilio doesn't retry.
func (h *TwilioWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		// Twilio is strict-form; anything else isn't valid input.
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	from := strings.TrimSpace(r.PostForm.Get("From"))
	body := strings.TrimSpace(r.PostForm.Get("Body"))
	upper := strings.ToUpper(body)

	switch {
	case stopKeywords[upper]:
		if from != "" {
			if err := insertUnsubscribe(r.Context(), h.Pool, "sms", from, "twilio_stop"); err != nil {
				h.Log.Warn("twilio webhook: insert unsubscribe failed", "err", err, "from", from)
			} else {
				h.Log.Info("twilio webhook: STOP recorded", "from", from)
			}
		}
		writeTwiML(w, "You have been unsubscribed from Levelbrook reminders. Reply START to resume.")
		return
	case startKeywords[upper]:
		if from != "" {
			if _, err := h.Pool.ExecContext(r.Context(),
				`DELETE FROM unsubscribes WHERE channel = 'sms' AND recipient = ?`, from); err != nil {
				h.Log.Warn("twilio webhook: clear unsubscribe failed", "err", err, "from", from)
			} else {
				h.Log.Info("twilio webhook: START recorded", "from", from)
			}
		}
		writeTwiML(w, "You are re-subscribed to Levelbrook reminders. Reply STOP to opt out.")
		return
	default:
		// Generic inbound — we don't run a two-way SMS conversation. Reply
		// with a brief help line. Twilio still honors STOP at carrier level
		// even if we miss it here.
		writeTwiML(w, "Levelbrook sends document reminders only. Reply STOP to opt out.")
		return
	}
}

func writeTwiML(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	// Minimal TwiML; no XML escaping needed for the static strings above.
	_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Response><Message>` + message + `</Message></Response>`))
}

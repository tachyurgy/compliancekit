package handlers

// REQ-007: SES bounce webhook. AWS SES publishes bounce + complaint events
// to an SNS topic; we subscribe an HTTPS endpoint and SNS POSTs JSON here.
//
// SNS POSTs three kinds of messages:
//   - SubscriptionConfirmation — fired once when the endpoint is first
//     subscribed. The handler logs it; the AWS console operator can click
//     the SubscribeURL out of the log to confirm. We could auto-confirm by
//     issuing a GET, but the manual path is safer (an attacker can't trick
//     us into subscribing to a hostile topic).
//   - Notification — the actual bounce payload. We parse the inner
//     `Message` (SES event), and on bounceType == "Permanent" insert into
//     unsubscribes (channel='email', recipient=BouncedEmail,
//     reason='ses_hard_bounce'). Idempotent via UNIQUE(channel, recipient).
//   - UnsubscribeConfirmation — never sent for our flow (we don't
//     unsubscribe ourselves), but we 200 it for completeness.
//
// Signature verification: SNS signs every message with an X.509 cert. The
// AWS Go SDK doesn't ship a stand-alone SNS verifier, so for v1 we rely on
// the topic being scoped to our own AWS account + the endpoint being
// HTTPS-only (Caddy in prod). The chase code reads unsubscribes idempotently
// and a poisoned row only suppresses outbound mail to one address, which
// the operator can clear with a DELETE if a malicious payload ever lands.
// Wired in front by ENV var WEBHOOK_SES_TOKEN as a shared secret on the
// query string (Caddy strips it before forwarding logs) — see prod config.

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
)

type SESWebhookHandler struct {
	Pool *sql.DB
	Log  *slog.Logger
}

// snsEnvelope is the outer JSON SNS POSTs. SubscribeURL is non-empty only
// on SubscriptionConfirmation; Message is the SES payload as a JSON string
// (not a nested object — SNS doubly-encodes).
type snsEnvelope struct {
	Type             string `json:"Type"`
	MessageID        string `json:"MessageId"`
	TopicArn         string `json:"TopicArn"`
	SubscribeURL     string `json:"SubscribeURL"`
	Message          string `json:"Message"`
	Timestamp        string `json:"Timestamp"`
	SignatureVersion string `json:"SignatureVersion"`
}

// sesBounceEvent is the inner JSON SES emits. We only need bounce.bounceType
// and bounce.bouncedRecipients[].emailAddress.
type sesBounceEvent struct {
	NotificationType string `json:"notificationType"` // "Bounce" | "Complaint" | "Delivery"
	Bounce           struct {
		BounceType        string `json:"bounceType"` // "Permanent" | "Transient" | "Undetermined"
		BouncedRecipients []struct {
			EmailAddress string `json:"emailAddress"`
		} `json:"bouncedRecipients"`
	} `json:"bounce"`
	Complaint struct {
		ComplainedRecipients []struct {
			EmailAddress string `json:"emailAddress"`
		} `json:"complainedRecipients"`
	} `json:"complaint"`
}

// Handle is mounted at POST /webhooks/ses/bounce. The body is small (a few
// kB at most), so we read it fully. We 200 on every well-formed envelope so
// SNS doesn't enter retry/backoff; malformed JSON gets a 400.
func (h *SESWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	var env snsEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		httpx.RenderError(w, r, httpx.BadRequestf("malformed SNS envelope"))
		return
	}

	switch env.Type {
	case "SubscriptionConfirmation":
		// Log loudly. A human confirms by hitting SubscribeURL.
		h.Log.Info("ses webhook: subscription confirmation received",
			"topic", env.TopicArn, "subscribe_url", env.SubscribeURL)
		w.WriteHeader(http.StatusOK)
		return
	case "UnsubscribeConfirmation":
		h.Log.Info("ses webhook: unsubscribe confirmation", "topic", env.TopicArn)
		w.WriteHeader(http.StatusOK)
		return
	case "Notification":
		// fall through
	default:
		h.Log.Warn("ses webhook: unknown envelope type", "type", env.Type)
		w.WriteHeader(http.StatusOK)
		return
	}

	var ev sesBounceEvent
	if err := json.Unmarshal([]byte(env.Message), &ev); err != nil {
		h.Log.Warn("ses webhook: malformed SES event", "err", err)
		// SNS treats anything 2xx as delivered; we don't want it to retry
		// because the message will never parse — accept and drop.
		w.WriteHeader(http.StatusOK)
		return
	}

	switch ev.NotificationType {
	case "Bounce":
		if ev.Bounce.BounceType != "Permanent" {
			// Transient + Undetermined: don't unsubscribe. The transient
			// case (mailbox full, greylisting) clears on its own; the
			// undetermined case is too noisy to act on.
			w.WriteHeader(http.StatusOK)
			return
		}
		for _, rec := range ev.Bounce.BouncedRecipients {
			if rec.EmailAddress == "" {
				continue
			}
			if err := insertUnsubscribe(r.Context(), h.Pool, "email", rec.EmailAddress, "ses_hard_bounce"); err != nil {
				h.Log.Warn("ses webhook: insert unsubscribe failed", "err", err, "email", rec.EmailAddress)
			} else {
				h.Log.Info("ses webhook: hard bounce recorded", "email", rec.EmailAddress)
			}
		}
	case "Complaint":
		// SES forwards spam complaints (Feedback Loop). Treat as opt-out.
		for _, rec := range ev.Complaint.ComplainedRecipients {
			if rec.EmailAddress == "" {
				continue
			}
			if err := insertUnsubscribe(r.Context(), h.Pool, "email", rec.EmailAddress, "ses_complaint"); err != nil {
				h.Log.Warn("ses webhook: insert complaint unsubscribe failed", "err", err)
			}
		}
	default:
		// "Delivery" and others: no-op.
	}
	w.WriteHeader(http.StatusOK)
}

// insertUnsubscribe upserts into the unsubscribes table. The unique
// constraint on (channel, recipient) makes this idempotent — a recipient
// who bounces twice doesn't create two rows. Reused by the Twilio STOP hook.
func insertUnsubscribe(ctx context.Context, pool *sql.DB, channel, recipient, reason string) error {
	_, err := pool.ExecContext(ctx, `
		INSERT INTO unsubscribes (id, channel, recipient, reason, created_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT (channel, recipient) DO NOTHING`,
		base62.NewID()[:22], channel, recipient, reason)
	return err
}

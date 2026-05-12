package workers

// REQ-022: daily worker that sends the trial-ending-3-days email to any
// provider whose trial_ends_at is roughly 3 days away. Idempotent via
// providers.trial_ending_email_sent_at — once we mark it, we never send
// again for that trial.

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/notify"
)

type TrialReminderWorker struct {
	Pool    *sql.DB
	Emailer *notify.Emailer
	AppBase string
	Log     *slog.Logger
}

// RunDaily runs once on start, then every 24h until ctx is canceled. Each
// pass fires two passes back-to-back — the 3-day reminder (now+2.5d..3.5d)
// and the 1-day reminder (now+0.5d..1.5d). Idempotent: the
// trial_ending_email_sent_at + trial_ending_1day_email_sent_at marker
// columns ensure a provider receives each reminder at most once.
func (w *TrialReminderWorker) RunDaily(ctx context.Context) {
	w.runOnce(ctx, time.Now().UTC())
	w.run1DayOnce(ctx, time.Now().UTC())
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			w.runOnce(ctx, now.UTC())
			w.run1DayOnce(ctx, now.UTC())
		}
	}
}

func (w *TrialReminderWorker) runOnce(ctx context.Context, now time.Time) {
	if w == nil || w.Pool == nil {
		return
	}
	log := w.Log
	if log == nil {
		log = slog.Default()
	}
	if w.Emailer == nil {
		log.Info("trial-reminder: emailer not configured; skipping")
		return
	}

	// Window: trial_ends_at falls in [now+2.5d, now+3.5d] AND we haven't
	// already sent the 3-day reminder for this trial.
	windowStart := now.Add(60 * time.Hour)  // +2.5d
	windowEnd := now.Add(84 * time.Hour)    // +3.5d

	rows, err := w.Pool.QueryContext(ctx, `
		SELECT p.id,
		       COALESCE(p.owner_email, ''),
		       COALESCE(p.name, p.legal_name, ''),
		       COALESCE((SELECT u.full_name FROM users u
		                  WHERE u.provider_id = p.id AND u.role = 'provider_admin'
		                    AND u.deleted_at IS NULL
		                  ORDER BY u.created_at ASC LIMIT 1), ''),
		       p.trial_ends_at
		  FROM providers p
		 WHERE p.deleted_at IS NULL
		   AND p.trial_ending_email_sent_at IS NULL
		   AND p.trial_ends_at IS NOT NULL
		   AND p.trial_ends_at >= ?
		   AND p.trial_ends_at <= ?`,
		windowStart.Format(time.RFC3339),
		windowEnd.Format(time.RFC3339),
	)
	if err != nil {
		log.Warn("trial-reminder: query failed", "err", err)
		return
	}
	defer rows.Close()

	type cand struct {
		id         string
		email      string
		name       string
		fullName   string
		trialEnds  string
	}
	var candidates []cand
	for rows.Next() {
		var c cand
		if err := rows.Scan(&c.id, &c.email, &c.name, &c.fullName, &c.trialEnds); err != nil {
			log.Warn("trial-reminder: scan failed", "err", err)
			continue
		}
		candidates = append(candidates, c)
	}
	_ = rows.Close()

	var sent int
	for _, c := range candidates {
		if ctx.Err() != nil {
			return
		}
		if c.email == "" {
			log.Warn("trial-reminder: provider has no owner_email", "provider_id", c.id)
			continue
		}
		trialEnds, err := time.Parse(time.RFC3339, c.trialEnds)
		if err != nil {
			log.Warn("trial-reminder: parse trial_ends_at", "provider_id", c.id, "err", err)
			continue
		}

		first := strings.TrimSpace(c.fullName)
		if first == "" {
			first = c.name
		}

		subject, html, text, err := notify.RenderTrialEnding3Days(notify.TrialEnding3DaysData{
			FirstName:    first,
			FacilityName: c.name,
			TrialEndsOn:  trialEnds.Format("Jan 2, 2006"),
			CardBrand:    "Card on file", // brand/last4 fetch from Stripe is a follow-up; template omits 'ending in' when empty
			CardLast4:    "",
			BillingURL:   strings.TrimRight(w.AppBase, "/") + "/settings/billing",
		})
		if err != nil {
			log.Warn("trial-reminder: render", "provider_id", c.id, "err", err)
			continue
		}

		sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err = w.Emailer.Send(sendCtx, notify.EmailMessage{
			To: c.email, Subject: subject, HTMLBody: html, PlainBody: text,
			ReferenceID: "trial-ending-3d:" + c.id,
		})
		cancel()
		if err != nil {
			log.Warn("trial-reminder: send", "provider_id", c.id, "err", err)
			continue
		}

		// Mark as sent ONLY after a successful send. Failed sends will be
		// retried on the next daily pass while the trial is still in the
		// window — that's acceptable; a duplicate is far less bad than a
		// missing reminder for a paying customer.
		if _, err := w.Pool.ExecContext(ctx,
			`UPDATE providers SET trial_ending_email_sent_at = ? WHERE id = ?`,
			now.Format(time.RFC3339), c.id); err != nil {
			log.Warn("trial-reminder: mark sent", "provider_id", c.id, "err", err)
		}
		sent++
	}

	log.Info("trial-reminder: pass complete", "candidates", len(candidates), "sent", sent)
}

// run1DayOnce is the REQ-010 1-day pass. Same shape as runOnce above but a
// tighter window (now+0.5d..1.5d) and a separate marker column so the
// 3-day and 1-day reminders dedup independently.
func (w *TrialReminderWorker) run1DayOnce(ctx context.Context, now time.Time) {
	if w == nil || w.Pool == nil {
		return
	}
	log := w.Log
	if log == nil {
		log = slog.Default()
	}
	if w.Emailer == nil {
		log.Info("trial-reminder-1d: emailer not configured; skipping")
		return
	}

	windowStart := now.Add(12 * time.Hour) // +0.5d
	windowEnd := now.Add(36 * time.Hour)   // +1.5d

	rows, err := w.Pool.QueryContext(ctx, `
		SELECT p.id,
		       COALESCE(p.owner_email, ''),
		       COALESCE(p.name, p.legal_name, ''),
		       COALESCE((SELECT u.full_name FROM users u
		                  WHERE u.provider_id = p.id AND u.role = 'provider_admin'
		                    AND u.deleted_at IS NULL
		                  ORDER BY u.created_at ASC LIMIT 1), ''),
		       p.trial_ends_at
		  FROM providers p
		 WHERE p.deleted_at IS NULL
		   AND p.trial_ending_1day_email_sent_at IS NULL
		   AND p.trial_ends_at IS NOT NULL
		   AND p.trial_ends_at >= ?
		   AND p.trial_ends_at <= ?`,
		windowStart.Format(time.RFC3339),
		windowEnd.Format(time.RFC3339),
	)
	if err != nil {
		log.Warn("trial-reminder-1d: query failed", "err", err)
		return
	}
	defer rows.Close()

	type cand struct {
		id        string
		email     string
		name      string
		fullName  string
		trialEnds string
	}
	var candidates []cand
	for rows.Next() {
		var c cand
		if err := rows.Scan(&c.id, &c.email, &c.name, &c.fullName, &c.trialEnds); err != nil {
			log.Warn("trial-reminder-1d: scan failed", "err", err)
			continue
		}
		candidates = append(candidates, c)
	}
	_ = rows.Close()

	var sent int
	for _, c := range candidates {
		if ctx.Err() != nil {
			return
		}
		if c.email == "" {
			log.Warn("trial-reminder-1d: provider has no owner_email", "provider_id", c.id)
			continue
		}
		trialEnds, err := time.Parse(time.RFC3339, c.trialEnds)
		if err != nil {
			log.Warn("trial-reminder-1d: parse trial_ends_at", "provider_id", c.id, "err", err)
			continue
		}

		first := strings.TrimSpace(c.fullName)
		if first == "" {
			first = c.name
		}

		subject, html, text, err := notify.RenderTrialEnding1Day(notify.TrialEnding1DayData{
			FirstName:    first,
			FacilityName: c.name,
			TrialEndsOn:  trialEnds.Format("Jan 2, 2006"),
			CardBrand:    "Card on file",
			CardLast4:    "",
			BillingURL:   strings.TrimRight(w.AppBase, "/") + "/settings/billing",
		})
		if err != nil {
			log.Warn("trial-reminder-1d: render", "provider_id", c.id, "err", err)
			continue
		}

		sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err = w.Emailer.Send(sendCtx, notify.EmailMessage{
			To: c.email, Subject: subject, HTMLBody: html, PlainBody: text,
			ReferenceID: "trial-ending-1d:" + c.id,
		})
		cancel()
		if err != nil {
			log.Warn("trial-reminder-1d: send", "provider_id", c.id, "err", err)
			continue
		}

		if _, err := w.Pool.ExecContext(ctx,
			`UPDATE providers SET trial_ending_1day_email_sent_at = ? WHERE id = ?`,
			now.Format(time.RFC3339), c.id); err != nil {
			log.Warn("trial-reminder-1d: mark sent", "provider_id", c.id, "err", err)
		}
		sent++
	}

	log.Info("trial-reminder-1d: pass complete", "candidates", len(candidates), "sent", sent)
}

package notify

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/magiclink"
)

// ChaseService scans documents for upcoming expirations and sends
// escalating reminders on a fixed schedule.
//
// Schedule thresholds (days before expiration): 90, 60, 30. After the 30-day
// reminder, weekly reminders fire indefinitely (including past expiry) at
// days 23, 16, 9, 2, -5, -12, ... Weekly buckets are encoded as negative
// integers (-1 = first weekly after 30, -2 = second, ...) so the existing
// (document_id, threshold_days, channel) PK keeps deduping cleanly.
//
// Channel selection follows preferred_contact_channel on staff/children:
// 'email' = email only, 'sms' = SMS only, 'both' = email + SMS at every fire.
//
// Dedup: at most one send per threshold per document per channel.
// Quiet hours: never send between 21:00 and 08:00 in the recipient's TZ.
type ChaseService struct {
	pool     *sql.DB
	emailer  *Emailer
	sms      *SMSSender
	magic    *magiclink.Service
	frontend string
	log      *slog.Logger
}

type ChaseDeps struct {
	Pool            *sql.DB
	Emailer         *Emailer
	SMS             *SMSSender
	Magic           *magiclink.Service
	FrontendBaseURL string
	Logger          *slog.Logger
}

func NewChaseService(d ChaseDeps) *ChaseService {
	log := d.Logger
	if log == nil {
		log = slog.Default()
	}
	return &ChaseService{
		pool:     d.Pool,
		emailer:  d.Emailer,
		sms:      d.SMS,
		magic:    d.Magic,
		frontend: d.FrontendBaseURL,
		log:      log,
	}
}

var chaseThresholds = []int{90, 60, 30}

// RunDaily loops forever, calling ProcessOnce once per 24h (with small jitter).
// Start this as a goroutine from main.
func (c *ChaseService) RunDaily(ctx context.Context) {
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	// run once on boot so a fresh deploy doesn't wait a day
	if err := c.ProcessOnce(ctx, time.Now()); err != nil {
		c.log.Error("chase: initial run failed", "err", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			if err := c.ProcessOnce(ctx, now); err != nil {
				c.log.Error("chase: run failed", "err", err)
			}
		}
	}
}

type expiringDoc struct {
	DocID              string
	ProviderID         string
	ProviderName       string
	ProviderTZ         string
	ProviderOwnerEmail string // for Reply-To on staff/parent reminders
	SubjectKind        string
	SubjectID          string
	DocTitle           string
	ExpiresAt          time.Time
	RecipEmail         string
	RecipPhone         string
	RecipName          string
	ChildFirstName     string
	ChildLastName      string
	PreferredChannel   string // 'email' | 'sms' | 'both'
}

// ProcessOnce runs a single scan/send pass. Exposed for manual triggering / tests.
func (c *ChaseService) ProcessOnce(ctx context.Context, now time.Time) error {
	if c == nil || c.pool == nil {
		return errors.New("chase: not initialized")
	}

	// The query unions child docs and staff docs with their respective
	// contacts. The window is wide on both ends:
	//   - upper bound now+95d so the 90-day bucket can fire (with slop).
	//   - no lower bound on "future expiry" — past-expiry docs still get
	//     weekly chases — but we floor at -365d so we don't chase docs that
	//     have been expired for years.
	// Pre-compute bounds in Go since SQLite has no INTERVAL literal.
	windowEnd := now.Add(95 * 24 * time.Hour)
	windowFloor := now.Add(-365 * 24 * time.Hour)
	rows, err := c.pool.QueryContext(ctx, `
WITH candidates AS (
  SELECT d.id AS doc_id, d.provider_id, d.title AS doc_title,
         d.expires_at, d.owner_kind, d.owner_id,
         p.name AS provider_name, COALESCE(p.timezone,'America/Los_Angeles') AS provider_tz,
         COALESCE(p.owner_email, '') AS provider_owner_email,
         CASE d.owner_kind
           WHEN 'child' THEN ch.parent_email
           WHEN 'staff' THEN st.email
           ELSE p.owner_email
         END AS recip_email,
         CASE d.owner_kind
           WHEN 'child' THEN ch.parent_phone
           WHEN 'staff' THEN st.phone
           ELSE p.owner_phone
         END AS recip_phone,
         CASE d.owner_kind
           WHEN 'child' THEN ch.first_name || ' ' || ch.last_name
           WHEN 'staff' THEN st.first_name || ' ' || st.last_name
           ELSE p.name
         END AS recip_name,
         CASE d.owner_kind WHEN 'child' THEN ch.first_name ELSE '' END AS child_first_name,
         CASE d.owner_kind WHEN 'child' THEN ch.last_name  ELSE '' END AS child_last_name,
         CASE d.owner_kind
           WHEN 'child' THEN ch.preferred_contact_channel
           WHEN 'staff' THEN st.preferred_contact_channel
           ELSE 'email'
         END AS preferred_channel
    FROM documents d
    JOIN providers p ON p.id = d.provider_id
    LEFT JOIN children ch ON ch.id = d.owner_id AND d.owner_kind = 'child'
    LEFT JOIN staff    st ON st.id = d.owner_id AND d.owner_kind = 'staff'
   WHERE d.deleted_at IS NULL
     AND d.expires_at IS NOT NULL
     AND d.expires_at >= ?
     AND d.expires_at <= ?
     -- REQ-006 confirmation gate: never fire on un-confirmed AI extractions.
     -- We chase the document only when:
     --   1) a human has confirmed the date (expiration_confirmed_at), OR
     --   2) the date came from a non-OCR source (no extracted_value) — i.e.
     --      a manually-entered expiration on the operator side.
     -- This is the primary defense against false reminders driven by an
     -- OCR misread; the ack flow stamps expiration_confirmed_at on the
     -- portal /confirm path and the manager dashboard /confirm path.
     AND (
       d.expiration_confirmed_at IS NOT NULL
       OR d.expiration_extracted_value IS NULL
       OR d.expiration_extracted_value = ''
     )
     AND COALESCE(d.does_not_expire, 0) = 0
)
SELECT * FROM candidates`, windowFloor, windowEnd)
	if err != nil {
		return fmt.Errorf("chase: query candidates: %w", err)
	}
	defer rows.Close()

	type row struct {
		d expiringDoc
	}
	var batch []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.d.DocID, &r.d.ProviderID, &r.d.DocTitle, &r.d.ExpiresAt,
			&r.d.SubjectKind, &r.d.SubjectID, &r.d.ProviderName, &r.d.ProviderTZ,
			&r.d.ProviderOwnerEmail,
			&r.d.RecipEmail, &r.d.RecipPhone, &r.d.RecipName, &r.d.ChildFirstName,
			&r.d.ChildLastName, &r.d.PreferredChannel); err != nil {
			return fmt.Errorf("chase: scan: %w", err)
		}
		batch = append(batch, r)
	}

	for _, r := range batch {
		days := int(r.d.ExpiresAt.Sub(now).Hours() / 24)
		threshold := matchThreshold(days)
		if threshold == 0 {
			continue
		}
		if !inBusinessHours(now, r.d.ProviderTZ) {
			continue
		}
		already, err := c.alreadySent(ctx, r.d.DocID, threshold)
		if err != nil {
			c.log.Warn("chase: dedup check failed", "err", err, "doc", r.d.DocID)
			continue
		}
		if already {
			continue
		}
		if err := c.sendOne(ctx, r.d, days); err != nil {
			c.log.Warn("chase: send failed", "err", err, "doc", r.d.DocID)
			continue
		}
		if err := c.recordSent(ctx, r.d.DocID, threshold, now); err != nil {
			c.log.Warn("chase: record failed", "err", err, "doc", r.d.DocID)
		}
	}
	return nil
}

// matchThreshold returns the bucket key to fire for a given days-to-expiry.
//
// Buckets:
//   - 90, 60: fire when days is within [t-5, t] (6-day window, covers daily
//     drift between runs).
//   - 30: fires when days is within [24, 30]. The window extends one extra
//     day low so it bridges cleanly to the weekly cadence (which starts at
//     day 23).
//   - Past day 23: weekly buckets, encoded as negative integers.
//     -1 fires at days 23..17, -2 at 16..10, -3 at 9..3, -4 at 2..-4,
//     -5 at -5..-11, etc. Negative encoding lets us reuse the existing
//     INTEGER threshold_days column and PK (document_id, threshold_days,
//     channel) without schema churn.
//
// Returns 0 when no bucket applies (no send).
func matchThreshold(days int) int {
	// Upper thresholds: 6-day window [t-5, t].
	if days >= 85 && days <= 90 {
		return 90
	}
	if days >= 55 && days <= 60 {
		return 60
	}
	// 30-day threshold extends down to 24 to bridge to weekly.
	if days >= 24 && days <= 30 {
		return 30
	}
	// Weekly buckets after the 30-day reminder. Floor division handles
	// negative days correctly: e.g. days=-5 → (30 - -5)/7 = 35/7 = 5.
	if days < 24 {
		weeks := (30 - days) / 7
		if weeks >= 1 {
			return -weeks
		}
	}
	return 0
}

func inBusinessHours(now time.Time, tz string) bool {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	local := now.In(loc)
	h := local.Hour()
	return h >= 8 && h < 21
}

func (c *ChaseService) alreadySent(ctx context.Context, docID string, threshold int) (bool, error) {
	var exists bool
	err := c.pool.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM document_chase_sends
			WHERE document_id = ? AND threshold_days = ?
		)`, docID, threshold).Scan(&exists)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	return exists, nil
}

func (c *ChaseService) recordSent(ctx context.Context, docID string, threshold int, sentAt time.Time) error {
	_, err := c.pool.ExecContext(ctx, `
		INSERT INTO document_chase_sends (document_id, threshold_days, sent_at)
		VALUES (?, ?, ?)
		ON CONFLICT (document_id, threshold_days) DO NOTHING`,
		docID, threshold, sentAt)
	if err != nil {
		return err
	}
	_, err = c.pool.ExecContext(ctx, `UPDATE documents SET last_chase_sent_at = ? WHERE id = ?`, sentAt, docID)
	return err
}

func (c *ChaseService) sendOne(ctx context.Context, d expiringDoc, days int) error {
	// Generate a fresh magic link for upload.
	var kind magiclink.Kind
	if d.SubjectKind == "child" {
		kind = magiclink.KindParentUpload
	} else {
		kind = magiclink.KindStaffUpload
	}
	token, path, err := c.magic.Generate(ctx, kind, d.ProviderID, d.SubjectID, 0)
	if err != nil {
		return fmt.Errorf("chase: generate magic: %w", err)
	}
	_ = token
	uploadURL := c.frontend + path

	who := "you"
	if d.ChildFirstName != "" {
		who = fmt.Sprintf("your child %s", d.ChildFirstName)
	}

	// Channel selection follows preferred_contact_channel. Empty/unknown
	// values default to email so a missing column or new row never goes
	// silent.
	channel := d.PreferredChannel
	if channel != "sms" && channel != "both" {
		channel = "email"
	}
	wantEmail := channel == "email" || channel == "both"
	wantSMS := channel == "sms" || channel == "both"

	if wantEmail && d.RecipEmail != "" && c.emailer != nil {
		subject, html, text, replyTo, renderErr := c.renderChaseEmail(d, days, uploadURL, who)
		if renderErr != nil {
			c.log.Warn("chase: render", "err", renderErr, "doc", d.DocID)
		} else {
			if err := c.emailer.Send(ctx, EmailMessage{
				To: d.RecipEmail, Subject: subject, HTMLBody: html, PlainBody: text,
				ReplyTo: replyTo, ReferenceID: d.DocID,
			}); err != nil {
				c.log.Warn("chase: email send", "err", err)
			}
		}
	}
	if wantSMS && d.RecipPhone != "" && c.sms != nil {
		if _, err := c.sms.Send(d.RecipPhone, SMSChaseReminder(d.DocTitle, days, uploadURL)); err != nil {
			c.log.Warn("chase: sms send", "err", err)
		}
	}
	return nil
}

// renderChaseEmail picks the right branded REQ-022 template based on
// owner_kind + (for child docs) the immunization-doc heuristic. Falls back
// to the legacy generic RenderChaseEmail for child docs that aren't
// immunization records, where the template's "your immunization record"
// copy would mis-label the email.
//
// Reply-To resolves to the provider's owner email when available — staff
// and parents who reply almost always want their director, not us.
func (c *ChaseService) renderChaseEmail(d expiringDoc, days int, uploadURL, who string) (subject, html, text, replyTo string, err error) {
	expShort := FormatExpiresShort(d.ExpiresAt)
	expLong := FormatExpiresLong(d.ExpiresAt)
	first := firstNameFromFull(d.RecipName)
	replyTo = d.ProviderOwnerEmail

	switch d.SubjectKind {
	case "staff":
		return mustReturn(RenderStaffReminder(StaffReminderData{
			FirstName:     first,
			FacilityName:  d.ProviderName,
			DocumentTitle: d.DocTitle,
			ExpiresOnLong: expLong,
			UploadURL:     uploadURL,
		}, expShort))(replyTo)

	case "child":
		if IsImmunizationDoc(d.DocTitle) {
			return mustReturn(RenderParentImmunizationReminder(ParentImmunizationReminderData{
				ChildFirstName: d.ChildFirstName,
				ChildLastName:  d.ChildLastName,
				FacilityName:   d.ProviderName,
				ExpiresOnLong:  expLong,
				UploadURL:      uploadURL,
			}, expShort))(replyTo)
		}
		// Non-immunization child doc: fall through to legacy generic render.
	}

	// Legacy fallback (provider-owned docs and non-immunization child docs).
	subj, h, t := RenderChaseEmail(ChaseEmailData{
		RecipientName:   d.RecipName,
		ChildOrStaff:    who,
		DocTitle:        d.DocTitle,
		ExpiresOn:       d.ExpiresAt.Format("Jan 2, 2006"),
		DaysUntil:       days,
		UploadURL:       uploadURL,
		ProviderName:    d.ProviderName,
		ProviderContact: "",
	})
	return subj, h, t, replyTo, nil
}

// mustReturn adapts the (subject, html, text, err) renderer return into
// renderChaseEmail's (subject, html, text, replyTo, err) signature. The
// returned closure binds the replyTo so the call site stays a single line.
func mustReturn(subject, html, text string, err error) func(replyTo string) (string, string, string, string, error) {
	return func(replyTo string) (string, string, string, string, error) {
		if err != nil {
			return "", "", "", "", err
		}
		return subject, html, text, replyTo, nil
	}
}

// firstNameFromFull splits a "First Last" string and returns the first part,
// or "there" if the input is empty.
func firstNameFromFull(s string) string {
	for i, r := range s {
		if r == ' ' {
			return s[:i]
		}
	}
	if s == "" {
		return "there"
	}
	return s
}

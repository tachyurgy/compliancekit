package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tachyurgy/compliancekit/backend/internal/magiclink"
)

// ChaseService scans documents for upcoming expirations and sends
// escalating reminders on a fixed schedule.
//
// Schedule thresholds (days before expiration): 42 (6w), 28 (4w), 14 (2w), 7 (1w), 3.
// Dedup: at most one send per threshold per document.
// Quiet hours: never send between 21:00 and 08:00 in the recipient's TZ.
type ChaseService struct {
	pool     *pgxpool.Pool
	emailer  *Emailer
	sms      *SMSSender
	magic    *magiclink.Service
	frontend string
	log      *slog.Logger
}

type ChaseDeps struct {
	Pool            *pgxpool.Pool
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

var chaseThresholds = []int{42, 28, 14, 7, 3}

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
	DocID          string
	ProviderID     string
	ProviderName   string
	ProviderTZ     string
	SubjectKind    string
	SubjectID      string
	DocTitle       string
	ExpiresAt      time.Time
	RecipEmail     string
	RecipPhone     string
	RecipName      string
	ChildFirstName string
}

// ProcessOnce runs a single scan/send pass. Exposed for manual triggering / tests.
func (c *ChaseService) ProcessOnce(ctx context.Context, now time.Time) error {
	if c == nil || c.pool == nil {
		return errors.New("chase: not initialized")
	}

	// The query unions child docs and staff docs with their respective
	// contacts, and filters by any document whose expiration falls within
	// the widest threshold window that we haven't yet sent a chase for.
	rows, err := c.pool.Query(ctx, `
WITH candidates AS (
  SELECT d.id AS doc_id, d.provider_id, d.title AS doc_title,
         d.expires_at, d.subject_kind, d.subject_id,
         p.name AS provider_name, COALESCE(p.timezone,'America/Los_Angeles') AS provider_tz,
         CASE d.subject_kind
           WHEN 'child' THEN ch.parent_email
           WHEN 'staff' THEN st.email
           ELSE p.owner_email
         END AS recip_email,
         CASE d.subject_kind
           WHEN 'child' THEN ch.parent_phone
           WHEN 'staff' THEN st.phone
           ELSE p.owner_phone
         END AS recip_phone,
         CASE d.subject_kind
           WHEN 'child' THEN ch.first_name || ' ' || ch.last_name
           WHEN 'staff' THEN st.first_name || ' ' || st.last_name
           ELSE p.name
         END AS recip_name,
         CASE d.subject_kind WHEN 'child' THEN ch.first_name ELSE '' END AS child_first_name
    FROM documents d
    JOIN providers p ON p.id = d.provider_id
    LEFT JOIN children ch ON ch.id = d.subject_id AND d.subject_kind = 'child'
    LEFT JOIN staff    st ON st.id = d.subject_id AND d.subject_kind = 'staff'
   WHERE d.deleted_at IS NULL
     AND d.expires_at IS NOT NULL
     AND d.expires_at > $1
     AND d.expires_at <= $1 + INTERVAL '45 days'
)
SELECT * FROM candidates`, now)
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
			&r.d.RecipEmail, &r.d.RecipPhone, &r.d.RecipName, &r.d.ChildFirstName); err != nil {
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

// matchThreshold returns the threshold bucket to fire for a given days-to-expiry.
// Thresholds (days): 42, 28, 14, 7, 3. Any doc within 5 days of a threshold
// falls into that bucket (covers daily drift between scheduler runs).
func matchThreshold(days int) int {
	if days < 0 {
		return 0
	}
	for _, t := range chaseThresholds {
		if days <= t && days > t-5 {
			return t
		}
	}
	if days <= 3 {
		return 3
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
	err := c.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM document_chase_sends
			WHERE document_id = $1 AND threshold_days = $2
		)`, docID, threshold).Scan(&exists)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	return exists, nil
}

func (c *ChaseService) recordSent(ctx context.Context, docID string, threshold int, sentAt time.Time) error {
	_, err := c.pool.Exec(ctx, `
		INSERT INTO document_chase_sends (document_id, threshold_days, sent_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (document_id, threshold_days) DO NOTHING`,
		docID, threshold, sentAt)
	if err != nil {
		return err
	}
	_, err = c.pool.Exec(ctx, `UPDATE documents SET last_chase_sent_at = $2 WHERE id = $1`, docID, sentAt)
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

	if d.RecipEmail != "" && c.emailer != nil {
		subject, html, text := RenderChaseEmail(ChaseEmailData{
			RecipientName:   d.RecipName,
			ChildOrStaff:    who,
			DocTitle:        d.DocTitle,
			ExpiresOn:       d.ExpiresAt.Format("Jan 2, 2006"),
			DaysUntil:       days,
			UploadURL:       uploadURL,
			ProviderName:    d.ProviderName,
			ProviderContact: "",
		})
		if err := c.emailer.Send(ctx, EmailMessage{
			To: d.RecipEmail, Subject: subject, HTMLBody: html, PlainBody: text, ReferenceID: d.DocID,
		}); err != nil {
			c.log.Warn("chase: email send", "err", err)
		}
	}
	if d.RecipPhone != "" && c.sms != nil && days <= 7 {
		if _, err := c.sms.Send(d.RecipPhone, SMSChaseReminder(d.DocTitle, days, uploadURL)); err != nil {
			c.log.Warn("chase: sms send", "err", err)
		}
	}
	return nil
}

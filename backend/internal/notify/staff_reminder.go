package notify

// REQ-022: branded staff document-expiry reminder. Source-of-truth template
// lives at active/product/email-templates/staff-reminder.{html,txt}; the
// embedded copy here is the build-time snapshot.

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	texttemplate "text/template"
	"time"
)

//go:embed templates/staff-reminder.html
var staffReminderHTML string

//go:embed templates/staff-reminder.txt
var staffReminderTXT string

// StaffReminderData fills the staff-reminder template's placeholders.
//
// FirstName       — staff member's first name (or "there" if unknown).
// FacilityName    — provider display name.
// DocumentTitle   — e.g. "CPR & First Aid certificate".
// ExpiresOnLong   — long-form date in body, e.g. "Saturday, June 7, 2026".
// UploadURL       — staff magic-link upload URL.
type StaffReminderData struct {
	FirstName     string
	FacilityName  string
	DocumentTitle string
	ExpiresOnLong string
	UploadURL     string
}

// RenderStaffReminder renders the staff-reminder email.
//
// The subject uses the short-form date format ("Sat, Jun 7") computed by the
// caller and passed in as expiresOnShort, since the template doesn't carry
// the original time.Time.
func RenderStaffReminder(d StaffReminderData, expiresOnShort string) (subject, html, text string, err error) {
	subject = fmt.Sprintf("Your %s expires %s", d.DocumentTitle, expiresOnShort)

	htmlT, err := template.New("staff-reminder.html").Parse(staffReminderHTML)
	if err != nil {
		return "", "", "", fmt.Errorf("notify: parse staff-reminder.html: %w", err)
	}
	var hb bytes.Buffer
	if err := htmlT.Execute(&hb, d); err != nil {
		return "", "", "", fmt.Errorf("notify: render staff-reminder.html: %w", err)
	}

	txtT, err := texttemplate.New("staff-reminder.txt").Parse(staffReminderTXT)
	if err != nil {
		return "", "", "", fmt.Errorf("notify: parse staff-reminder.txt: %w", err)
	}
	var tb bytes.Buffer
	if err := txtT.Execute(&tb, d); err != nil {
		return "", "", "", fmt.Errorf("notify: render staff-reminder.txt: %w", err)
	}
	return subject, hb.String(), tb.String(), nil
}

// FormatExpiresShort renders an expiration date for a subject line: "Sat, Jun 7".
func FormatExpiresShort(t time.Time) string {
	return t.Format("Mon, Jan 2")
}

// FormatExpiresLong renders an expiration date for the email body:
// "Saturday, June 7, 2026".
func FormatExpiresLong(t time.Time) string {
	return t.Format("Monday, January 2, 2006")
}

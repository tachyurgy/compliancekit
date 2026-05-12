package notify

// REQ-010: render the YC-tier email-verification email sent right after
// signup. One-time token, 24h expiry. Mirrors the password-reset pattern.
//
// The source-of-truth template lives at active/product/email-templates/.
// We keep a build-time copy under templates/email-verification.{html,txt}
// and //go:embed it.

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	texttemplate "text/template"
)

//go:embed templates/email-verification.html
var emailVerificationHTML string

//go:embed templates/email-verification.txt
var emailVerificationTXT string

// EmailVerificationExpiryHours is the TTL the body advertises. Must match
// the duration the handler actually persists on the email_verifications
// row — keep them in sync if you change one. 24h is the REQ-010 default.
const EmailVerificationExpiryHours = 24

// EmailVerificationData fills the email-verification template's placeholders.
//
// FirstName       — the operator's first name; falls back to "there" when empty.
// FacilityName    — the daycare's display name; falls back to "your account".
// VerifyURL       — absolute confirm URL, e.g.
//                   https://app.levelbrook.com/verify-email/<raw-token>.
// ExpiresInHours  — surfaces the TTL in the body copy. Defaults to
//                   EmailVerificationExpiryHours when zero.
type EmailVerificationData struct {
	FirstName      string
	FacilityName   string
	VerifyURL      string
	ExpiresInHours int
}

// RenderEmailVerification returns (subject, htmlBody, plainBody). Subject is
// fixed — no template needed.
func RenderEmailVerification(d EmailVerificationData) (subject, html, text string, err error) {
	if d.ExpiresInHours == 0 {
		d.ExpiresInHours = EmailVerificationExpiryHours
	}
	if d.FirstName == "" {
		d.FirstName = "there"
	} else {
		d.FirstName = firstNameOnly(d.FirstName)
	}
	if d.FacilityName == "" {
		d.FacilityName = "your account"
	}

	subject = "Confirm your Levelbrook email"

	htmlT, err := template.New("email-verification.html").Parse(emailVerificationHTML)
	if err != nil {
		return "", "", "", fmt.Errorf("notify: parse email-verification.html: %w", err)
	}
	var hb bytes.Buffer
	if err := htmlT.Execute(&hb, d); err != nil {
		return "", "", "", fmt.Errorf("notify: render email-verification.html: %w", err)
	}

	txtT, err := texttemplate.New("email-verification.txt").Parse(emailVerificationTXT)
	if err != nil {
		return "", "", "", fmt.Errorf("notify: parse email-verification.txt: %w", err)
	}
	var tb bytes.Buffer
	if err := txtT.Execute(&tb, d); err != nil {
		return "", "", "", fmt.Errorf("notify: render email-verification.txt: %w", err)
	}
	return subject, hb.String(), tb.String(), nil
}

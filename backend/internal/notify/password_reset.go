package notify

// REQ-034: render the YC-tier password-reset email when a user hits
// /api/auth/forgot-password. Replaces the inline fmt.Sprintf("<html>…")
// block that lived in handlers/auth.go.
//
// The source-of-truth template lives at active/product/email-templates/.
// We keep a build-time copy under templates/password-reset.{html,txt} and
// //go:embed it. To keep them in sync, infra/scripts/sync-email-templates.sh
// (or a Makefile target) can be added later — for the launch we manually copy.

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	texttemplate "text/template"
)

//go:embed templates/password-reset.html
var passwordResetHTML string

//go:embed templates/password-reset.txt
var passwordResetTXT string

// PasswordResetExpiryMinutes is the TTL the email surfaces in its body. It
// must match the duration the handler actually persists on the
// password_resets row — keep them in sync if you change one. 30 minutes is
// the REQ-034 default; long-lived reset tokens are a phishing amplifier.
const PasswordResetExpiryMinutes = 30

// PasswordResetData fills the password-reset template's named placeholders.
//
// FirstName        — the user's first name; falls back to "there" when empty
//
//	(handled by firstNameOnly).
//
// ResetURL         — the absolute URL the recipient clicks to set a new
//
//	password, e.g. https://app.levelbrook.com/reset-password/<raw-token>.
//
// ExpiresInMinutes — surfaces the TTL in the body copy. Defaults to
//
//	PasswordResetExpiryMinutes when zero.
type PasswordResetData struct {
	FirstName        string
	ResetURL         string
	ExpiresInMinutes int
}

// RenderPasswordReset returns (subject, htmlBody, plainBody). Subject is
// fixed — no template needed. The body uses html/template + text/template
// so the FirstName is escaped automatically (the old inline path did this
// by hand via htmlSafe).
func RenderPasswordReset(d PasswordResetData) (subject, html, text string, err error) {
	if d.ExpiresInMinutes == 0 {
		d.ExpiresInMinutes = PasswordResetExpiryMinutes
	}
	if d.FirstName == "" {
		d.FirstName = "there"
	} else {
		d.FirstName = firstNameOnly(d.FirstName)
	}

	subject = "Reset your Levelbrook password"

	htmlT, err := template.New("password-reset.html").Parse(passwordResetHTML)
	if err != nil {
		return "", "", "", fmt.Errorf("notify: parse password-reset.html: %w", err)
	}
	var hb bytes.Buffer
	if err := htmlT.Execute(&hb, d); err != nil {
		return "", "", "", fmt.Errorf("notify: render password-reset.html: %w", err)
	}

	txtT, err := texttemplate.New("password-reset.txt").Parse(passwordResetTXT)
	if err != nil {
		return "", "", "", fmt.Errorf("notify: parse password-reset.txt: %w", err)
	}
	var tb bytes.Buffer
	if err := txtT.Execute(&tb, d); err != nil {
		return "", "", "", fmt.Errorf("notify: render password-reset.txt: %w", err)
	}
	return subject, hb.String(), tb.String(), nil
}

package notify

// REQ-002: send the YC-tier welcome email after Stripe checkout completes.
//
// The source-of-truth template lives at active/product/email-templates/.
// We keep a build-time copy under templates/welcome.{html,txt} and
// //go:embed it. To keep them in sync, infra/scripts/sync-email-templates.sh
// (or a Makefile target) can be added later — for the launch we manually copy.

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	texttemplate "text/template"
	"time"
)

//go:embed templates/welcome.html
var welcomeHTML string

//go:embed templates/welcome.txt
var welcomeTXT string

// WelcomeData fills the welcome template's named placeholders.
//
// FirstName       — the operator's first name (or full_name if first isn't known).
// FacilityName    — the daycare's display name.
// AppURL          — the app base URL with no trailing slash, e.g. https://app.levelbrook.com
// TrialEndsOn     — formatted date like "May 23, 2026".
type WelcomeData struct {
	FirstName    string
	FacilityName string
	AppURL       string
	TrialEndsOn  string
}

// RenderWelcome returns (subject, htmlBody, plainBody). Subject is fixed —
// the operator-facing copy lives in the body.
func RenderWelcome(d WelcomeData) (subject, html, text string, err error) {
	subject = fmt.Sprintf("Welcome to Levelbrook, %s!", firstNameOnly(d.FirstName))

	htmlT, err := template.New("welcome.html").Parse(welcomeHTML)
	if err != nil {
		return "", "", "", fmt.Errorf("notify: parse welcome.html: %w", err)
	}
	var hb bytes.Buffer
	if err := htmlT.Execute(&hb, d); err != nil {
		return "", "", "", fmt.Errorf("notify: render welcome.html: %w", err)
	}

	txtT, err := texttemplate.New("welcome.txt").Parse(welcomeTXT)
	if err != nil {
		return "", "", "", fmt.Errorf("notify: parse welcome.txt: %w", err)
	}
	var tb bytes.Buffer
	if err := txtT.Execute(&tb, d); err != nil {
		return "", "", "", fmt.Errorf("notify: render welcome.txt: %w", err)
	}
	return subject, hb.String(), tb.String(), nil
}

// FormatTrialEnds is a tiny helper so callers don't have to remember the
// "Sat May 23" format. We render the long-form date the welcome email uses.
func FormatTrialEnds(t time.Time) string {
	return t.Format("Mon Jan 2, 2006")
}

func firstNameOnly(s string) string {
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

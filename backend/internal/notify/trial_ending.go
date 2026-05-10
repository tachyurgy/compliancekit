package notify

// REQ-022: branded "your trial ends in 3 days" email driven by a daily
// worker that scans providers whose trial_ends_at is in ~3 days.

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	texttemplate "text/template"
)

//go:embed templates/trial-ending-3-days.html
var trialEnding3DaysHTML string

//go:embed templates/trial-ending-3-days.txt
var trialEnding3DaysTXT string

// TrialEnding3DaysData fills the trial-ending-3-days template's placeholders.
//
// TrialEndsOn   — formatted date like "May 24, 2026".
// CardBrand     — e.g. "Visa".
// CardLast4     — e.g. "4242".
// BillingURL    — Stripe Billing Portal session URL (15-min TTL).
type TrialEnding3DaysData struct {
	FirstName    string
	FacilityName string
	TrialEndsOn  string
	CardBrand    string
	CardLast4    string
	BillingURL   string
}

// RenderTrialEnding3Days renders the trial-ending-3-days email.
func RenderTrialEnding3Days(d TrialEnding3DaysData) (subject, html, text string, err error) {
	subject = "Your Levelbrook trial ends in 3 days"

	htmlT, err := template.New("trial-ending-3-days.html").Parse(trialEnding3DaysHTML)
	if err != nil {
		return "", "", "", fmt.Errorf("notify: parse trial-ending-3-days.html: %w", err)
	}
	var hb bytes.Buffer
	if err := htmlT.Execute(&hb, d); err != nil {
		return "", "", "", fmt.Errorf("notify: render trial-ending-3-days.html: %w", err)
	}

	txtT, err := texttemplate.New("trial-ending-3-days.txt").Parse(trialEnding3DaysTXT)
	if err != nil {
		return "", "", "", fmt.Errorf("notify: parse trial-ending-3-days.txt: %w", err)
	}
	var tb bytes.Buffer
	if err := txtT.Execute(&tb, d); err != nil {
		return "", "", "", fmt.Errorf("notify: render trial-ending-3-days.txt: %w", err)
	}
	return subject, hb.String(), tb.String(), nil
}

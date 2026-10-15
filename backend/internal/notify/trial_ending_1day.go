package notify

// REQ-010: branded "1 day left on your trial" companion to the existing
// trial-ending-3-days reminder. Fired by the same daily worker on the
// trial day-13 pass (provider's trial_ends_at is between now+0.5d and
// now+1.5d AND the 1-day marker column is NULL).

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	texttemplate "text/template"
)

//go:embed templates/trial-ending-1-day.html
var trialEnding1DayHTML string

//go:embed templates/trial-ending-1-day.txt
var trialEnding1DayTXT string

// TrialEnding1DayData fills the trial-ending-1-day template's placeholders.
// Shape matches TrialEnding3DaysData so the worker can reuse the same
// candidate-row scan code.
//
// TrialEndsOn   — formatted date like "May 26, 2026".
// CardBrand     — e.g. "Visa".  When CardLast4 is empty, the body omits
//
//	the "ending in" phrase (template uses an {{if}} guard).
//
// CardLast4     — e.g. "4242".
// BillingURL    — Stripe Billing Portal session URL.
type TrialEnding1DayData struct {
	FirstName    string
	FacilityName string
	TrialEndsOn  string
	CardBrand    string
	CardLast4    string
	BillingURL   string
}

// RenderTrialEnding1Day renders the trial-ending-1-day email.
func RenderTrialEnding1Day(d TrialEnding1DayData) (subject, html, text string, err error) {
	if d.FirstName == "" {
		d.FirstName = "there"
	} else {
		d.FirstName = firstNameOnly(d.FirstName)
	}
	if d.FacilityName == "" {
		d.FacilityName = "your account"
	}
	if d.CardBrand == "" {
		d.CardBrand = "Card on file"
	}

	subject = "1 day left on your Levelbrook trial"

	htmlT, err := template.New("trial-ending-1-day.html").Parse(trialEnding1DayHTML)
	if err != nil {
		return "", "", "", fmt.Errorf("notify: parse trial-ending-1-day.html: %w", err)
	}
	var hb bytes.Buffer
	if err := htmlT.Execute(&hb, d); err != nil {
		return "", "", "", fmt.Errorf("notify: render trial-ending-1-day.html: %w", err)
	}

	txtT, err := texttemplate.New("trial-ending-1-day.txt").Parse(trialEnding1DayTXT)
	if err != nil {
		return "", "", "", fmt.Errorf("notify: parse trial-ending-1-day.txt: %w", err)
	}
	var tb bytes.Buffer
	if err := txtT.Execute(&tb, d); err != nil {
		return "", "", "", fmt.Errorf("notify: render trial-ending-1-day.txt: %w", err)
	}
	return subject, hb.String(), tb.String(), nil
}

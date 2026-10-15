package notify

// REQ-010: branded trial-converted email — sent after the first
// successful charge on the operator's subscription (Stripe webhook
// invoice.payment_succeeded with billing_reason = subscription_cycle,
// or the customer.subscription.updated event flipping status from
// `trialing` to `active`).

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	texttemplate "text/template"
)

//go:embed templates/trial-converted.html
var trialConvertedHTML string

//go:embed templates/trial-converted.txt
var trialConvertedTXT string

// TrialConvertedData fills the trial-converted template's placeholders.
//
// FirstName       — operator's first name; falls back to "there".
// FacilityName    — daycare display name; falls back to "your account".
// PlanName        — human-readable plan ("Monthly", "Annual"); the body
//
//	prefixes "Levelbrook Professional" — the meta row
//	shows just this label. Falls back to "Levelbrook
//	Professional Monthly".
//
// AmountCharged   — pre-formatted dollar string like "99.00" (no $ sign;
//
//	template renders the dollar sign).
//
// NextChargeOn    — formatted date like "Jun 26, 2026". Falls back to a
//
//	helpful placeholder so the email is never blank.
//
// AppURL          — app base URL with no trailing slash.
type TrialConvertedData struct {
	FirstName     string
	FacilityName  string
	PlanName      string
	AmountCharged string
	NextChargeOn  string
	AppURL        string
}

// RenderTrialConverted renders the trial-converted email.
func RenderTrialConverted(d TrialConvertedData) (subject, html, text string, err error) {
	if d.FirstName == "" {
		d.FirstName = "there"
	} else {
		d.FirstName = firstNameOnly(d.FirstName)
	}
	if d.FacilityName == "" {
		d.FacilityName = "your account"
	}
	if d.PlanName == "" {
		d.PlanName = "Levelbrook Professional Monthly"
	}
	if d.NextChargeOn == "" {
		d.NextChargeOn = "next billing cycle"
	}
	if d.AmountCharged == "" {
		d.AmountCharged = "99.00"
	}

	subject = "Welcome to Levelbrook Professional"

	htmlT, err := template.New("trial-converted.html").Parse(trialConvertedHTML)
	if err != nil {
		return "", "", "", fmt.Errorf("notify: parse trial-converted.html: %w", err)
	}
	var hb bytes.Buffer
	if err := htmlT.Execute(&hb, d); err != nil {
		return "", "", "", fmt.Errorf("notify: render trial-converted.html: %w", err)
	}

	txtT, err := texttemplate.New("trial-converted.txt").Parse(trialConvertedTXT)
	if err != nil {
		return "", "", "", fmt.Errorf("notify: parse trial-converted.txt: %w", err)
	}
	var tb bytes.Buffer
	if err := txtT.Execute(&tb, d); err != nil {
		return "", "", "", fmt.Errorf("notify: render trial-converted.txt: %w", err)
	}
	return subject, hb.String(), tb.String(), nil
}

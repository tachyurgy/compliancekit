package notify

// REQ-022: branded payment-failed email triggered by Stripe's
// invoice.payment_failed webhook.

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	texttemplate "text/template"
)

//go:embed templates/payment-failed.html
var paymentFailedHTML string

//go:embed templates/payment-failed.txt
var paymentFailedTXT string

// PaymentFailedData fills the payment-failed template's placeholders.
//
// AmountDue is a pre-formatted dollar string like "99.00" (no $ sign — the
// template renders the dollar sign in the body).
// DeclineReason is the human-readable string from Stripe (or our fallback);
// never an internal error code like "card_declined".
// RetryDeadline is formatted like "May 17, 2026".
type PaymentFailedData struct {
	FirstName        string
	FacilityName     string
	CardBrand        string
	CardLast4        string
	AmountDue        string
	DeclineReason    string
	RetryDeadline    string
	UpdatePaymentURL string
}

// RenderPaymentFailed renders the payment-failed email.
func RenderPaymentFailed(d PaymentFailedData) (subject, html, text string, err error) {
	subject = "Your Levelbrook payment didn't go through"

	htmlT, err := template.New("payment-failed.html").Parse(paymentFailedHTML)
	if err != nil {
		return "", "", "", fmt.Errorf("notify: parse payment-failed.html: %w", err)
	}
	var hb bytes.Buffer
	if err := htmlT.Execute(&hb, d); err != nil {
		return "", "", "", fmt.Errorf("notify: render payment-failed.html: %w", err)
	}

	txtT, err := texttemplate.New("payment-failed.txt").Parse(paymentFailedTXT)
	if err != nil {
		return "", "", "", fmt.Errorf("notify: parse payment-failed.txt: %w", err)
	}
	var tb bytes.Buffer
	if err := txtT.Execute(&tb, d); err != nil {
		return "", "", "", fmt.Errorf("notify: render payment-failed.txt: %w", err)
	}
	return subject, hb.String(), tb.String(), nil
}

// FormatAmount converts a Stripe cents amount into the display string the
// template expects ("99.00").
func FormatAmount(cents int64) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

// DefaultDeclineReason is the safe fallback when Stripe doesn't give us a
// human-readable reason. We never expose internal error codes.
const DefaultDeclineReason = "Your card was declined. Please contact your bank for more information."

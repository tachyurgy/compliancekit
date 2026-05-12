package notify

import (
	"strings"
	"testing"
)

func TestRenderTrialEnding1Day_HappyPath(t *testing.T) {
	t.Parallel()
	subject, html, text, err := RenderTrialEnding1Day(TrialEnding1DayData{
		FirstName:    "Linda Chen",
		FacilityName: "Sunny Days Learning Center",
		TrialEndsOn:  "May 26, 2026",
		CardBrand:    "Visa",
		CardLast4:    "4242",
		BillingURL:   "https://app.levelbrook.com/settings/billing?session=abc",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if subject != "1 day left on your Levelbrook trial" {
		t.Fatalf("subject = %q", subject)
	}
	for _, want := range []string{
		"Linda", // first name only
		"Sunny Days Learning Center",
		"May 26, 2026",
		"Visa",
		"4242",
		"ending in 4242",
		"https://app.levelbrook.com/settings/billing?session=abc",
		"levelbrookteam@gmail.com",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML body missing %q", want)
		}
		if !strings.Contains(text, want) {
			t.Errorf("text body missing %q", want)
		}
	}
	if strings.Contains(html, "Linda Chen") {
		t.Errorf("HTML rendered full name; expected first-name-only")
	}
}

// TestRenderTrialEnding1Day_OmitsLast4WhenEmpty: when we don't have card
// brand/last4 from Stripe yet (the invoice.payment_failed event doesn't
// carry it), the body should still read clean.
func TestRenderTrialEnding1Day_OmitsLast4WhenEmpty(t *testing.T) {
	t.Parallel()
	_, html, text, err := RenderTrialEnding1Day(TrialEnding1DayData{
		FirstName:    "Linda",
		FacilityName: "Sunny Days",
		TrialEndsOn:  "May 26, 2026",
		BillingURL:   "https://app.levelbrook.com/settings/billing",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(html, "ending in") {
		t.Errorf("HTML should omit 'ending in' when CardLast4 is empty")
	}
	if strings.Contains(text, "ending in") {
		t.Errorf("text should omit 'ending in' when CardLast4 is empty")
	}
	if !strings.Contains(html, "Card on file") {
		t.Errorf("HTML missing default CardBrand 'Card on file'")
	}
}

func TestRenderTrialEnding1Day_HTMLEscapes(t *testing.T) {
	t.Parallel()
	_, html, _, err := RenderTrialEnding1Day(TrialEnding1DayData{
		FirstName:    "<script>x</script>",
		FacilityName: "<b>boom</b>",
		BillingURL:   "https://app.levelbrook.com/settings/billing",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(html, "<script>") {
		t.Fatalf("unescaped <script> in HTML output")
	}
	if strings.Contains(html, "<b>boom</b>") {
		t.Fatalf("unescaped <b> in HTML output")
	}
}

func TestRenderTrialConverted_HappyPath(t *testing.T) {
	t.Parallel()
	subject, html, text, err := RenderTrialConverted(TrialConvertedData{
		FirstName:     "Linda Chen",
		FacilityName:  "Sunny Days Learning Center",
		PlanName:      "Annual",
		AmountCharged: "948.00",
		NextChargeOn:  "May 26, 2027",
		AppURL:        "https://app.levelbrook.com",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if subject != "Welcome to Levelbrook Professional" {
		t.Fatalf("subject = %q", subject)
	}
	for _, want := range []string{
		"Linda", // first name only
		"Sunny Days Learning Center",
		"$948.00",
		"Annual",
		"May 26, 2027",
		"https://app.levelbrook.com",
		"levelbrookteam@gmail.com",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML body missing %q", want)
		}
		if !strings.Contains(text, want) {
			t.Errorf("text body missing %q", want)
		}
	}
	if strings.Contains(html, "Linda Chen") {
		t.Errorf("HTML rendered full name; expected first-name-only")
	}
}

func TestRenderTrialConverted_HTMLEscapes(t *testing.T) {
	t.Parallel()
	_, html, _, err := RenderTrialConverted(TrialConvertedData{
		FirstName:     "<script>x</script>",
		FacilityName:  "<b>boom</b>",
		PlanName:      "<i>p</i>",
		AmountCharged: "99.00",
		AppURL:        "https://app.levelbrook.com",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, bad := range []string{"<script>", "<b>boom</b>", "<i>p</i>"} {
		if strings.Contains(html, bad) {
			t.Fatalf("unescaped %q in HTML output", bad)
		}
	}
}

func TestRenderTrialConverted_Defaults(t *testing.T) {
	t.Parallel()
	_, html, text, err := RenderTrialConverted(TrialConvertedData{
		AppURL: "https://app.levelbrook.com",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{
		"there",
		"Levelbrook Professional Monthly",
		"$99.00",
		"next billing cycle",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML missing default %q", want)
		}
		if !strings.Contains(text, want) {
			t.Errorf("text missing default %q", want)
		}
	}
}

package notify

// REQ-022 unit tests — pure-function coverage for the four branded
// renderers, the immunization-doc heuristic, and the formatting helpers.
// DB-backed integration coverage of the chase dispatch + trial-ending
// worker lives in test/integration/ (REQ-022 follow-up).

import (
	"strings"
	"testing"
	"time"
)

func TestRenderStaffReminder_substitutesAllFields(t *testing.T) {
	t.Parallel()
	d := StaffReminderData{
		FirstName:     "Maria",
		FacilityName:  "Sunshine Pines Learning Center",
		DocumentTitle: "CPR & First Aid certificate",
		ExpiresOnLong: "Saturday, June 7, 2026",
		UploadURL:     "https://app.levelbrook.com/u/3xK9mQp2vR7N",
	}
	subject, html, text, err := RenderStaffReminder(d, "Sat, Jun 7")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	wantSubject := "Your CPR & First Aid certificate expires Sat, Jun 7"
	if subject != wantSubject {
		t.Errorf("subject mismatch:\n  got:  %q\n  want: %q", subject, wantSubject)
	}
	for _, frag := range []string{
		"Hi Maria",
		"Sunshine Pines Learning Center",
		"CPR &amp; First Aid certificate", // html/template auto-escapes
		"Saturday, June 7, 2026",
		"https://app.levelbrook.com/u/3xK9mQp2vR7N",
	} {
		if !strings.Contains(html, frag) {
			t.Errorf("html missing %q", frag)
		}
	}
	for _, frag := range []string{
		"Maria",
		"CPR & First Aid certificate", // plain text — no escaping
		"Saturday, June 7, 2026",
		"https://app.levelbrook.com/u/3xK9mQp2vR7N",
	} {
		if !strings.Contains(text, frag) {
			t.Errorf("text missing %q", frag)
		}
	}
}

func TestRenderParentImmunizationReminder_substitutesAllFields(t *testing.T) {
	t.Parallel()
	d := ParentImmunizationReminderData{
		ChildFirstName: "Logan",
		ChildLastName:  "Hernandez",
		FacilityName:   "Sunshine Pines Learning Center",
		ExpiresOnLong:  "Monday, July 12, 2026",
		UploadURL:      "https://app.levelbrook.com/u/8zP4vNb1tW3Q",
	}
	subject, html, text, err := RenderParentImmunizationReminder(d, "Mon, Jul 12")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if got, want := subject, "Logan's immunization record expires Mon, Jul 12"; got != want {
		t.Errorf("subject mismatch:\n  got:  %q\n  want: %q", got, want)
	}
	for _, frag := range []string{"Logan", "Hernandez", "Sunshine Pines", "Monday, July 12, 2026", "8zP4vNb1tW3Q"} {
		if !strings.Contains(html, frag) {
			t.Errorf("html missing %q", frag)
		}
		if !strings.Contains(text, frag) {
			t.Errorf("text missing %q", frag)
		}
	}
}

func TestRenderPaymentFailed_substitutesAllFields_andSuppressesLast4WhenEmpty(t *testing.T) {
	t.Parallel()
	d := PaymentFailedData{
		FirstName:        "Sarah",
		FacilityName:     "Sunshine Pines Learning Center",
		CardBrand:        "Visa",
		CardLast4:        "4242",
		AmountDue:        "99.00",
		DeclineReason:    "Your card was declined.",
		RetryDeadline:    "May 17, 2026",
		UpdatePaymentURL: "https://app.levelbrook.com/settings/billing",
	}
	subject, html, text, err := RenderPaymentFailed(d)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if subject != "Your Levelbrook payment didn't go through" {
		t.Errorf("subject = %q", subject)
	}
	for _, frag := range []string{
		"Hi Sarah",
		"Sunshine Pines Learning Center",
		"Visa ending in 4242",
		"$99.00",
		"Your card was declined",
		"May 17, 2026",
		"https://app.levelbrook.com/settings/billing",
	} {
		if !strings.Contains(html, frag) {
			t.Errorf("html missing %q", frag)
		}
	}
	for _, frag := range []string{"Sarah", "Visa ending in 4242", "$99.00"} {
		if !strings.Contains(text, frag) {
			t.Errorf("text missing %q", frag)
		}
	}

	// Empty CardLast4: the template must NOT render the literal " ending in ".
	d2 := d
	d2.CardLast4 = ""
	d2.CardBrand = "Card on file"
	_, html2, text2, err := RenderPaymentFailed(d2)
	if err != nil {
		t.Fatalf("render2: %v", err)
	}
	if strings.Contains(html2, "ending in ") {
		t.Errorf("html should suppress 'ending in' when CardLast4 is empty")
	}
	if strings.Contains(text2, "ending in ") {
		t.Errorf("text should suppress 'ending in' when CardLast4 is empty")
	}
	if !strings.Contains(html2, "Card on file") {
		t.Errorf("html missing 'Card on file'")
	}
}

func TestRenderTrialEnding3Days_substitutesAllFields(t *testing.T) {
	t.Parallel()
	d := TrialEnding3DaysData{
		FirstName:    "Sarah",
		FacilityName: "Sunshine Pines Learning Center",
		TrialEndsOn:  "May 24, 2026",
		CardBrand:    "Visa",
		CardLast4:    "4242",
		BillingURL:   "https://app.levelbrook.com/settings/billing",
	}
	subject, html, text, err := RenderTrialEnding3Days(d)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if subject != "Your Levelbrook trial ends in 3 days" {
		t.Errorf("subject = %q", subject)
	}
	for _, frag := range []string{
		"Hi Sarah",
		"Sunshine Pines Learning Center",
		"May 24, 2026",
		"Visa ending in 4242",
		"https://app.levelbrook.com/settings/billing",
	} {
		if !strings.Contains(html, frag) {
			t.Errorf("html missing %q", frag)
		}
		if !strings.Contains(text, frag) {
			t.Errorf("text missing %q", frag)
		}
	}
}

func TestIsImmunizationDoc_matchesFLLaunchVocabulary(t *testing.T) {
	t.Parallel()
	cases := []struct {
		title string
		want  bool
	}{
		// Positives.
		{"DH 680 Immunization", true},
		{"DH-680", true},
		{"DH680 form", true},
		{"Immunization record", true},
		{"immunization record (renewal)", true},
		{"Shot Record", true},
		{"Vaccination card", true},
		{"vaccinations - 2026", true},
		// Negatives.
		{"Custody paperwork", false},
		{"Income verification", false},
		{"Birth certificate", false},
		{"DH 3040 Student Health Exam", false}, // not immunization-shaped
		{"", false},
	}
	for _, c := range cases {
		got := IsImmunizationDoc(c.title)
		if got != c.want {
			t.Errorf("IsImmunizationDoc(%q) = %v, want %v", c.title, got, c.want)
		}
	}
}

func TestFormatExpires_shortAndLong(t *testing.T) {
	t.Parallel()
	// 2026-06-06 is a Saturday.
	d := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	if got, want := FormatExpiresShort(d), "Sat, Jun 6"; got != want {
		t.Errorf("short = %q, want %q", got, want)
	}
	if got, want := FormatExpiresLong(d), "Saturday, June 6, 2026"; got != want {
		t.Errorf("long = %q, want %q", got, want)
	}
}

func TestFormatAmount_centsToDollarsString(t *testing.T) {
	t.Parallel()
	cases := []struct {
		cents int64
		want  string
	}{
		{9900, "99.00"},
		{0, "0.00"},
		{1, "0.01"},
		{99, "0.99"},
		{12345, "123.45"},
		{100000, "1000.00"},
	}
	for _, c := range cases {
		if got := FormatAmount(c.cents); got != c.want {
			t.Errorf("FormatAmount(%d) = %q, want %q", c.cents, got, c.want)
		}
	}
}

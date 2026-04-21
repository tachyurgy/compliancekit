package notify

import (
	"testing"
	"time"
)

// matchThreshold + inBusinessHours are package-private; tests here live in the
// same package so we can exercise them directly without exposing internals.

func TestMatchThreshold_BucketsByDaysUntilExpiry(t *testing.T) {
	t.Parallel()
	cases := []struct {
		days int
		want int
	}{
		{50, 0}, // outside widest threshold window
		{42, 42},
		{41, 42},
		{38, 42},
		{37, 0}, // past 42 − 5 slop, below next threshold
		{28, 28},
		{25, 28},
		{24, 28}, // 28 - 5 + 1 = 24 (inclusive)
		{23, 0},
		{14, 14},
		{11, 14},
		{10, 14},
		{9, 0},
		{7, 7},
		{4, 7},
		{3, 7}, // 3 days ≤ 7 and > 2 → 7 bucket
		{2, 3},
		{0, 3}, // covered by trailing `if days <= 3` catch-all
		{-1, 0}, // expired
	}
	for _, tc := range cases {
		got := matchThreshold(tc.days)
		if got != tc.want {
			t.Errorf("matchThreshold(%d) = %d, want %d", tc.days, got, tc.want)
		}
	}
}

func TestInBusinessHours_PacificTime(t *testing.T) {
	t.Parallel()
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skipf("tz data unavailable: %v", err)
	}
	cases := []struct {
		h    int
		want bool
	}{
		{0, false},
		{7, false},
		{8, true},
		{12, true},
		{20, true},
		{21, false},
		{23, false},
	}
	for _, tc := range cases {
		now := time.Date(2026, 4, 15, tc.h, 0, 0, 0, loc)
		got := inBusinessHours(now, "America/Los_Angeles")
		if got != tc.want {
			t.Errorf("inBusinessHours(%d:00) = %v, want %v", tc.h, got, tc.want)
		}
	}
}

func TestInBusinessHours_UnknownTZ_FallsBackToUTC(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	if !inBusinessHours(now, "Not/A/Zone") {
		t.Fatal("unknown TZ should fall back to UTC; 12:00 UTC is business hours")
	}
	// 03:00 UTC — outside business hours.
	early := time.Date(2026, 4, 15, 3, 0, 0, 0, time.UTC)
	if inBusinessHours(early, "Not/A/Zone") {
		t.Fatal("03:00 UTC should not be business hours")
	}
}

// ---- SMS template ----

func TestSMSChaseReminder_IncludesTitle(t *testing.T) {
	t.Parallel()
	got := SMSChaseReminder("TB Test", 7, "https://example.com/u")
	if !contains(got, "TB Test") || !contains(got, "7 days") || !contains(got, "https://example.com/u") {
		t.Fatalf("template missing fields: %q", got)
	}
}

func TestSMSChaseReminder_Truncated(t *testing.T) {
	t.Parallel()
	// Deliberately build a long string to trigger truncation.
	longTitle := ""
	for i := 0; i < 500; i++ {
		longTitle += "x"
	}
	got := SMSChaseReminder(longTitle, 3, "https://x")
	// truncate uses 320 chars then appends a 3-byte UTF-8 ellipsis, so max
	// possible byte length is 322. The intent is "capped near 320", not exact.
	if len(got) > 325 {
		t.Fatalf("SMS length = %d, want <= 325", len(got))
	}
	if len(got) < 100 {
		t.Fatalf("SMS length = %d, want truncation to have happened", len(got))
	}
}

func TestSMSMagicLink_FormatsOK(t *testing.T) {
	t.Parallel()
	got := SMSMagicLink("Sign in", "https://go.example/x")
	if !contains(got, "Sign in") || !contains(got, "https://go.example/x") {
		t.Fatalf("template missing fields: %q", got)
	}
}

// ---- Email templates ----

func TestRenderChaseEmail_TonePerDays(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		days int
		tone string
	}{
		{3, "URGENT"},
		{7, "Reminder"},
		{14, ""},
		{42, ""},
	} {
		subject, _, _ := RenderChaseEmail(ChaseEmailData{
			RecipientName: "Mom", DocTitle: "Immunization", DaysUntil: tc.days, ExpiresOn: "Jan 1",
			UploadURL: "https://u/x", ProviderName: "Sunshine",
		})
		if tc.tone == "" && (contains(subject, "URGENT") || contains(subject, "Reminder")) {
			t.Errorf("days=%d subject has unexpected tone: %q", tc.days, subject)
		}
		if tc.tone != "" && !contains(subject, tc.tone) {
			t.Errorf("days=%d subject missing tone %q: %q", tc.days, tc.tone, subject)
		}
	}
}

func TestRenderChaseEmail_HTMLEscaping(t *testing.T) {
	t.Parallel()
	_, html, _ := RenderChaseEmail(ChaseEmailData{
		RecipientName: "<script>",
		DocTitle:      "<x>",
		ExpiresOn:     "2026",
		DaysUntil:     14,
		UploadURL:     "https://x",
		ProviderName:  "A&B",
	})
	if contains(html, "<script>") {
		t.Fatal("unescaped <script> in HTML output")
	}
	if !contains(html, "&lt;script&gt;") {
		t.Fatal("expected escaped <script>")
	}
}

func TestRenderMagicLinkEmail_HasURLAndExpiry(t *testing.T) {
	t.Parallel()
	subject, html, text := RenderMagicLinkEmail(MagicLinkEmailData{
		RecipientName: "Alice", ActionText: "Sign in", URL: "https://go.example/abc", ExpiresIn: "15 minutes",
	})
	if !contains(subject, "ComplianceKit") {
		t.Fatal("subject missing brand")
	}
	if !contains(html, "https://go.example/abc") || !contains(text, "https://go.example/abc") {
		t.Fatal("URL missing from body")
	}
	if !contains(text, "15 minutes") {
		t.Fatal("expiry missing")
	}
}

func TestRenderWelcomeEmail(t *testing.T) {
	t.Parallel()
	subject, html, text := RenderWelcomeEmail("Sunshine")
	if subject == "" || html == "" || text == "" {
		t.Fatal("empty welcome parts")
	}
	if !contains(html, "Sunshine") {
		t.Fatal("provider name missing from welcome HTML")
	}
}

func TestRenderReceiptEmail(t *testing.T) {
	t.Parallel()
	subject, html, _ := RenderReceiptEmail("Sunshine", 9900, "May 15")
	if !contains(subject, "receipt") && !contains(subject, "Receipt") {
		t.Fatal("subject missing receipt")
	}
	if !contains(html, "99.00") {
		t.Fatalf("dollars formatting missing: %q", html)
	}
}

// ---- Emailer + SMSSender guard checks ----

func TestEmailer_NilReceiver_Errors(t *testing.T) {
	t.Parallel()
	var e *Emailer
	if err := e.Send(nil, EmailMessage{To: "x@y.com", Subject: "hi"}); err == nil {
		t.Fatal("expected error from nil emailer")
	}
}

func TestSMSSender_NilReceiver_Errors(t *testing.T) {
	t.Parallel()
	var s *SMSSender
	if _, err := s.Send("+15555550100", "hi"); err == nil {
		t.Fatal("expected error from nil sender")
	}
}

func TestSMSSender_UninitClient_Errors(t *testing.T) {
	t.Parallel()
	s := NewSMSSender(SMSConfig{})
	if _, err := s.Send("+15555550100", "hi"); err == nil {
		t.Fatal("expected error from SMSSender without creds")
	}
}

// ---- helpers ----

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

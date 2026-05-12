package notify

import (
	"strings"
	"testing"
)

// TestRenderEmailVerification_HappyPath confirms the template renders, fills
// the placeholders, and produces both an HTML and a plain-text body.
func TestRenderEmailVerification_HappyPath(t *testing.T) {
	t.Parallel()
	subject, html, text, err := RenderEmailVerification(EmailVerificationData{
		FirstName:    "Sarah Johnson",
		FacilityName: "Sunny Days Learning Center",
		VerifyURL:    "https://app.levelbrook.com/verify-email/Bx7kPmQ2nT9rW4yL3vZ8aH6cJ5dF1gE0M",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if subject != "Confirm your Levelbrook email" {
		t.Fatalf("subject = %q", subject)
	}
	for _, want := range []string{
		"Sarah", // first name only
		"Sunny Days Learning Center",
		"https://app.levelbrook.com/verify-email/Bx7kPmQ2nT9rW4yL3vZ8aH6cJ5dF1gE0M",
		"24", // default expiry hours
		"levelbrookteam@gmail.com",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML body missing %q", want)
		}
		if !strings.Contains(text, want) {
			t.Errorf("text body missing %q", want)
		}
	}
	if strings.Contains(html, "Sarah Johnson") {
		t.Errorf("HTML rendered full name; expected first-name-only")
	}
}

// TestRenderEmailVerification_HTMLEscapes: html/template auto-escapes
// FirstName + FacilityName so a hostile value cannot inject script tags.
func TestRenderEmailVerification_HTMLEscapes(t *testing.T) {
	t.Parallel()
	_, html, _, err := RenderEmailVerification(EmailVerificationData{
		FirstName:    "<script>alert(1)</script>",
		FacilityName: "<b>Naughty</b>",
		VerifyURL:    "https://app.levelbrook.com/verify-email/x",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(html, "<script>") {
		t.Fatalf("unescaped <script> in HTML output")
	}
	if strings.Contains(html, "<b>Naughty</b>") {
		t.Fatalf("unescaped <b> in HTML output")
	}
}

// TestRenderEmailVerification_Defaults: zero values get safe fallbacks.
func TestRenderEmailVerification_Defaults(t *testing.T) {
	t.Parallel()
	_, html, text, err := RenderEmailVerification(EmailVerificationData{
		VerifyURL: "https://app.levelbrook.com/verify-email/x",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(html, "there") {
		t.Errorf("expected fallback FirstName 'there' in HTML")
	}
	if !strings.Contains(html, "your account") {
		t.Errorf("expected fallback FacilityName 'your account' in HTML")
	}
	if !strings.Contains(html, "24") {
		t.Errorf("expected default 24-hour expiry in HTML")
	}
	if !strings.Contains(text, "24") {
		t.Errorf("expected default 24-hour expiry in text")
	}
}

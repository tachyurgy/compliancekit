package notify

import (
	"strings"
	"testing"
)

// TestRenderPasswordReset_HappyPath confirms the template renders, fills the
// placeholders, and produces both an HTML and a plain-text body.
func TestRenderPasswordReset_HappyPath(t *testing.T) {
	t.Parallel()
	subject, html, text, err := RenderPasswordReset(PasswordResetData{
		FirstName: "Sarah Johnson",
		ResetURL:  "https://app.levelbrook.com/reset-password/Bx7kPmQ2nT9rW4yL3vZ8aH6cJ5dF1gE0M",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if subject != "Reset your Levelbrook password" {
		t.Fatalf("subject = %q", subject)
	}
	for _, want := range []string{
		"Sarah",                                                                          // first name only
		"https://app.levelbrook.com/reset-password/Bx7kPmQ2nT9rW4yL3vZ8aH6cJ5dF1gE0M",     // reset URL
		"30",                                                                             // default expiry minutes
		"levelbrookteam@gmail.com",                                                       // footer support address
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML body missing %q", want)
		}
		if !strings.Contains(text, want) {
			t.Errorf("text body missing %q", want)
		}
	}
	if strings.Contains(html, "Sarah Johnson") {
		t.Errorf("HTML rendered the full name; expected first-name-only (got %q)", "Sarah Johnson")
	}
}

// TestRenderPasswordReset_HTMLEscapesFirstName: html/template auto-escapes,
// so a hostile FirstName cannot inject script tags.
func TestRenderPasswordReset_HTMLEscapesFirstName(t *testing.T) {
	t.Parallel()
	_, html, _, err := RenderPasswordReset(PasswordResetData{
		FirstName: "<script>alert(1)</script>",
		ResetURL:  "https://app.levelbrook.com/reset-password/x",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(html, "<script>") {
		t.Fatalf("unescaped <script> in HTML output")
	}
}

// TestRenderPasswordReset_DefaultsExpiryAndFirstName: zero values get sane
// defaults — `30` minutes and "there" for an empty name.
func TestRenderPasswordReset_Defaults(t *testing.T) {
	t.Parallel()
	_, html, text, err := RenderPasswordReset(PasswordResetData{
		ResetURL: "https://app.levelbrook.com/reset-password/x",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(html, "there") {
		t.Errorf("expected fallback FirstName 'there' in HTML")
	}
	if !strings.Contains(html, "30") {
		t.Errorf("expected default 30-minute expiry in HTML")
	}
	if !strings.Contains(text, "30") {
		t.Errorf("expected default 30-minute expiry in text")
	}
}

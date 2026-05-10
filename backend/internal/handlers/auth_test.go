package handlers

import (
	"testing"
	"time"
)

// TestSessionTTL covers REQ-011 #1 + #2: 90 days when remember_me is set,
// 24 hours otherwise. Table-driven to make the contract obvious from a
// quick read of the test.
func TestSessionTTL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		rememberMe bool
		want       time.Duration
	}{
		{"remembered → 90 days", true, 90 * 24 * time.Hour},
		{"single-session → 24 hours", false, 24 * time.Hour},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := SessionTTL(tc.rememberMe)
			if got != tc.want {
				t.Fatalf("SessionTTL(%v) = %v, want %v", tc.rememberMe, got, tc.want)
			}
		})
	}
}

// TestSHA256Hex_Stable confirms our document hash is stable over the same
// input and different over different inputs. This is the contract relied on
// by the legal_signatures.document_text_sha256 column — we hash the same
// Markdown the operator saw and store its fingerprint.
func TestSHA256Hex_Stable(t *testing.T) {
	t.Parallel()
	a := sha256Hex("hello")
	b := sha256Hex("hello")
	c := sha256Hex("hello!")
	if a != b {
		t.Fatalf("sha256Hex not stable: %q vs %q", a, b)
	}
	if a == c {
		t.Fatalf("sha256Hex collided on different input: %q", a)
	}
	// SHA-256 hex is always 64 chars.
	if len(a) != 64 {
		t.Fatalf("sha256Hex length = %d, want 64", len(a))
	}
}

// TestNewResetToken confirms our reset token is high-entropy and hex-encoded
// (so it survives URL embedding without escaping). 32 random bytes → 64 hex.
func TestNewResetToken(t *testing.T) {
	t.Parallel()
	a, err := newResetToken()
	if err != nil {
		t.Fatalf("newResetToken: %v", err)
	}
	b, err := newResetToken()
	if err != nil {
		t.Fatalf("newResetToken: %v", err)
	}
	if a == b {
		t.Fatalf("newResetToken collided: %q", a)
	}
	if len(a) != 64 {
		t.Fatalf("newResetToken length = %d, want 64", len(a))
	}
}

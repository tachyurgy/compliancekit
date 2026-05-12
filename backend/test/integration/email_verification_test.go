package integration

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
)

// REQ-010 — end-to-end coverage of the email-verification flow.
//
// The harness wires Emailer=nil, so the actual SES send is skipped. We
// exercise everything else: row creation by signup, /api/auth/verify-email
// happy path (used_at + users.email_verified_at flipped), and the
// rejection cases (unknown, expired, reused).

// mintEmailVerification writes an email_verifications row directly using
// the same shape the handler does. Returns the raw token + row id.
func mintEmailVerification(t *testing.T, h *Harness, userID string, expires time.Time) (rawToken, rowID string) {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	rawToken = hex.EncodeToString(b)
	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])
	rowID = base62.NewID()[:22]
	if _, err := h.DB.Exec(
		`INSERT INTO email_verifications (id, user_id, token_hash, expires_at, ip, user_agent)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		rowID, userID, tokenHash, expires.UTC().Format(time.RFC3339), "127.0.0.1", "go-test",
	); err != nil {
		t.Fatalf("insert: %v", err)
	}
	return rawToken, rowID
}

// TestSignup_CreatesEmailVerificationRow: a successful /api/auth/signup
// should kick off SendEmailVerification asynchronously. The Emailer is nil
// in the harness so no SES call happens, but the email_verifications row
// is the database-side proof the path ran.
func TestSignup_CreatesEmailVerificationRow(t *testing.T) {
	h := NewHarness(t)

	const email = "verify-row@example.com"
	signupForReset(t, h, "Verify Row Daycare", email)

	// The signup handler dispatches the send in a goroutine. Poll briefly.
	var n int
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := h.DB.QueryRow(
			`SELECT COUNT(*) FROM email_verifications ev
			   JOIN users u ON u.id = ev.user_id
			  WHERE u.email = ?`, email).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n >= 1 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if n != 1 {
		t.Fatalf("expected one email_verifications row after signup, got %d", n)
	}
}

// TestVerifyEmail_HappyPath: a valid token flips users.email_verified_at
// and marks the row used.
func TestVerifyEmail_HappyPath(t *testing.T) {
	h := NewHarness(t)

	const email = "verify-happy@example.com"
	userID := signupForReset(t, h, "Verify Happy", email)
	rawToken, rowID := mintEmailVerification(t, h, userID, time.Now().Add(24*time.Hour))

	resp := doJSON(t, http.MethodPost, h.URL("/api/auth/verify-email"),
		map[string]string{"token": rawToken})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("verify-email: expected 200, got %d", resp.StatusCode)
	}

	var usedAt string
	if err := h.DB.QueryRow(`SELECT COALESCE(used_at, '') FROM email_verifications WHERE id = ?`, rowID).Scan(&usedAt); err != nil {
		t.Fatalf("read used_at: %v", err)
	}
	if usedAt == "" {
		t.Fatal("email_verifications.used_at not set")
	}

	var verifiedAt string
	if err := h.DB.QueryRow(`SELECT COALESCE(email_verified_at, '') FROM users WHERE id = ?`, userID).Scan(&verifiedAt); err != nil {
		t.Fatalf("read users.email_verified_at: %v", err)
	}
	if verifiedAt == "" {
		t.Fatal("users.email_verified_at not set after verify")
	}
}

func TestVerifyEmail_RejectsReusedToken(t *testing.T) {
	h := NewHarness(t)
	userID := signupForReset(t, h, "Verify Reuse", "verify-reuse@example.com")
	rawToken, _ := mintEmailVerification(t, h, userID, time.Now().Add(24*time.Hour))

	first := doJSON(t, http.MethodPost, h.URL("/api/auth/verify-email"),
		map[string]string{"token": rawToken})
	first.Body.Close()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first verify: expected 200, got %d", first.StatusCode)
	}
	second := doJSON(t, http.MethodPost, h.URL("/api/auth/verify-email"),
		map[string]string{"token": rawToken})
	second.Body.Close()
	if second.StatusCode == http.StatusOK {
		t.Fatalf("second verify: expected non-200 (token reuse), got 200")
	}
}

func TestVerifyEmail_RejectsExpiredToken(t *testing.T) {
	h := NewHarness(t)
	userID := signupForReset(t, h, "Verify Expired", "verify-expired@example.com")
	rawToken, _ := mintEmailVerification(t, h, userID, time.Now().Add(-1*time.Hour))

	resp := doJSON(t, http.MethodPost, h.URL("/api/auth/verify-email"),
		map[string]string{"token": rawToken})
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("expired token: expected non-200, got 200")
	}
}

func TestVerifyEmail_RejectsUnknownToken(t *testing.T) {
	h := NewHarness(t)
	bogus := "00000000000000000000000000000000000000000000000000000000000000ff"
	resp := doJSON(t, http.MethodPost, h.URL("/api/auth/verify-email"),
		map[string]string{"token": bogus})
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("unknown token: expected non-200, got 200")
	}
}

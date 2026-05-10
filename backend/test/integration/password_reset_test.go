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

// REQ-034 — end-to-end coverage of the password-reset flow.
//
// The test harness wires Emailer=nil, so the actual SES send is skipped.
// We exercise everything else: row creation on /forgot-password, no
// existence leak on unknown emails, /reset-password happy path with
// session establishment, and the rejection cases (used, expired, unknown,
// short password). The render side of the email is unit-tested in
// internal/notify/password_reset_test.go.

// signupForReset is a small fixture helper that registers a provider via
// the public signup endpoint and returns the resulting user_id. Mirrors the
// idiom used in TestSignin_RoundtripWithCorrectPassword so we stay close to
// the rest of the auth-test set.
func signupForReset(t *testing.T, h *Harness, name, email string) (userID string) {
	t.Helper()
	resp := doJSON(t, http.MethodPost, h.URL("/api/auth/signup"), map[string]string{
		"name":        name,
		"owner_email": email,
		"state_code":  "FL",
		"password":    testSignupPassword,
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("signup precondition for %s: %d", email, resp.StatusCode)
	}
	if err := h.DB.QueryRow(`SELECT id FROM users WHERE email = ?`, email).Scan(&userID); err != nil {
		t.Fatalf("lookup user %s: %v", email, err)
	}
	return userID
}

// mintPasswordReset writes a reset-token row directly via SQL using the same
// shape the handler does: 32-byte random hex token, sha256 of that token as
// `token_hash`. Returns the raw token (the bit that's emailed) and the row id.
// Used because the test harness' Emailer is nil, so we can't intercept the
// raw token from the email-send path.
func mintPasswordReset(t *testing.T, h *Harness, userID string, expires time.Time) (rawToken, resetID string) {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("mintPasswordReset: rand: %v", err)
	}
	rawToken = hex.EncodeToString(b)
	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])
	resetID = base62.NewID()[:22]
	if _, err := h.DB.Exec(
		`INSERT INTO password_resets (id, user_id, token_hash, expires_at, ip, user_agent)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		resetID, userID, tokenHash, expires.UTC().Format(time.RFC3339), "127.0.0.1", "go-test",
	); err != nil {
		t.Fatalf("mintPasswordReset: insert: %v", err)
	}
	return rawToken, resetID
}

// TestForgotPassword_AlwaysReturns200_WithRowOnHit confirms the no-leak
// posture: a forgot-password request returns 200 whether or not the email
// exists, AND a password_resets row is only inserted when the email maps to
// a real account.
func TestForgotPassword_AlwaysReturns200_WithRowOnHit(t *testing.T) {
	h := NewHarness(t)

	const realEmail = "real-forgot@example.com"
	signupForReset(t, h, "Real Daycare", realEmail)

	// Hit /forgot-password for the real email → 200.
	hit := doJSON(t, http.MethodPost, h.URL("/api/auth/forgot-password"),
		map[string]string{"email": realEmail})
	hit.Body.Close()
	if hit.StatusCode != http.StatusOK {
		t.Fatalf("forgot-password real: expected 200, got %d", hit.StatusCode)
	}
	var n int
	if err := h.DB.QueryRow(
		`SELECT COUNT(*) FROM password_resets pr
		   JOIN users u ON u.id = pr.user_id
		  WHERE u.email = ?`, realEmail).Scan(&n); err != nil {
		t.Fatalf("count password_resets: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected one password_resets row for real account, got %d", n)
	}

	// Unknown email → 200, but NO row inserted.
	miss := doJSON(t, http.MethodPost, h.URL("/api/auth/forgot-password"),
		map[string]string{"email": "nobody@example.com"})
	miss.Body.Close()
	if miss.StatusCode != http.StatusOK {
		t.Fatalf("forgot-password unknown: expected 200, got %d", miss.StatusCode)
	}
	var totalAfterMiss int
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM password_resets`).Scan(&totalAfterMiss); err != nil {
		t.Fatalf("count password_resets after miss: %v", err)
	}
	if totalAfterMiss != 1 {
		t.Fatalf("unknown-email path inserted a row; expected 1 total, got %d", totalAfterMiss)
	}

	// Empty email → 200 with no row inserted (legitimately ignored).
	empty := doJSON(t, http.MethodPost, h.URL("/api/auth/forgot-password"),
		map[string]string{"email": ""})
	empty.Body.Close()
	if empty.StatusCode != http.StatusOK {
		t.Fatalf("forgot-password empty: expected 200, got %d", empty.StatusCode)
	}
}

// TestResetPassword_HappyPath: a valid token + a fresh ≥12-char password
// resets the password, marks the token used, revokes any other open
// sessions for the user, sets a new ck_sess cookie on the response, and
// lets the user sign in with the new password.
func TestResetPassword_HappyPath(t *testing.T) {
	h := NewHarness(t)

	const email = "happy-reset@example.com"
	const newPassword = "fresh-password-9999"
	userID := signupForReset(t, h, "Happy Reset", email)

	// Insert a stray active session row to verify it gets revoked.
	staySessID := base62.NewID()[:22]
	if _, err := h.DB.Exec(
		`INSERT INTO sessions (id, provider_id, user_id, expires_at, created_at)
		 VALUES (?, (SELECT provider_id FROM users WHERE id = ?), ?, ?, CURRENT_TIMESTAMP)`,
		staySessID, userID, userID, time.Now().Add(48*time.Hour).UTC().Format(time.RFC3339),
	); err != nil {
		t.Fatalf("seed stray session: %v", err)
	}

	// Mint a token directly and reset.
	rawToken, resetID := mintPasswordReset(t, h, userID, time.Now().Add(20*time.Minute))
	resp := doJSON(t, http.MethodPost, h.URL("/api/auth/reset-password"),
		map[string]any{"token": rawToken, "password": newPassword, "remember_me": true})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reset-password: expected 200, got %d", resp.StatusCode)
	}

	// New session cookie set on the response.
	gotSess := false
	for _, c := range resp.Cookies() {
		if c.Name == "ck_sess" {
			gotSess = true
			break
		}
	}
	if !gotSess {
		t.Fatal("reset-password did not set ck_sess cookie")
	}

	// used_at on the reset row.
	var usedAt string
	if err := h.DB.QueryRow(`SELECT COALESCE(used_at, '') FROM password_resets WHERE id = ?`, resetID).Scan(&usedAt); err != nil {
		t.Fatalf("read used_at: %v", err)
	}
	if usedAt == "" {
		t.Fatal("password_resets.used_at not set")
	}

	// The stray session is revoked.
	var revokedAt string
	if err := h.DB.QueryRow(`SELECT COALESCE(revoked_at, '') FROM sessions WHERE id = ?`, staySessID).Scan(&revokedAt); err != nil {
		t.Fatalf("read stray session: %v", err)
	}
	if revokedAt == "" {
		t.Fatal("stray session was not revoked after password reset")
	}

	// Sign in with the new password → 200 + fresh ck_sess.
	signin := doJSON(t, http.MethodPost, h.URL("/api/auth/signin"),
		map[string]string{"email": email, "password": newPassword})
	signin.Body.Close()
	if signin.StatusCode != http.StatusOK {
		t.Fatalf("signin with new password: expected 200, got %d", signin.StatusCode)
	}
}

// TestResetPassword_RejectsReusedToken: the second call with the same token
// is rejected.
func TestResetPassword_RejectsReusedToken(t *testing.T) {
	h := NewHarness(t)
	userID := signupForReset(t, h, "Reuse Reset", "reuse-reset@example.com")
	rawToken, _ := mintPasswordReset(t, h, userID, time.Now().Add(20*time.Minute))

	first := doJSON(t, http.MethodPost, h.URL("/api/auth/reset-password"),
		map[string]any{"token": rawToken, "password": "first-time-password-1"})
	first.Body.Close()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first reset: expected 200, got %d", first.StatusCode)
	}

	second := doJSON(t, http.MethodPost, h.URL("/api/auth/reset-password"),
		map[string]any{"token": rawToken, "password": "second-time-password-2"})
	second.Body.Close()
	if second.StatusCode == http.StatusOK {
		t.Fatalf("second reset: expected non-200 (token reuse), got 200")
	}
}

// TestResetPassword_RejectsExpiredToken: a token whose expires_at is in the
// past is rejected.
func TestResetPassword_RejectsExpiredToken(t *testing.T) {
	h := NewHarness(t)
	userID := signupForReset(t, h, "Expired Reset", "expired-reset@example.com")
	rawToken, _ := mintPasswordReset(t, h, userID, time.Now().Add(-1*time.Hour))

	resp := doJSON(t, http.MethodPost, h.URL("/api/auth/reset-password"),
		map[string]any{"token": rawToken, "password": "expired-token-pwd-99"})
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("expired token: expected non-200, got 200")
	}
}

// TestResetPassword_RejectsUnknownToken: a token never minted is rejected.
func TestResetPassword_RejectsUnknownToken(t *testing.T) {
	h := NewHarness(t)
	bogus := "00000000000000000000000000000000000000000000000000000000000000ff"
	resp := doJSON(t, http.MethodPost, h.URL("/api/auth/reset-password"),
		map[string]any{"token": bogus, "password": "doesnt-matter-12chars"})
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("unknown token: expected non-200, got 200")
	}
}

// TestResetPassword_RejectsShortPassword: 12-char minimum.
func TestResetPassword_RejectsShortPassword(t *testing.T) {
	h := NewHarness(t)
	userID := signupForReset(t, h, "Short PW Reset", "short-pwd-reset@example.com")
	rawToken, _ := mintPasswordReset(t, h, userID, time.Now().Add(20*time.Minute))

	resp := doJSON(t, http.MethodPost, h.URL("/api/auth/reset-password"),
		map[string]any{"token": rawToken, "password": "short"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("short password: expected 400, got %d", resp.StatusCode)
	}
}

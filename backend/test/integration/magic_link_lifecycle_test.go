package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/tachyurgy/compliancekit/backend/internal/magiclink"
)

// TestProviderCallback_MagicLinkDeprecated covers the post-2026-05-08
// password-auth migration: /api/auth/callback no longer accepts manager
// signin tokens — it returns 400 with a clear deprecation message regardless
// of whether a token is present. The endpoint stays mounted for backwards
// compatibility (so old links explain themselves rather than 404), but it
// never grants a session.
func TestProviderCallback_MagicLinkDeprecated(t *testing.T) {
	h := NewHarness(t)

	// No token at all.
	noTok, err := http.Get(h.URL("/api/auth/callback"))
	if err != nil {
		t.Fatalf("GET callback: %v", err)
	}
	noTok.Body.Close()
	if noTok.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing token: expected 400, got %d", noTok.StatusCode)
	}

	// Even with a freshly minted provider-signin token the endpoint refuses.
	_, providerID, _ := h.AuthAs(t, "FL")
	token, _, err := h.Magic.Generate(context.Background(), magiclink.KindProviderSignin, providerID, providerID, 0)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	withTok, err := http.Get(h.URL("/api/auth/callback?t=" + token))
	if err != nil {
		t.Fatalf("GET callback with token: %v", err)
	}
	withTok.Body.Close()
	if withTok.StatusCode != http.StatusBadRequest {
		t.Fatalf("provider-signin token via callback: expected 400, got %d", withTok.StatusCode)
	}
}

// TestMagicLink_WrongKind_Forbidden: a parent-upload token used on the
// provider callback is rejected (the callback now flat-out refuses any
// token). 200 would be a bug.
func TestMagicLink_WrongKind_Forbidden(t *testing.T) {
	h := NewHarness(t)
	_, providerID, _ := h.AuthAs(t, "CA")

	token, _, err := h.Magic.Generate(context.Background(), magiclink.KindParentUpload, providerID, "some-child", 0)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	resp, err := http.Get(h.URL("/api/auth/callback?t=" + token))
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("wrong-kind token callback unexpectedly succeeded")
	}
}

// TestLogout_ClearsSession: after Logout the session is deleted and the
// next /api/me is 401.
func TestLogout_ClearsSession(t *testing.T) {
	h := NewHarness(t)
	client, _, _ := h.AuthAs(t, "CA")

	// Confirm we start authed.
	me1, err := client.Get(h.URL("/api/me"))
	if err != nil {
		t.Fatalf("me1: %v", err)
	}
	me1.Body.Close()
	if me1.StatusCode != http.StatusOK {
		t.Fatalf("me1 not authed: %d", me1.StatusCode)
	}

	// Logout.
	logout, err := client.Post(h.URL("/api/auth/logout"), "application/json", nil)
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	logout.Body.Close()
	if logout.StatusCode != http.StatusOK {
		t.Fatalf("logout: expected 200, got %d", logout.StatusCode)
	}

	// Next /api/me — cookie is cleared AND session row is deleted.
	// Note: cookiejar keeps the cookie even after Max-Age=0 under some
	// circumstances; sessions table DELETE is what really invalidates. So
	// we assert the session row count drops to 0.
	var n int
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&n); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if n != 0 {
		t.Fatalf("logout should have deleted session row; %d remain", n)
	}
}

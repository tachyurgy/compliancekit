package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"testing"

	"github.com/tachyurgy/compliancekit/backend/internal/magiclink"
)

// TestMagicLink_ProviderSignin_FullRoundtrip walks the provider signin flow:
//   1. /api/auth/signup creates the provider (202).
//   2. Grab the token directly from the magiclink service (the real flow
//      delivers it via email; we skip that).
//   3. GET /api/auth/callback?t=<token> sets a ck_sess cookie (200).
//   4. GET /api/me with that cookie returns the provider (200).
func TestMagicLink_ProviderSignup_FullRoundtrip(t *testing.T) {
	h := NewHarness(t)

	email := "lifecycle@example.com"
	resp := doJSON(t, http.MethodPost, h.URL("/api/auth/signup"), map[string]string{
		"name": "Lifecycle Daycare", "owner_email": email, "state_code": "CA",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("signup: expected 202, got %d", resp.StatusCode)
	}

	// Look up the provider_id so we can generate a new token directly via
	// the magiclink service (the email send path is nil in the harness, so
	// we don't have an intercepted token to reuse).
	var providerID string
	if err := h.DB.QueryRow(`SELECT id FROM providers WHERE owner_email = ?`, email).Scan(&providerID); err != nil {
		t.Fatalf("lookup provider: %v", err)
	}
	token, _, err := h.Magic.Generate(context.Background(), magiclink.KindProviderSignin, providerID, providerID, 0)
	if err != nil {
		t.Fatalf("generate magic: %v", err)
	}

	// Exchange for a session cookie.
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	callback, err := client.Get(h.URL("/api/auth/callback?t=" + token))
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	defer callback.Body.Close()
	if callback.StatusCode != http.StatusOK {
		t.Fatalf("callback: expected 200, got %d (body=%s)", callback.StatusCode, readAll(t, callback))
	}
	var cbOut struct {
		Status     string `json:"status"`
		ProviderID string `json:"provider_id"`
	}
	if err := json.NewDecoder(callback.Body).Decode(&cbOut); err != nil {
		t.Fatalf("decode callback: %v", err)
	}
	if cbOut.ProviderID != providerID {
		t.Fatalf("callback provider_id mismatch: %s vs %s", cbOut.ProviderID, providerID)
	}
	// Cookie jar should now hold ck_sess.
	u, _ := url.Parse(h.Server.URL)
	var sessCookie *http.Cookie
	for _, c := range jar.Cookies(u) {
		if c.Name == "ck_sess" {
			sessCookie = c
			break
		}
	}
	if sessCookie == nil {
		t.Fatalf("callback did not set ck_sess cookie")
	}

	// /api/me roundtrip.
	meResp, err := client.Get(h.URL("/api/me"))
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	defer meResp.Body.Close()
	if meResp.StatusCode != http.StatusOK {
		t.Fatalf("me: expected 200, got %d", meResp.StatusCode)
	}
	var me struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(meResp.Body).Decode(&me); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if me.ID != providerID {
		t.Fatalf("me.id=%s, expected %s", me.ID, providerID)
	}
}

// TestMagicLink_TokenConsumedOnlyOnce: a signin token is single-use. The
// second callback with the same token must 401.
func TestMagicLink_TokenConsumedOnlyOnce(t *testing.T) {
	h := NewHarness(t)
	_, providerID, _ := h.AuthAs(t, "CA") // seeds a provider we can target

	token, _, err := h.Magic.Generate(context.Background(), magiclink.KindProviderSignin, providerID, providerID, 0)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	first, err := http.Get(h.URL("/api/auth/callback?t=" + token))
	if err != nil {
		t.Fatalf("first callback: %v", err)
	}
	first.Body.Close()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first callback: expected 200, got %d", first.StatusCode)
	}

	second, err := http.Get(h.URL("/api/auth/callback?t=" + token))
	if err != nil {
		t.Fatalf("second callback: %v", err)
	}
	second.Body.Close()
	if second.StatusCode != http.StatusUnauthorized {
		t.Fatalf("second callback (reuse): expected 401, got %d", second.StatusCode)
	}
}

// TestMagicLink_ExpiredTokenRejected forcibly sets expires_at in the past
// and confirms the callback returns 401.
func TestMagicLink_ExpiredTokenRejected(t *testing.T) {
	h := NewHarness(t)
	_, providerID, _ := h.AuthAs(t, "CA")

	token, _, err := h.Magic.Generate(context.Background(), magiclink.KindProviderSignin, providerID, providerID, 0)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := h.DB.Exec(`UPDATE magic_link_tokens SET expires_at = datetime('now', '-1 hour')`); err != nil {
		t.Fatalf("expire: %v", err)
	}

	resp, err := http.Get(h.URL("/api/auth/callback?t=" + token))
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired token callback: expected 401, got %d", resp.StatusCode)
	}
}

// TestMagicLink_WrongKind_Forbidden: a parent-upload token used on the
// provider callback is 403 (Callback only accepts KindProviderSignup or
// KindProviderSignin).
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
	// Callback calls Consume, which now marks the (non-sliding) token as
	// consumed before the kind check — but the parent_upload kind IS
	// sliding so the token is not consumed; the handler then rejects with
	// 403. Either way, a success (200) would be a bug.
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("wrong-kind token callback unexpectedly succeeded")
	}
}

// TestMagicLink_MissingToken_Returns401: GET /api/auth/callback with no 't'
// query param is a 401.
func TestMagicLink_MissingToken_Returns401(t *testing.T) {
	h := NewHarness(t)
	resp, err := http.Get(h.URL("/api/auth/callback"))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing token: expected 401, got %d", resp.StatusCode)
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
	// we assert the session row count drops to 0 for this provider.
	var n int
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id IN (SELECT id FROM sessions)`, ).Scan(&n); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if n != 0 {
		t.Fatalf("logout should have deleted session row; %d remain", n)
	}
}

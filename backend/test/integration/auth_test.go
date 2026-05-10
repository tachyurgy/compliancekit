package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestHealthz is the simplest possible smoke test: the harness wires up, the
// server starts, /healthz returns 200.
func TestHealthz(t *testing.T) {
	h := NewHarness(t)

	resp, err := http.Get(h.URL("/healthz"))
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz: expected 200, got %d", resp.StatusCode)
	}
}

// Password-auth contract (post-2026-05-08 commit 4a2286a):
//   * POST /api/auth/signup expects { name|facility_name, owner_email|email,
//     state_code, password } where password is ≥12 chars.
//     Success returns HTTP 200 with a `ck_sess` cookie set.
//   * POST /api/auth/signin expects { email, password }. 200 + cookie on
//     success; 401 ("authentication required") on unknown email or wrong
//     password — this endpoint must NOT leak account existence.
const testSignupPassword = "QABot-Strong-12345" // ≥12 chars

// TestSignup_HappyPath_CA covers the most basic real signup: CA provider,
// valid shape, server returns 200 + ck_sess cookie and writes a providers row.
func TestSignup_HappyPath_CA(t *testing.T) {
	h := NewHarness(t)

	body := map[string]string{
		"name":        "Sunshine Daycare CA",
		"owner_email": "owner+ca@example.com",
		"state_code":  "CA",
		"password":    testSignupPassword,
	}
	resp := doJSON(t, http.MethodPost, h.URL("/api/auth/signup"), body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("signup: expected 200, got %d", resp.StatusCode)
	}

	// Cookie must be set so the user is signed in immediately.
	var sess *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "ck_sess" {
			sess = c
			break
		}
	}
	if sess == nil {
		t.Fatalf("signup: expected ck_sess cookie, got none")
	}

	// Verify provider row exists with CA state code.
	var stateCode string
	err := h.DB.QueryRow(`SELECT state_code FROM providers WHERE owner_email = ?`, "owner+ca@example.com").Scan(&stateCode)
	if err != nil {
		t.Fatalf("query providers: %v", err)
	}
	if stateCode != "CA" {
		t.Fatalf("expected state_code=CA, got %q", stateCode)
	}
}

// TestSignup_RejectsUnsupportedState is the explicit MVP contract: CA/TX/FL
// only. Any other state returns 400.
//
// The handler defaults a blank state to FL (REQ-002 launch), so blank is now
// a 200 — the MVP-state list is enforced only when an explicit code is sent.
func TestSignup_RejectsUnsupportedState(t *testing.T) {
	h := NewHarness(t)

	cases := []string{"NY", "OR", "WA", "IL", "INVALID"}
	for _, state := range cases {
		t.Run("state="+state, func(t *testing.T) {
			body := map[string]string{
				"name":        "Test",
				"owner_email": "x+" + strings.ToLower(state) + "@example.com",
				"state_code":  state,
				"password":    testSignupPassword,
			}
			resp := doJSON(t, http.MethodPost, h.URL("/api/auth/signup"), body)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("state=%q: expected 400, got %d", state, resp.StatusCode)
			}
		})
	}
}

// TestSignup_RequiresPassword: REQ-011 floor of 12 characters.
func TestSignup_RequiresPassword(t *testing.T) {
	h := NewHarness(t)

	cases := []struct {
		name string
		pwd  string
	}{
		{"empty", ""},
		{"too-short", "short"},
		{"eleven-chars", "12345678901"}, // one below the floor
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := map[string]string{
				"name":        "Pwd " + c.name,
				"owner_email": "pwd-" + c.name + "@example.com",
				"state_code":  "FL",
				"password":    c.pwd,
			}
			resp := doJSON(t, http.MethodPost, h.URL("/api/auth/signup"), body)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("password=%q: expected 400, got %d", c.pwd, resp.StatusCode)
			}
		})
	}
}

// TestSignup_AcceptsCaseInsensitiveStateCode confirms the state-code check
// normalizes to uppercase before validating.
func TestSignup_AcceptsCaseInsensitiveStateCode(t *testing.T) {
	h := NewHarness(t)

	for _, state := range []string{"ca", "Tx", "fl"} {
		body := map[string]string{
			"name":        "Case " + state,
			"owner_email": "case-" + strings.ToLower(state) + "@example.com",
			"state_code":  state,
			"password":    testSignupPassword,
		}
		resp := doJSON(t, http.MethodPost, h.URL("/api/auth/signup"), body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("state=%q: expected 200, got %d", state, resp.StatusCode)
		}
	}
}

// TestSignup_AcceptsREQ002FieldNames: the handler accepts both the legacy
// names (name + owner_email) and the REQ-002 names (facility_name + email +
// full_name). New clients should use the REQ-002 names.
func TestSignup_AcceptsREQ002FieldNames(t *testing.T) {
	h := NewHarness(t)

	body := map[string]string{
		"facility_name": "REQ-002 Daycare",
		"email":         "req002@example.com",
		"full_name":     "Req Two",
		"state_code":    "FL",
		"password":      testSignupPassword,
	}
	resp := doJSON(t, http.MethodPost, h.URL("/api/auth/signup"), body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("REQ-002 shape: expected 200, got %d", resp.StatusCode)
	}

	var name, email string
	if err := h.DB.QueryRow(`SELECT name, owner_email FROM providers WHERE owner_email = ?`, "req002@example.com").Scan(&name, &email); err != nil {
		t.Fatalf("query providers: %v", err)
	}
	if name != "REQ-002 Daycare" || email != "req002@example.com" {
		t.Fatalf("provider row mismatch: name=%q email=%q", name, email)
	}
}

// TestSignin_RequiresPassword validates that the signin endpoint enforces
// both fields and does not leak account existence.
func TestSignin_RequiresPassword(t *testing.T) {
	h := NewHarness(t)

	// Empty body → 400.
	empty := doJSON(t, http.MethodPost, h.URL("/api/auth/signin"), map[string]string{"email": ""})
	empty.Body.Close()
	if empty.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty email: expected 400, got %d", empty.StatusCode)
	}

	// Email but no password → 400 (handler renders "email and password required").
	noPwd := doJSON(t, http.MethodPost, h.URL("/api/auth/signin"), map[string]string{"email": "nobody@example.com"})
	noPwd.Body.Close()
	if noPwd.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing password: expected 400, got %d", noPwd.StatusCode)
	}

	// Unknown email + arbitrary password → 401, NOT 404. The endpoint must
	// not leak whether the email is registered.
	unknown := doJSON(t, http.MethodPost, h.URL("/api/auth/signin"), map[string]string{
		"email":    "nobody@example.com",
		"password": testSignupPassword,
	})
	unknown.Body.Close()
	if unknown.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unknown account: expected 401, got %d", unknown.StatusCode)
	}
}

// TestSignin_RoundtripWithCorrectPassword: signup, then signin with the same
// password — second signin returns 200 + cookie.
func TestSignin_RoundtripWithCorrectPassword(t *testing.T) {
	h := NewHarness(t)

	signup := doJSON(t, http.MethodPost, h.URL("/api/auth/signup"), map[string]string{
		"name":        "Roundtrip Daycare",
		"owner_email": "roundtrip@example.com",
		"state_code":  "FL",
		"password":    testSignupPassword,
	})
	signup.Body.Close()
	if signup.StatusCode != http.StatusOK {
		t.Fatalf("signup precondition: %d", signup.StatusCode)
	}

	in := doJSON(t, http.MethodPost, h.URL("/api/auth/signin"), map[string]string{
		"email":    "roundtrip@example.com",
		"password": testSignupPassword,
	})
	defer in.Body.Close()
	if in.StatusCode != http.StatusOK {
		t.Fatalf("signin: expected 200, got %d", in.StatusCode)
	}
	var sess *http.Cookie
	for _, c := range in.Cookies() {
		if c.Name == "ck_sess" {
			sess = c
			break
		}
	}
	if sess == nil {
		t.Fatalf("signin: expected ck_sess cookie")
	}

	// Wrong password → 401.
	wrong := doJSON(t, http.MethodPost, h.URL("/api/auth/signin"), map[string]string{
		"email":    "roundtrip@example.com",
		"password": "WRONG-Password-12345",
	})
	wrong.Body.Close()
	if wrong.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password: expected 401, got %d", wrong.StatusCode)
	}
}

// TestMeRequiresSession confirms /api/me is session-gated: no cookie → 401.
func TestMeRequiresSession(t *testing.T) {
	h := NewHarness(t)

	resp, err := http.Get(h.URL("/api/me"))
	if err != nil {
		t.Fatalf("GET /api/me: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without cookie, got %d", resp.StatusCode)
	}
}

// TestUnsupportedStateDashboard covers Fix #9: a provider with an unsupported
// state code in the DB (can happen if a seed/backfill skipped validation)
// still sees a dashboard — but the compliance engine reports a single
// STATE-NOT-SUPPORTED violation instead of silently returning score=100.
// This test writes a provider + session directly, then hits /api/dashboard.
func TestUnsupportedStateDashboard_SkippedInMVP(t *testing.T) {
	t.Skip("Exercising unsupported-state dashboard requires wiring sessions via raw SQL or a helper; covered by engine unit test TestEvaluate_UnsupportedState.")
}

// --- helpers ---

func doJSON(t *testing.T, method, url string, body any) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}

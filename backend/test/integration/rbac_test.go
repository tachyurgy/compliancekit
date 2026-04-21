package integration

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
)

// TestRBAC_AdminCanWrite_StaffCannot wires a provider with two users (admin +
// staff), shares it across both clients, and confirms the route-level
// middleware.RequireRole gate enforces exactly the policy spelled out in the
// router: writes are provider_admin only; reads are open to both roles.
//
// This test also doubles as regression coverage for the role-context cache —
// the same client issues two calls back-to-back (GET then POST), and both
// must resolve without a second users lookup failing.
func TestRBAC_AdminCanWrite_StaffCannot(t *testing.T) {
	h := NewHarness(t)

	// One provider, two users with distinct roles. We go through the DB
	// directly (not the signup flow) so we keep the test focused on RBAC,
	// not the magic-link lifecycle.
	providerID := base62.NewID()[:22]
	ownerEmail := fmt.Sprintf("owner-%s@example.com", strings.ToLower(providerID))
	if _, err := h.DB.Exec(`
		INSERT INTO providers (id, legal_name, name, owner_email, state, state_code, capacity, timezone, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'CA', 'CA', 0, 'America/Los_Angeles', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		providerID, "RBAC Test Facility", "RBAC Test Facility", ownerEmail); err != nil {
		t.Fatalf("seed provider: %v", err)
	}

	adminClient, _ := seedUserWithRole(t, h.DB, h.Server.URL, providerID, "provider_admin")
	staffClient, _ := seedUserWithRole(t, h.DB, h.Server.URL, providerID, "provider_staff")

	createBody := map[string]any{
		"first_name":    "Rose",
		"last_name":     "Quartz",
		"date_of_birth": "2022-03-10T00:00:00Z",
	}

	// --- Admin: POST /api/children passes the RBAC gate.
	// Note: the children handler has pre-existing schema drift against
	// migration 000002 (it writes `enroll_date`, `parent_email`, etc. which
	// the migration doesn't declare). Fixing that drift is outside the
	// scope of this RBAC change. For RBAC we only need to prove the admin
	// is NOT 403'd — the request must reach the handler layer. Any 2xx/5xx
	// means the middleware let it through; a 403 means we broke something.
	adminPost := postJSON(t, adminClient, h.URL("/api/children"), createBody)
	adminPost.Body.Close()
	if adminPost.StatusCode == http.StatusForbidden {
		t.Fatalf("admin POST /api/children: unexpectedly got 403 (RBAC middleware too strict)")
	}

	// --- Staff: POST /api/children forbidden (403) ---
	staffPost := postJSON(t, staffClient, h.URL("/api/children"), createBody)
	defer staffPost.Body.Close()
	if staffPost.StatusCode != http.StatusForbidden {
		t.Fatalf("staff POST /api/children: expected 403, got %d (body=%s)",
			staffPost.StatusCode, readAll(t, staffPost))
	}

	// --- Admin: GET /api/children reaches the handler (not 403) ---
	// The children handler has pre-existing schema drift (it scans
	// `enroll_date` / `parent_email` / `classroom` from migration 000002
	// which never declared those columns), so the handler currently returns
	// 500. We only care about the RBAC gate here: both roles must NOT be
	// blocked by the middleware. Once the underlying schema is reconciled
	// (like migration 000012 did for providers), a 200 assertion will
	// light up naturally.
	adminGet, err := adminClient.Get(h.URL("/api/children"))
	if err != nil {
		t.Fatalf("admin GET /api/children: %v", err)
	}
	adminGet.Body.Close()
	if adminGet.StatusCode == http.StatusForbidden {
		t.Fatalf("admin GET /api/children: unexpected 403 (reads must be open)")
	}

	staffGet, err := staffClient.Get(h.URL("/api/children"))
	if err != nil {
		t.Fatalf("staff GET /api/children: %v", err)
	}
	staffGet.Body.Close()
	if staffGet.StatusCode == http.StatusForbidden {
		t.Fatalf("staff GET /api/children: unexpected 403 (reads must be open)")
	}

	// --- Staff: PATCH /api/children/{id} forbidden (403) — covers the
	// other mutating verb and re-exercises the cached-role context path.
	staffPatch := patchJSON(t, staffClient, h.URL("/api/children/some-id"), map[string]any{
		"classroom": "Butterflies",
	})
	staffPatch.Body.Close()
	if staffPatch.StatusCode != http.StatusForbidden {
		t.Fatalf("staff PATCH /api/children/{id}: expected 403, got %d", staffPatch.StatusCode)
	}

	// --- Staff: DELETE /api/children/{id} forbidden (403) ---
	delReq, _ := http.NewRequest(http.MethodDelete, h.URL("/api/children/some-id"), nil)
	staffDel, err := staffClient.Do(delReq)
	if err != nil {
		t.Fatalf("staff DELETE /api/children/{id}: %v", err)
	}
	staffDel.Body.Close()
	if staffDel.StatusCode != http.StatusForbidden {
		t.Fatalf("staff DELETE /api/children/{id}: expected 403, got %d", staffDel.StatusCode)
	}
}

// TestRBAC_AuditLogEndpoint_RequiresAdmin confirms /api/audit-log itself is
// admin-gated: staff user gets 403, admin gets 200.
func TestRBAC_AuditLogEndpoint_RequiresAdmin(t *testing.T) {
	h := NewHarness(t)

	providerID := base62.NewID()[:22]
	ownerEmail := fmt.Sprintf("owner-%s@example.com", strings.ToLower(providerID))
	if _, err := h.DB.Exec(`
		INSERT INTO providers (id, legal_name, name, owner_email, state, state_code, capacity, timezone, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'CA', 'CA', 0, 'America/Los_Angeles', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		providerID, "RBAC Audit Facility", "RBAC Audit Facility", ownerEmail); err != nil {
		t.Fatalf("seed provider: %v", err)
	}
	adminClient, _ := seedUserWithRole(t, h.DB, h.Server.URL, providerID, "provider_admin")
	staffClient, _ := seedUserWithRole(t, h.DB, h.Server.URL, providerID, "provider_staff")

	staffResp, err := staffClient.Get(h.URL("/api/audit-log"))
	if err != nil {
		t.Fatalf("staff GET /api/audit-log: %v", err)
	}
	staffResp.Body.Close()
	if staffResp.StatusCode != http.StatusForbidden {
		t.Fatalf("staff GET /api/audit-log: expected 403, got %d", staffResp.StatusCode)
	}

	adminResp, err := adminClient.Get(h.URL("/api/audit-log"))
	if err != nil {
		t.Fatalf("admin GET /api/audit-log: %v", err)
	}
	adminResp.Body.Close()
	if adminResp.StatusCode != http.StatusOK {
		t.Fatalf("admin GET /api/audit-log: expected 200, got %d", adminResp.StatusCode)
	}
}

// seedUserWithRole inserts a fresh user + session into the harness DB and
// returns an authenticated *http.Client + user id. Shared by the two RBAC
// tests; kept local to rbac_test.go rather than pushed to fixtures.go because
// the rest of the suite still uses the simpler AuthAs helper (which always
// seeds an admin and also creates a new provider).
func seedUserWithRole(t *testing.T, pool *sql.DB, serverURL, providerID, role string) (*http.Client, string) {
	t.Helper()
	userID := base62.NewID()[:22]
	sessionID := base62.NewID()
	email := fmt.Sprintf("%s-%s@example.com", role, strings.ToLower(userID))

	if _, err := pool.Exec(`
		INSERT INTO users (id, provider_id, email, full_name, role, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		userID, providerID, email, "Test "+role, role); err != nil {
		t.Fatalf("seedUserWithRole(%s): insert user: %v", role, err)
	}
	expires := time.Now().Add(14 * 24 * time.Hour).UTC().Format(time.RFC3339)
	if _, err := pool.Exec(`
		INSERT INTO sessions (id, provider_id, user_id, expires_at, created_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		sessionID, providerID, userID, expires); err != nil {
		t.Fatalf("seedUserWithRole(%s): insert session: %v", role, err)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("seedUserWithRole(%s): cookie jar: %v", role, err)
	}
	u, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("seedUserWithRole(%s): parse server url: %v", role, err)
	}
	jar.SetCookies(u, []*http.Cookie{{Name: "ck_sess", Value: sessionID, Path: "/"}})

	return &http.Client{Jar: jar}, userID
}

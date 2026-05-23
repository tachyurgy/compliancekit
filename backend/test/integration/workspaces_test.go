package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
)

type workspaceJSON struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	IsDemo   bool   `json:"isDemo"`
	IsActive bool   `json:"isActive"`
}

type workspacesResp struct {
	Workspaces       []workspaceJSON `json:"workspaces"`
	ActiveProviderID string          `json:"active_provider_id"`
}

// REQ-038: a brand-new authed user starts with one membership (their real
// provider). Listing workspaces returns exactly that one, marked active.
func TestWorkspaces_List_DefaultsToRealProvider(t *testing.T) {
	h := NewHarness(t)
	client, providerID, _ := h.AuthAs(t, "FL")

	resp, err := client.Get(h.URL("/api/me/workspaces"))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, readAll(t, resp))
	}
	var got workspacesResp
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Workspaces) != 1 {
		t.Fatalf("expected 1 workspace, got %d (%+v)", len(got.Workspaces), got.Workspaces)
	}
	w := got.Workspaces[0]
	if w.ID != providerID {
		t.Fatalf("workspace id = %s, want %s", w.ID, providerID)
	}
	if !w.IsActive {
		t.Fatalf("default workspace should be active")
	}
	if w.IsDemo {
		t.Fatalf("default workspace should not be marked demo")
	}
	if got.ActiveProviderID != providerID {
		t.Fatalf("active_provider_id = %s, want %s", got.ActiveProviderID, providerID)
	}
}

// REQ-038: signed-in user with a second (demo) provider attached gets both
// rows, demo last by ORDER BY is_demo ASC.
func TestWorkspaces_List_IncludesDemo(t *testing.T) {
	h := NewHarness(t)
	client, realID, userID := h.AuthAs(t, "FL")

	// Manually attach a demo provider via SQL — this mirrors what
	// demoseed.SeedDemoFacility would do, without spinning up its tx.
	demoID := base62.NewID()[:22]
	if _, err := h.DB.Exec(`
		INSERT INTO providers (id, legal_name, name, state, state_code, timezone, is_demo, created_at, updated_at)
		VALUES (?, ?, ?, 'FL', 'FL', 'America/New_York', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		demoID, "Demo Facility", "Demo Facility"); err != nil {
		t.Fatalf("insert demo provider: %v", err)
	}
	if _, err := h.DB.Exec(`
		INSERT INTO provider_memberships (id, user_id, provider_id, role, created_at)
		VALUES (?, ?, ?, 'provider_admin', CURRENT_TIMESTAMP)`,
		base62.NewID()[:22], userID, demoID); err != nil {
		t.Fatalf("insert demo membership: %v", err)
	}

	resp, err := client.Get(h.URL("/api/me/workspaces"))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, readAll(t, resp))
	}
	var got workspacesResp
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Workspaces) != 2 {
		t.Fatalf("expected 2 workspaces, got %d", len(got.Workspaces))
	}
	var sawReal, sawDemo bool
	for _, w := range got.Workspaces {
		if w.ID == realID && !w.IsDemo {
			sawReal = true
			if !w.IsActive {
				t.Fatalf("real workspace should still be active by default")
			}
		}
		if w.ID == demoID && w.IsDemo {
			sawDemo = true
			if w.IsActive {
				t.Fatalf("demo workspace should not be active by default")
			}
		}
	}
	if !sawReal || !sawDemo {
		t.Fatalf("expected both real + demo workspaces, got %+v", got.Workspaces)
	}
}

// REQ-038: switching to a provider the user IS a member of returns 200
// and the next request sees the new provider via LookupSession.
func TestWorkspaces_SetActive_HappyPath(t *testing.T) {
	h := NewHarness(t)
	client, realID, userID := h.AuthAs(t, "FL")

	demoID := base62.NewID()[:22]
	if _, err := h.DB.Exec(`
		INSERT INTO providers (id, legal_name, name, state, state_code, timezone, is_demo, created_at, updated_at)
		VALUES (?, ?, ?, 'FL', 'FL', 'America/New_York', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		demoID, "Demo Facility", "Demo Facility"); err != nil {
		t.Fatalf("insert demo provider: %v", err)
	}
	if _, err := h.DB.Exec(`
		INSERT INTO provider_memberships (id, user_id, provider_id, role, created_at)
		VALUES (?, ?, ?, 'provider_admin', CURRENT_TIMESTAMP)`,
		base62.NewID()[:22], userID, demoID); err != nil {
		t.Fatalf("insert demo membership: %v", err)
	}

	body := `{"providerId":"` + demoID + `"}`
	resp, err := client.Post(h.URL("/api/me/active-workspace"), "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, readAll(t, resp))
	}

	// Subsequent /api/me/workspaces should now mark demo active.
	r2, err := client.Get(h.URL("/api/me/workspaces"))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer r2.Body.Close()
	var got workspacesResp
	if err := json.NewDecoder(r2.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ActiveProviderID != demoID {
		t.Fatalf("active_provider_id = %s, want %s (real=%s)", got.ActiveProviderID, demoID, realID)
	}
	for _, w := range got.Workspaces {
		if w.ID == demoID && !w.IsActive {
			t.Fatalf("demo workspace should be marked active after switch")
		}
		if w.ID == realID && w.IsActive {
			t.Fatalf("real workspace should no longer be active after switch")
		}
	}
}

// REQ-038: a switch to a provider this user does NOT belong to returns 403.
// Without this the active-workspace endpoint would be a tenant-hopping hole.
func TestWorkspaces_SetActive_RejectsForeignProvider(t *testing.T) {
	h := NewHarness(t)
	client, _, _ := h.AuthAs(t, "FL")

	// A second provider with no membership for this user.
	foreignID := base62.NewID()[:22]
	if _, err := h.DB.Exec(`
		INSERT INTO providers (id, legal_name, state, state_code, timezone, created_at, updated_at)
		VALUES (?, 'Stranger Facility', 'FL', 'FL', 'America/New_York', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		foreignID); err != nil {
		t.Fatalf("insert foreign provider: %v", err)
	}

	body := `{"providerId":"` + foreignID + `"}`
	resp, err := client.Post(h.URL("/api/me/active-workspace"), "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d (body=%s)", resp.StatusCode, readAll(t, resp))
	}
}

// REQ-038: posting an empty body is a 400.
func TestWorkspaces_SetActive_RequiresProviderID(t *testing.T) {
	h := NewHarness(t)
	client, _, _ := h.AuthAs(t, "FL")
	resp, err := client.Post(h.URL("/api/me/active-workspace"), "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// REQ-038: /api/demo/purge is a no-op when the active workspace is real.
func TestDemo_Purge_NoopOnRealProvider(t *testing.T) {
	h := NewHarness(t)
	client, realID, _ := h.AuthAs(t, "FL")

	// Sprinkle a couple of (incorrectly) is_demo=1 rows on the real provider
	// to simulate legacy state from the old REQ-024 behavior.
	if _, err := h.DB.Exec(`
		INSERT INTO staff (id, provider_id, first_name, last_name, role, status, is_demo, created_at, updated_at)
		VALUES (?, ?, 'Legacy', 'Demo', 'lead_teacher', 'active', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		base62.NewID()[:22], realID); err != nil {
		t.Fatalf("seed legacy demo row: %v", err)
	}

	resp, err := client.Post(h.URL("/api/demo/purge"), "application/json", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, readAll(t, resp))
	}
	// The legacy row must still be there — we never touch real providers
	// from this endpoint.
	var n int
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM staff WHERE provider_id = ? AND is_demo = 1`, realID).Scan(&n)
	if n != 1 {
		t.Fatalf("real-provider demo rows = %d after no-op purge, want 1", n)
	}
}

// REQ-038: /api/demo/purge wipes the demo provider's rows when active.
func TestDemo_Purge_HappyPath(t *testing.T) {
	h := NewHarness(t)
	client, _, userID := h.AuthAs(t, "FL")

	// Spin up a real demo provider via the seeder, attach + switch to it.
	demoID := base62.NewID()[:22]
	if _, err := h.DB.Exec(`
		INSERT INTO providers (id, legal_name, name, state, state_code, timezone, is_demo, created_at, updated_at)
		VALUES (?, ?, ?, 'FL', 'FL', 'America/New_York', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		demoID, "Demo Facility", "Demo Facility"); err != nil {
		t.Fatalf("insert demo provider: %v", err)
	}
	if _, err := h.DB.Exec(`
		INSERT INTO provider_memberships (id, user_id, provider_id, role, created_at)
		VALUES (?, ?, ?, 'provider_admin', CURRENT_TIMESTAMP)`,
		base62.NewID()[:22], userID, demoID); err != nil {
		t.Fatalf("insert membership: %v", err)
	}
	if _, err := h.DB.Exec(`
		INSERT INTO staff (id, provider_id, first_name, last_name, role, status, is_demo, created_at, updated_at)
		VALUES (?, ?, 'Demo', 'Person', 'lead_teacher', 'active', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		base62.NewID()[:22], demoID); err != nil {
		t.Fatalf("insert demo staff: %v", err)
	}

	switchBody := `{"providerId":"` + demoID + `"}`
	if r, err := client.Post(h.URL("/api/me/active-workspace"), "application/json", strings.NewReader(switchBody)); err != nil || r.StatusCode != 200 {
		t.Fatalf("switch failed: err=%v status=%v", err, r)
	}

	resp, err := client.Post(h.URL("/api/demo/purge"), "application/json", nil)
	if err != nil {
		t.Fatalf("POST purge: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, readAll(t, resp))
	}
	var n int
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM staff WHERE provider_id = ?`, demoID).Scan(&n)
	if n != 0 {
		t.Fatalf("demo staff after purge = %d, want 0", n)
	}
}

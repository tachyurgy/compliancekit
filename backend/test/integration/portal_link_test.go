package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
)

// TestParentPortalLink_MintsURLForChild mints a parent portal link for an
// existing child and verifies the shape of the response + that the URL
// actually contains a token parsable by the magic-link service.
func TestParentPortalLink_MintsURLForChild(t *testing.T) {
	h := NewHarness(t)
	client, providerID, _ := h.AuthAs(t, "CA")

	// Seed a child directly (faster than driving the create handler).
	childID := base62.NewID()[:22]
	if _, err := h.DB.Exec(`
		INSERT INTO children (id, provider_id, first_name, last_name, date_of_birth,
		                     enrollment_date, enroll_date, guardians, parent_email, status,
		                     created_at, updated_at)
		VALUES (?,?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,'[]','parent@example.com','enrolled',
		         CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		childID, providerID, "Kiddo", "K", "2022-01-01"); err != nil {
		t.Fatalf("seed child: %v", err)
	}

	resp := postJSON(t, client, h.URL("/api/children/"+childID+"/portal-link"), map[string]any{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", resp.StatusCode, readAll(t, resp))
	}
	var out struct {
		URL         string `json:"url"`
		ExpiresAt   string `json:"expires_at"`
		SubjectID   string `json:"subject_id"`
		SubjectKind string `json:"subject_kind"`
		Emailed     bool   `json:"emailed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.SubjectID != childID || out.SubjectKind != "child" {
		t.Fatalf("unexpected subject: %+v", out)
	}
	if !strings.Contains(out.URL, "/u/") {
		t.Fatalf("url should hit /u/{token}; got %q", out.URL)
	}
	if out.ExpiresAt == "" {
		t.Fatalf("expires_at must be set")
	}
	// No emailer in harness → emailed=false even if send=email were requested.
	if out.Emailed {
		t.Fatalf("emailed should be false when no emailer is wired")
	}

	// A magic_link_tokens row should exist with kind=parent_upload +
	// subject_id=childID + provider_id=providerID.
	var n int
	if err := h.DB.QueryRow(`
		SELECT COUNT(*) FROM magic_link_tokens
		WHERE kind = 'parent_upload' AND subject_id = ? AND provider_id = ? AND consumed_at IS NULL`,
		childID, providerID).Scan(&n); err != nil {
		t.Fatalf("count tokens: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 live parent_upload token, got %d", n)
	}
}

func TestStaffPortalLink_MintsURLForStaff(t *testing.T) {
	h := NewHarness(t)
	client, providerID, _ := h.AuthAs(t, "CA")

	staffID := base62.NewID()[:22]
	if _, err := h.DB.Exec(`
		INSERT INTO staff (id, provider_id, first_name, last_name, role, email, hired_on, hire_date, status, created_at, updated_at)
		VALUES (?,?,?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,'active',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		staffID, providerID, "Staffer", "S", "lead_teacher", "staff@example.com"); err != nil {
		t.Fatalf("seed staff: %v", err)
	}

	resp := postJSON(t, client, h.URL("/api/staff/"+staffID+"/portal-link"), map[string]any{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", resp.StatusCode, readAll(t, resp))
	}
	var out struct {
		URL         string `json:"url"`
		SubjectKind string `json:"subject_kind"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.SubjectKind != "staff" || !strings.Contains(out.URL, "/u/") {
		t.Fatalf("unexpected response: %+v", out)
	}
}

func TestPortalLink_RequiresAdmin(t *testing.T) {
	h := NewHarness(t)
	staffClient, _, _ := h.AuthAsStaff(t, "CA")
	// We don't need a real child; RBAC runs before the DB lookup.
	resp := postJSON(t, staffClient, h.URL("/api/children/any-id/portal-link"), map[string]any{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("staff POST: expected 403, got %d", resp.StatusCode)
	}
}

func TestPortalLink_NotFoundForForeignChild(t *testing.T) {
	h := NewHarness(t)
	client, _, _ := h.AuthAs(t, "CA")

	// Child with a made-up ID that doesn't belong to this provider.
	resp := postJSON(t, client, h.URL("/api/children/does-not-exist/portal-link"), map[string]any{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestPortalLink_TenantIsolation(t *testing.T) {
	h := NewHarness(t)

	// Provider A seeds a child.
	_, providerA, _ := h.AuthAs(t, "CA")
	childID := base62.NewID()[:22]
	if _, err := h.DB.Exec(`
		INSERT INTO children (id, provider_id, first_name, last_name, date_of_birth,
		                     enrollment_date, enroll_date, guardians, status,
		                     created_at, updated_at)
		VALUES (?,?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,'[]','enrolled',
		         CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		childID, providerA, "A", "A", "2022-01-01"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Provider B (different tenant) tries to mint a link for A's child → 404.
	clientB, _, _ := h.AuthAs(t, "TX")
	resp := postJSON(t, clientB, h.URL("/api/children/"+childID+"/portal-link"), map[string]any{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-tenant: expected 404, got %d", resp.StatusCode)
	}
}

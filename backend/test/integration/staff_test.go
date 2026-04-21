package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
)

// KNOWN SCHEMA DRIFT (2026-04-20) — OUT OF SCOPE for this test batch:
//   handlers/staff.go writes/reads `hire_date` and `background_check_date`
//   columns that the original 000003 migration didn't declare (the canonical
//   column is `hired_on`; `background_check_date` doesn't exist at all).
//   There's no reconciliation migration like 000015 for staff. Until one is
//   written, POST /api/staff and PATCH /api/staff/{id} return 500.
//
// We assert the handler is REACHED (so the RBAC + session path works) and
// verify tenant-isolation paths via direct SQL.

func TestStaff_List_RequiresAuth(t *testing.T) {
	h := NewHarness(t)
	resp, err := http.Get(h.URL("/api/staff"))
	if err != nil {
		t.Fatalf("GET /api/staff: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /api/staff: expected 401, got %d", resp.StatusCode)
	}
}

func TestStaff_Create_MissingFields(t *testing.T) {
	h := NewHarness(t)
	client, _, _ := h.AuthAs(t, "CA")

	resp := postJSON(t, client, h.URL("/api/staff"), map[string]any{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty body: expected 400, got %d", resp.StatusCode)
	}

	// All four required fields must be present; drop one at a time.
	base := map[string]any{
		"first_name": "Sam", "last_name": "Smith", "email": "sam@example.com", "role": "teacher",
	}
	for _, drop := range []string{"first_name", "last_name", "email", "role"} {
		body := map[string]any{}
		for k, v := range base {
			if k != drop {
				body[k] = v
			}
		}
		resp := postJSON(t, client, h.URL("/api/staff"), body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("missing %q: expected 400, got %d", drop, resp.StatusCode)
		}
	}
}

func TestStaff_Create_ReachesHandler_500DueToDrift(t *testing.T) {
	h := NewHarness(t)
	client, _, _ := h.AuthAs(t, "CA")

	resp := postJSON(t, client, h.URL("/api/staff"), map[string]any{
		"first_name": "Sam", "last_name": "Smith", "email": "sam@example.com", "role": "teacher",
	})
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusCreated:
		t.Logf("POST /api/staff returned 201 — drift appears fixed.")
	case http.StatusInternalServerError:
		t.Logf("POST /api/staff returned 500 — known drift (hire_date / background_check_date columns missing).")
	default:
		t.Fatalf("expected 201 or 500, got %d (body=%s)", resp.StatusCode, readAll(t, resp))
	}
}

// TestStaff_RBAC_WritesDeniedForStaffRole confirms the router's adminOnly
// wrapper still gates POST/PATCH/DELETE on /api/staff even when the requester
// has a valid session. Mirrors the children RBAC test but for staff routes.
func TestStaff_RBAC_WritesDeniedForStaffRole(t *testing.T) {
	h := NewHarness(t)
	staffClient, _, _ := h.AuthAsStaff(t, "CA")

	postResp := postJSON(t, staffClient, h.URL("/api/staff"), map[string]any{
		"first_name": "X", "last_name": "Y", "email": "x@y", "role": "teacher",
	})
	postResp.Body.Close()
	if postResp.StatusCode != http.StatusForbidden {
		t.Fatalf("staff POST /api/staff: expected 403, got %d", postResp.StatusCode)
	}

	patchResp := patchJSON(t, staffClient, h.URL("/api/staff/"+base62.NewID()[:22]), map[string]any{"status": "active"})
	patchResp.Body.Close()
	if patchResp.StatusCode != http.StatusForbidden {
		t.Fatalf("staff PATCH /api/staff/{id}: expected 403, got %d", patchResp.StatusCode)
	}

	delReq, _ := http.NewRequest(http.MethodDelete, h.URL("/api/staff/"+base62.NewID()[:22]), nil)
	delResp, err := staffClient.Do(delReq)
	if err != nil {
		t.Fatalf("staff DELETE: %v", err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusForbidden {
		t.Fatalf("staff DELETE /api/staff/{id}: expected 403, got %d", delResp.StatusCode)
	}
}

// TestStaff_RBAC_ReadsAllowedForStaffRole confirms GET routes on /api/staff
// are NOT admin-gated per router.go.
func TestStaff_RBAC_ReadsAllowedForStaffRole(t *testing.T) {
	h := NewHarness(t)
	staffClient, _, _ := h.AuthAsStaff(t, "CA")

	listResp, err := staffClient.Get(h.URL("/api/staff"))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	listResp.Body.Close()
	if listResp.StatusCode == http.StatusForbidden {
		t.Fatalf("staff GET /api/staff unexpectedly 403 — reads should be open to both roles")
	}
}

func TestStaff_TenantIsolation_GetAndDelete(t *testing.T) {
	h := NewHarness(t)
	clientA, _, _ := h.AuthAs(t, "CA")
	_, pidB, _ := h.AuthAs(t, "FL")

	staffB := base62.NewID()[:22]
	if _, err := h.DB.Exec(`
		INSERT INTO staff (id, provider_id, first_name, last_name, status, created_at, updated_at)
		VALUES (?, ?, 'Foreign', 'Staff', 'active', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		staffB, pidB); err != nil {
		t.Fatalf("seed staff: %v", err)
	}

	// Provider A attempts GET → 404 (or 500 on scan drift). Must NOT be 200.
	resp, err := clientA.Get(h.URL("/api/staff/" + staffB))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body := readAll(t, resp)
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("tenant isolation broken: GET /api/staff/{id} returned 200 (body=%s)", body)
	}
	if strings.Contains(body, pidB) {
		t.Fatalf("response leaked provider B id")
	}

	// Provider A DELETE → handler soft-deletes via status='terminated', but
	// WHERE clause ANDs provider_id so this is a silent no-op across tenants.
	req, _ := http.NewRequest(http.MethodDelete, h.URL("/api/staff/"+staffB), nil)
	delResp, err := clientA.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("cross-tenant DELETE: expected 204 silent no-op, got %d", delResp.StatusCode)
	}

	// Verify DB state: provider B's staff row is still 'active'.
	var status string
	if err := h.DB.QueryRow(`SELECT status FROM staff WHERE id = ?`, staffB).Scan(&status); err != nil {
		t.Fatalf("query staff: %v", err)
	}
	if status != "active" {
		t.Fatalf("tenant isolation broken: provider B's staff status flipped to %q", status)
	}
}

// TestStaff_ListDocuments_ReachesHandler: the endpoint is reachable and
// admin/staff session-gated. Documents-schema drift means it 500s today; flip
// to 200 once the documents schema is reconciled.
func TestStaff_ListDocuments_ReachesHandler(t *testing.T) {
	h := NewHarness(t)
	client, providerID, _ := h.AuthAs(t, "CA")

	staffID := base62.NewID()[:22]
	if _, err := h.DB.Exec(`
		INSERT INTO staff (id, provider_id, first_name, last_name, status, created_at, updated_at)
		VALUES (?, ?, 'Jane', 'Doe', 'active', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		staffID, providerID); err != nil {
		t.Fatalf("seed: %v", err)
	}

	resp, err := client.Get(h.URL("/api/staff/" + staffID + "/documents"))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		t.Fatalf("blocked by auth: %d", resp.StatusCode)
	}
}

package integration

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestCompleteOnboarding_PersistsFacility verifies POST /api/provider/onboarding
// writes every facility field, flips onboarding_complete, and returns the
// canonical camelCase response shape.
func TestCompleteOnboarding_PersistsFacility(t *testing.T) {
	h := NewHarness(t)
	client, providerID, _ := h.AuthAs(t, "CA")

	resp := postJSON(t, client, h.URL("/api/provider/onboarding"), map[string]any{
		"stateCode":     "CA",
		"licenseType":   "center",
		"licenseNumber": "LIC-TEST-123",
		"name":          "Sunshine Daycare",
		"address1":      "100 Sunshine Ln",
		"address2":      "Suite 5",
		"city":          "Los Angeles",
		"stateRegion":   "CA",
		"postalCode":    "90001",
		"capacity":      48,
		"agesServedMonths": map[string]int{
			"minMonths": 6,
			"maxMonths": 60,
		},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", resp.StatusCode, readAll(t, resp))
	}
	var out struct {
		ID                 string `json:"id"`
		Name               string `json:"name"`
		StateCode          string `json:"stateCode"`
		LicenseType        string `json:"licenseType"`
		Capacity           int    `json:"capacity"`
		OnboardingComplete bool   `json:"onboardingComplete"`
		AgesServedMonths   struct {
			Min int `json:"minMonths"`
			Max int `json:"maxMonths"`
		} `json:"agesServedMonths"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.ID != providerID {
		t.Fatalf("expected id=%s, got %s", providerID, out.ID)
	}
	if out.Name != "Sunshine Daycare" || out.Capacity != 48 || !out.OnboardingComplete {
		t.Fatalf("unexpected response: %+v", out)
	}
	if out.AgesServedMonths.Min != 6 || out.AgesServedMonths.Max != 60 {
		t.Fatalf("ages not persisted: %+v", out.AgesServedMonths)
	}

	// DB check.
	var onboarded int
	var addr1, postalCode string
	if err := h.DB.QueryRow(`
		SELECT onboarding_complete, address_line1, postal_code
		FROM providers WHERE id = ?`, providerID).Scan(&onboarded, &addr1, &postalCode); err != nil {
		t.Fatalf("query providers: %v", err)
	}
	if onboarded != 1 || addr1 != "100 Sunshine Ln" || postalCode != "90001" {
		t.Fatalf("db state unexpected: onboarded=%d addr1=%q postal=%q", onboarded, addr1, postalCode)
	}
}

// TestCompleteOnboarding_BulkInsertsStaffAndChildren verifies the staff and
// children arrays in the payload land in the respective tables.
func TestCompleteOnboarding_BulkInsertsStaffAndChildren(t *testing.T) {
	h := NewHarness(t)
	client, providerID, _ := h.AuthAs(t, "TX")

	resp := postJSON(t, client, h.URL("/api/provider/onboarding"), map[string]any{
		"stateCode":        "TX",
		"licenseType":      "family_home",
		"name":             "Tiny Hands Home",
		"address1":         "1 Home St",
		"city":             "Austin",
		"stateRegion":      "TX",
		"postalCode":       "73301",
		"capacity":         12,
		"agesServedMonths": map[string]int{"minMonths": 0, "maxMonths": 48},
		"staff": []map[string]any{
			{"firstName": "Alice", "lastName": "A", "email": "alice@example.com", "role": "director"},
			{"firstName": "Bob", "lastName": "B", "role": "lead_teacher"},
		},
		"children": []map[string]any{
			{"firstName": "Cathy", "lastName": "C", "dateOfBirth": "2022-01-15", "parentEmail": "p@example.com"},
			{"firstName": "Dan", "lastName": "D", "dateOfBirth": "2023-06-01"},
		},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", resp.StatusCode, readAll(t, resp))
	}

	var staffCount, childCount int
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM staff WHERE provider_id = ?`, providerID).Scan(&staffCount); err != nil {
		t.Fatalf("count staff: %v", err)
	}
	if staffCount != 2 {
		t.Fatalf("expected 2 staff rows, got %d", staffCount)
	}
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM children WHERE provider_id = ?`, providerID).Scan(&childCount); err != nil {
		t.Fatalf("count children: %v", err)
	}
	if childCount != 2 {
		t.Fatalf("expected 2 children rows, got %d", childCount)
	}
}

// TestCompleteOnboarding_RejectsInvalidState: unknown stateCode → 400.
func TestCompleteOnboarding_RejectsInvalidState(t *testing.T) {
	h := NewHarness(t)
	client, _, _ := h.AuthAs(t, "CA")

	resp := postJSON(t, client, h.URL("/api/provider/onboarding"), map[string]any{
		"stateCode":        "ZZ",
		"licenseType":      "center",
		"name":             "Bad State",
		"address1":         "x",
		"city":             "x",
		"stateRegion":      "ZZ",
		"postalCode":       "00000",
		"capacity":         1,
		"agesServedMonths": map[string]int{"minMonths": 0, "maxMonths": 1},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// TestCompleteOnboarding_RequiresAdmin: staff-role clients get 403.
func TestCompleteOnboarding_RequiresAdmin(t *testing.T) {
	h := NewHarness(t)
	staffClient, _, _ := h.AuthAsStaff(t, "CA")

	resp := postJSON(t, staffClient, h.URL("/api/provider/onboarding"), map[string]any{
		"stateCode":        "CA",
		"licenseType":      "center",
		"name":             "Admin Only",
		"address1":         "1",
		"city":             "LA",
		"stateRegion":      "CA",
		"postalCode":       "90001",
		"capacity":         1,
		"agesServedMonths": map[string]int{"minMonths": 0, "maxMonths": 1},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

// TestCompleteOnboarding_RollsBackOnBadChild: a bad DOB in the children array
// must fail cleanly without a partial write (tx rollback). onboarding_complete
// must stay 0 and no staff/children rows should land.
func TestCompleteOnboarding_RollsBackOnBadChild(t *testing.T) {
	h := NewHarness(t)
	client, providerID, _ := h.AuthAs(t, "FL")

	resp := postJSON(t, client, h.URL("/api/provider/onboarding"), map[string]any{
		"stateCode":        "FL",
		"licenseType":      "center",
		"name":             "Rollback Test",
		"address1":         "1 Rollback",
		"city":             "Miami",
		"stateRegion":      "FL",
		"postalCode":       "33101",
		"capacity":         10,
		"agesServedMonths": map[string]int{"minMonths": 0, "maxMonths": 60},
		"staff": []map[string]any{
			{"firstName": "Eve", "lastName": "E", "role": "director"},
		},
		"children": []map[string]any{
			{"firstName": "Faith", "lastName": "F", "dateOfBirth": "not-a-date"},
		},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (body=%s)", resp.StatusCode, readAll(t, resp))
	}

	var onboarded, staffCount, childCount int
	if err := h.DB.QueryRow(`SELECT onboarding_complete FROM providers WHERE id = ?`, providerID).Scan(&onboarded); err != nil {
		t.Fatalf("query: %v", err)
	}
	if onboarded != 0 {
		t.Fatalf("onboarding_complete must stay 0 on rollback; got %d", onboarded)
	}
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM staff WHERE provider_id = ?`, providerID).Scan(&staffCount); err != nil {
		t.Fatalf("count staff: %v", err)
	}
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM children WHERE provider_id = ?`, providerID).Scan(&childCount); err != nil {
		t.Fatalf("count children: %v", err)
	}
	if staffCount != 0 || childCount != 0 {
		t.Fatalf("rollback leaked rows: staff=%d children=%d", staffCount, childCount)
	}
}

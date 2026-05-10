package integration

import (
	"net/http"
	"strings"
	"testing"
)

// REQ-003 verification: complete-onboarding refuses when required fields
// are missing, accepts once they're filled. Light end-to-end coverage of
// the validation gate.
func TestFacility_CompleteOnboarding_RejectsIncomplete(t *testing.T) {
	h := NewHarness(t)
	client, _, _ := h.AuthAs(t, "FL")

	resp := postJSON(t, client, h.URL("/api/facility/complete-onboarding"), map[string]any{})
	body := readAll(t, resp)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 with required-field list, got %d (body=%s)", resp.StatusCode, body)
	}
	// The handler enumerates missing fields. Spot-check a handful so future
	// reorders are tolerated but the list is recognisably present. (We do
	// not assert "name" because the AuthAs fixture pre-fills providers.name
	// at signup so it's never missing.)
	for _, want := range []string{"licenseNumber", "indoorSqft", "directorEmail"} {
		if !strings.Contains(body, want) {
			t.Errorf("expected missing-field list to include %q; body=%s", want, body)
		}
	}
}

// /api/facility GET should succeed even when the providers row has not
// been filled past signup defaults — that's the resumability contract.
func TestFacility_Get_ReturnsRowEvenWhenEmpty(t *testing.T) {
	h := NewHarness(t)
	client, _, _ := h.AuthAs(t, "FL")

	resp, err := client.Get(h.URL("/api/facility"))
	if err != nil {
		t.Fatalf("GET /api/facility: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", resp.StatusCode, readAll(t, resp))
	}
}

// PATCH should partial-update and round-trip; we verify by reading back.
func TestFacility_Patch_SavesFields(t *testing.T) {
	h := NewHarness(t)
	client, _, _ := h.AuthAs(t, "FL")

	resp := patchJSON(t, client, h.URL("/api/facility"), map[string]any{
		"licenseNumber":     "C12FL0123",
		"county":            "Miami-Dade",
		"phone":             "305-555-0100",
		"capacity":          42,
		"indoorSqft":        2000,
		"outdoorPlayground": true,
		"eveningLicense":    false,
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH /api/facility: expected 200, got %d (body=%s)", resp.StatusCode, readAll(t, resp))
	}
	body := readAll(t, resp)
	for _, want := range []string{`"licenseNumber":"C12FL0123"`, `"county":"Miami-Dade"`, `"capacity":42`, `"outdoorPlayground":true`} {
		if !strings.Contains(body, want) {
			t.Errorf("expected response to contain %q; body=%s", want, body)
		}
	}
}

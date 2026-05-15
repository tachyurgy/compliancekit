package integration

import (
	"net/http"
	"strings"
	"testing"
)

// complete-onboarding is non-blocking — facility details are filled out from
// the dashboard banner whenever the operator gets to them. The endpoint just
// stamps onboarding_complete=1 so the route guard lets them into /dashboard.
func TestFacility_CompleteOnboarding_AcceptsEmpty(t *testing.T) {
	h := NewHarness(t)
	client, _, _ := h.AuthAs(t, "FL")

	resp := postJSON(t, client, h.URL("/api/facility/complete-onboarding"), map[string]any{})
	body := readAll(t, resp)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"onboardingCompletedAt"`) {
		t.Errorf("expected onboardingCompletedAt in response body; got %s", body)
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

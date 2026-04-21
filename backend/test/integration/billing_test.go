package integration

import (
	"net/http"
	"testing"
)

// The BillingHandler requires a *billing.Service to be non-nil to call Stripe.
// The shared harness intentionally leaves it nil (no network). The tests here
// document the two observable states:
//
//   1. With a nil billing.Service, POST /api/billing/checkout and
//      /api/billing/portal panic on method call; chi's Recoverer serves 500.
//   2. RBAC on billing routes is open to both admin AND staff (no adminOnly
//      wrapper on the /billing subroute) — writes still require a session
//      but not the provider_admin role.
//
// Integration-testing real Stripe API calls is OUT OF SCOPE — it requires
// either a stripe-mock server or recorded HTTP fixtures. The Stripe SDK does
// not expose a public interface for the stripe-go customer.New etc. calls
// that Service uses, so we can't swap them out for a fake via our code.

func TestBilling_Checkout_RequiresAuth(t *testing.T) {
	h := NewHarness(t)
	resp, err := http.Post(h.URL("/api/billing/checkout"), "application/json", stringReader(`{}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestBilling_Portal_RequiresAuth(t *testing.T) {
	h := NewHarness(t)
	resp, err := http.Post(h.URL("/api/billing/portal"), "application/json", stringReader(`{}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

// TestBilling_Checkout_NilBillingService_Returns500: the shared harness does
// not wire a billing.Service into BillingHandler.Billing, so the handler
// nil-derefs when invoked. chi's Recoverer returns 500.
func TestBilling_Checkout_NilBillingService_Returns500(t *testing.T) {
	h := NewHarness(t)
	client, _, _ := h.AuthAs(t, "CA")

	resp := postJSON(t, client, h.URL("/api/billing/checkout"), map[string]any{})
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("nil billing: expected 500, got %d", resp.StatusCode)
	}
}

func TestBilling_Portal_NilBillingService_Returns500(t *testing.T) {
	h := NewHarness(t)
	client, _, _ := h.AuthAs(t, "CA")

	resp := postJSON(t, client, h.URL("/api/billing/portal"), map[string]any{})
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("nil billing: expected 500, got %d", resp.StatusCode)
	}
}

// TestBilling_RoutesOpenToStaff: /api/billing/* is NOT admin-gated in
// router.go. Staff sessions reach the handler (which then 500s on nil
// Stripe). This documents current routing; flip to adminOnly if policy
// changes.
func TestBilling_RoutesOpenToStaff(t *testing.T) {
	h := NewHarness(t)
	staffClient, _, _ := h.AuthAsStaff(t, "CA")

	resp := postJSON(t, staffClient, h.URL("/api/billing/checkout"), map[string]any{})
	resp.Body.Close()
	// Not 403 — staff is allowed through RBAC. 500 because Stripe is nil.
	if resp.StatusCode == http.StatusForbidden {
		t.Fatalf("staff /api/billing/checkout unexpectedly 403; router may have been changed to adminOnly without updating this test")
	}
}

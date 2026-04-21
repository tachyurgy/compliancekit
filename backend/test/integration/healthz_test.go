package integration

import (
	"io"
	"net/http"
	"testing"
)

// TestHealthz_ReturnsOK confirms /healthz returns 200 with the body "ok".
// The existing auth_test.go already covers 200 status; this test adds
// assertions on the body and that the route is NOT accidentally
// authenticated.
func TestHealthz_ReturnsOK(t *testing.T) {
	h := NewHarness(t)

	resp, err := http.Get(h.URL("/healthz"))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(b) != "ok" {
		t.Fatalf(`expected body="ok", got %q`, string(b))
	}
}

// TestHealthz_NoAuth confirms /healthz does not require a session cookie.
// This is important for uptime monitors — they must be able to probe the
// endpoint without credentials.
func TestHealthz_NoAuth(t *testing.T) {
	h := NewHarness(t)

	req, _ := http.NewRequest(http.MethodGet, h.URL("/healthz"), nil)
	// Deliberately no cookie.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("anonymous healthz: expected 200, got %d", resp.StatusCode)
	}
}

// TestReadyz_NotMounted documents that /readyz is currently NOT a registered
// route. Uptime monitors configured to probe /readyz will get 404. Remove
// this test (or flip to 200) once the route is added.
func TestReadyz_NotMounted(t *testing.T) {
	h := NewHarness(t)

	resp, err := http.Get(h.URL("/readyz"))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for unmounted /readyz, got %d", resp.StatusCode)
	}
}

package integration

import (
	"net/http"
	"testing"
)

// The harness configures CORS with AllowedOrigins = ["http://localhost:5173"]
// (see fixtures.go). We verify:
//   - preflight from the allowed origin gets an Access-Control-Allow-Origin
//   - preflight from a disallowed origin does NOT get CORS headers
//   - credentials are allowed (AllowCredentials: true)

// TestCORS_Preflight_AllowedOrigin sends an OPTIONS request with
// Origin: http://localhost:5173 and verifies the CORS response headers.
func TestCORS_Preflight_AllowedOrigin(t *testing.T) {
	h := NewHarness(t)

	req, _ := http.NewRequest(http.MethodOptions, h.URL("/api/children"), nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS: %v", err)
	}
	resp.Body.Close()

	// go-chi/cors returns 204 for preflight.
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 204 (or 200) for allowed-origin preflight, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("expected Access-Control-Allow-Origin=http://localhost:5173, got %q", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected Access-Control-Allow-Credentials=true, got %q", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Methods"); got == "" {
		t.Fatalf("expected non-empty Access-Control-Allow-Methods")
	}
}

// TestCORS_Preflight_DisallowedOrigin: foreign origin gets NO
// Access-Control-Allow-Origin header. go-chi/cors returns the preflight
// without the allowed-origin header (some CORS middlewares return 403;
// go-chi's behaviour is silent omission).
func TestCORS_Preflight_DisallowedOrigin(t *testing.T) {
	h := NewHarness(t)

	req, _ := http.NewRequest(http.MethodOptions, h.URL("/api/children"), nil)
	req.Header.Set("Origin", "https://evil.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS: %v", err)
	}
	resp.Body.Close()

	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("disallowed origin must not receive Access-Control-Allow-Origin; got %q", got)
	}
}

// TestCORS_SameOrigin_NoEcho: a request without an Origin header (e.g. from
// curl) must not accidentally get a wildcard CORS header.
func TestCORS_NoOriginHeader_NoEcho(t *testing.T) {
	h := NewHarness(t)

	resp, err := http.Get(h.URL("/healthz"))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unexpected CORS header on Origin-less request: %q", got)
	}
}

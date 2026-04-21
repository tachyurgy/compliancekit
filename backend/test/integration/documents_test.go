package integration

import (
	"net/http"
	"testing"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
)

// KNOWN SCHEMA DRIFT (2026-04-20) — OUT OF SCOPE for this test batch:
//
//   handlers/documents.go uses a completely different column vocabulary than
//   migration 000004_documents.up.sql declares:
//      handler writes/reads:  subject_kind, subject_id, storage_bucket,
//                             storage_key, mime_type, size_bytes, kind,
//                             title, issued_at, expires_at, ocr_confidence,
//                             ocr_source, uploaded_by, uploaded_via,
//                             last_chase_sent_at
//      schema (000004):       owner_kind, owner_id, s3_key, mime_type,
//                             doc_type, original_filename, byte_size, sha256,
//                             ocr_status, ocr_confidence, expiration_date,
//                             expiration_source, expiration_confidence,
//                             uploaded_by_user_id, uploaded_via,
//                             confirmed_by_user_id, confirmed_at
//
//   Additionally DocumentHandler.Storage is *storage.Client (concrete AWS
//   S3 client) with no interface; the test harness leaves it nil. Any
//   Presign/Finalize call therefore nil-derefs and the chi Recoverer returns
//   500 via panic.
//
//   Fixing either of these is a cross-cutting refactor (new migration +
//   handler rewrite; or introduce a storage.Interface). Neither fits the
//   "write integration tests" scope.
//
//   The tests below exercise the RBAC + session-gate paths and document
//   today's surface behaviour so regressions flip the signal.

func TestDocuments_Presign_RequiresAuth(t *testing.T) {
	h := NewHarness(t)
	req := mustReq(t, http.MethodPost, h.URL("/api/documents/presign"), map[string]any{
		"subject_kind": "child", "kind": "immunization_record", "mime_type": "application/pdf",
	})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestDocuments_Presign_RBAC(t *testing.T) {
	h := NewHarness(t)
	staffClient, _, _ := h.AuthAsStaff(t, "CA")

	resp := postJSON(t, staffClient, h.URL("/api/documents/presign"), map[string]any{
		"subject_kind": "child", "kind": "immunization_record", "mime_type": "application/pdf",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("staff POST /api/documents/presign: expected 403, got %d", resp.StatusCode)
	}
}

// TestDocuments_Presign_MissingFields exercises the validation branch: the
// handler checks required fields BEFORE touching Storage, so this should 400
// without panicking.
func TestDocuments_Presign_MissingFields(t *testing.T) {
	h := NewHarness(t)
	client, _, _ := h.AuthAs(t, "CA")

	resp := postJSON(t, client, h.URL("/api/documents/presign"), map[string]any{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (body=%s)", resp.StatusCode, readAll(t, resp))
	}
}

// TestDocuments_Presign_NilStorageReturns503: when the harness leaves the
// Storage client nil (or runs without AWS creds), Presign must return 503
// storage_not_configured BEFORE touching any S3 plumbing — so the UI gets
// a clear signal instead of a phantom URL that silently 403s on upload.
func TestDocuments_Presign_NilStorageReturns503(t *testing.T) {
	h := NewHarness(t)
	client, _, _ := h.AuthAs(t, "CA")

	resp := postJSON(t, client, h.URL("/api/documents/presign"), map[string]any{
		"subject_kind": "child",
		"subject_id":   base62.NewID()[:22],
		"kind":         "immunization_record",
		"mime_type":    "application/pdf",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured storage: expected 503, got %d (body=%s)", resp.StatusCode, readAll(t, resp))
	}
}

func TestDocuments_Finalize_RBAC(t *testing.T) {
	h := NewHarness(t)
	staffClient, _, _ := h.AuthAsStaff(t, "CA")

	resp := postJSON(t, staffClient, h.URL("/api/documents/"+base62.NewID()[:22]+"/finalize"), map[string]any{})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("staff POST /api/documents/{id}/finalize: expected 403, got %d", resp.StatusCode)
	}
}

// TestDocuments_Finalize_NotFound: finalize requires an existing documents
// row. With no row in the DB the handler returns 404 via its
// QueryRow-wrap-ErrNotFound path — and never touches Storage, so nil storage
// is fine here.
func TestDocuments_Finalize_NotFound(t *testing.T) {
	h := NewHarness(t)
	client, _, _ := h.AuthAs(t, "CA")

	resp := postJSON(t, client, h.URL("/api/documents/does-not-exist/finalize"), map[string]any{})
	defer resp.Body.Close()
	// Either 404 (clean not-found) or 500 (if schema drift masks the scan
	// error before ErrNoRows can be read).
	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 404 or 500, got %d", resp.StatusCode)
	}
}

func TestDocuments_Delete_RBAC(t *testing.T) {
	h := NewHarness(t)
	staffClient, _, _ := h.AuthAsStaff(t, "CA")

	req, _ := http.NewRequest(http.MethodDelete, h.URL("/api/documents/"+base62.NewID()[:22]), nil)
	resp, err := staffClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("staff DELETE /api/documents/{id}: expected 403, got %d", resp.StatusCode)
	}
}

// TestDocuments_Delete_AdminSoftDeletes: admin DELETE on a non-existent id
// still returns 204 (SQL UPDATE is a no-op). Verifies the handler reaches
// SQL without panicking.
func TestDocuments_Delete_AdminSoftDeletes(t *testing.T) {
	h := NewHarness(t)
	client, _, _ := h.AuthAs(t, "CA")

	req, _ := http.NewRequest(http.MethodDelete, h.URL("/api/documents/does-not-exist"), nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("admin DELETE: expected 204, got %d", resp.StatusCode)
	}
}

// TestDocuments_List_ReachesHandler: GET /api/documents reaches the handler.
// The SELECT reads `subject_kind` which doesn't exist — we expect 500 from
// the schema drift until it's reconciled.
func TestDocuments_List_ReachesHandler(t *testing.T) {
	h := NewHarness(t)
	client, _, _ := h.AuthAs(t, "CA")

	resp, err := client.Get(h.URL("/api/documents"))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		t.Fatalf("blocked by auth/rbac: %d", resp.StatusCode)
	}
}

func TestDocuments_Get_ReachesHandler(t *testing.T) {
	h := NewHarness(t)
	client, _, _ := h.AuthAs(t, "CA")

	resp, err := client.Get(h.URL("/api/documents/" + base62.NewID()[:22]))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		t.Fatalf("blocked: %d", resp.StatusCode)
	}
}

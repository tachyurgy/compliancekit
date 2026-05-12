package integration

// REQ-007: manual supersede endpoint.
//
// POST /api/staff/{id}/document/{doc_id}/supersede flips superseded_at
// without requiring a replacement. Used when the auto-supersede path
// can't infer (different kind, staff transferred, etc.). Admin-only,
// idempotent.

import (
	"database/sql"
	"net/http"
	"testing"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
)

// seedStaffDoc inserts a staff member + an active document tied to the
// given providerID. Returns the staff_id and document_id. The document is
// confirmed (expiration_confirmed_at set) so the chase candidate would
// otherwise pick it up.
func seedStaffDoc(t *testing.T, db *sql.DB, providerID string) (string, string) {
	t.Helper()
	staffID := base62.NewID()[:22]
	docID := base62.NewID()[:22]

	// Use legacy column names because the documents table's NOT NULL
	// columns (owner_kind, owner_id) live there per migration 000004; the
	// handler-era subject_kind / subject_id mirror them.
	if _, err := db.Exec(`INSERT INTO staff (id, provider_id, email, first_name, last_name, status, created_at, updated_at)
		VALUES (?, ?, 'jane@example.com', 'Jane', 'Doe', 'active', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		staffID, providerID); err != nil {
		t.Fatalf("seed staff: %v", err)
	}
	// documents has legacy NOT NULL columns (s3_key, uploaded_via) from
	// migration 000004 that we satisfy here. The handler-era column names
	// (subject_kind, subject_id, kind) are mirrored alongside the legacy
	// ones the schema requires.
	if _, err := db.Exec(`INSERT INTO documents
		(id, provider_id, owner_kind, owner_id, subject_kind, subject_id, kind, doc_type, title,
		 s3_key, uploaded_via,
		 expiration_confirmed_at, created_at, updated_at)
		VALUES (?, ?, 'staff', ?, 'staff', ?, 'cpr_cert', 'cpr_cert', 'CPR',
		        ?, 'provider',
		        CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		docID, providerID, staffID, staffID, "test/key/"+docID); err != nil {
		t.Fatalf("seed doc: %v", err)
	}
	return staffID, docID
}

func TestManualSupersede_FlipsRow(t *testing.T) {
	h := NewHarness(t)
	client, providerID, _ := h.AuthAs(t, "FL")
	staffID, docID := seedStaffDoc(t, h.DB, providerID)

	resp := postJSON(t, client, h.URL("/api/staff/"+staffID+"/document/"+docID+"/supersede"), map[string]any{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", resp.StatusCode, readAll(t, resp))
	}

	var supersededAt sql.NullString
	if err := h.DB.QueryRow(`SELECT superseded_at FROM documents WHERE id = ?`, docID).Scan(&supersededAt); err != nil {
		t.Fatalf("query: %v", err)
	}
	if !supersededAt.Valid || supersededAt.String == "" {
		t.Fatal("superseded_at not stamped after manual supersede")
	}
}

func TestManualSupersede_Idempotent(t *testing.T) {
	h := NewHarness(t)
	client, providerID, _ := h.AuthAs(t, "FL")
	staffID, docID := seedStaffDoc(t, h.DB, providerID)

	url := h.URL("/api/staff/" + staffID + "/document/" + docID + "/supersede")
	r1 := postJSON(t, client, url, map[string]any{})
	r1.Body.Close()
	if r1.StatusCode != http.StatusOK {
		t.Fatalf("first POST: %d", r1.StatusCode)
	}

	// Snapshot the first superseded_at timestamp.
	var first string
	_ = h.DB.QueryRow(`SELECT superseded_at FROM documents WHERE id = ?`, docID).Scan(&first)

	r2 := postJSON(t, client, url, map[string]any{})
	defer r2.Body.Close()
	if r2.StatusCode != http.StatusOK {
		t.Fatalf("second POST: %d", r2.StatusCode)
	}

	// Idempotent: the stamp must NOT be overwritten on the second call.
	var second string
	_ = h.DB.QueryRow(`SELECT superseded_at FROM documents WHERE id = ?`, docID).Scan(&second)
	if second != first {
		t.Fatalf("idempotent supersede should not bump timestamp: %q != %q", second, first)
	}
}

func TestManualSupersede_WrongStaff_NotFound(t *testing.T) {
	h := NewHarness(t)
	client, providerID, _ := h.AuthAs(t, "FL")
	_, docID := seedStaffDoc(t, h.DB, providerID)
	// Use a wrong staff id in the URL — the handler verifies subject_id
	// matches and 404s otherwise.
	resp := postJSON(t, client, h.URL("/api/staff/wrongstaffid/document/"+docID+"/supersede"), map[string]any{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for mismatched subject, got %d", resp.StatusCode)
	}
}

func TestManualSupersede_StaffRole_Forbidden(t *testing.T) {
	h := NewHarness(t)
	// Admin seeds, then a staff-role user tries to call the endpoint.
	_, providerID, _ := h.AuthAs(t, "FL")
	staffID, docID := seedStaffDoc(t, h.DB, providerID)
	staffClient, _, _ := h.AuthAsStaff(t, "FL")

	resp := postJSON(t, staffClient, h.URL("/api/staff/"+staffID+"/document/"+docID+"/supersede"), map[string]any{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin, got %d", resp.StatusCode)
	}
}

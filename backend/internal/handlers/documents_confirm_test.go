package handlers_test

import (
	"context"
	"testing"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/testhelp"
)

// TestDocuments_ConfirmationFlow_GatesChase verifies REQ-006's invariant:
// until expiration_confirmed_at is set on a document with an OCR-extracted
// expiration, the chase candidates query returns zero rows for it; after
// confirmation, it shows up. Toggling does_not_expire removes it again.
//
// We don't run the whole chase pipeline here — we just exercise the SQL
// gate directly, which is the load-bearing change in chase.go.
func TestDocuments_ConfirmationFlow_GatesChase(t *testing.T) {
	pool := testhelp.OpenDB(t)
	ctx := context.Background()

	pid := "p_test01"
	cid := "c_test01"
	did := "d_test01"
	if _, err := pool.ExecContext(ctx, `
		INSERT INTO providers (id, legal_name, name, owner_email, state, state_code, capacity, timezone, created_at, updated_at)
		VALUES (?, 'Test LLC', 'Test', 'a@b.c', 'FL', 'FL', 30, 'America/New_York', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, pid); err != nil {
		t.Fatalf("seed provider: %v", err)
	}
	if _, err := pool.ExecContext(ctx, `
		INSERT INTO children (id, provider_id, first_name, last_name, date_of_birth, enrollment_date, parent_email, parent_phone, classroom, status, created_at, updated_at)
		VALUES (?, ?, 'Ada', 'L', '2022-01-01', '2024-01-01', 'p@b.c', '+15551112222', 'Bumblebees', 'enrolled', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, cid, pid); err != nil {
		t.Fatalf("seed child: %v", err)
	}
	expires := time.Now().Add(45 * 24 * time.Hour) // within 60-day chase bucket
	if _, err := pool.ExecContext(ctx, `
		INSERT INTO documents (id, provider_id, owner_kind, owner_id, subject_kind, subject_id,
		                      doc_type, kind, original_filename, mime_type, s3_key, storage_key,
		                      uploaded_via, expiration_date, expires_at,
		                      expiration_extracted_value, extraction_confidence,
		                      created_at, updated_at)
		VALUES (?, ?, 'child', ?, 'child', ?,
		        'immunization_record', 'immunization_record', 'a.pdf', 'application/pdf', 'k', 'k',
		        'parent_portal', ?, ?,
		        ?, 0.92,
		        CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		did, pid, cid, cid, expires.Format("2006-01-02"), expires.Format("2006-01-02"),
		expires.Format("2006-01-02")); err != nil {
		t.Fatalf("seed doc: %v", err)
	}

	countCandidates := func() int {
		windowFloor := time.Now().Add(-365 * 24 * time.Hour)
		windowEnd := time.Now().Add(95 * 24 * time.Hour)
		var n int
		err := pool.QueryRowContext(ctx, `
			SELECT COUNT(*)
			  FROM documents d
			 WHERE d.deleted_at IS NULL
			   AND d.expires_at IS NOT NULL
			   AND d.expires_at >= ?
			   AND d.expires_at <= ?
			   AND (
			     d.expiration_confirmed_at IS NOT NULL
			     OR d.expiration_extracted_value IS NULL
			     OR d.expiration_extracted_value = ''
			   )
			   AND COALESCE(d.does_not_expire, 0) = 0`,
			windowFloor, windowEnd).Scan(&n)
		if err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}

	if n := countCandidates(); n != 0 {
		t.Fatalf("before confirmation: expected 0 chase candidates, got %d", n)
	}

	if _, err := pool.ExecContext(ctx,
		`UPDATE documents SET expiration_confirmed_at = CURRENT_TIMESTAMP WHERE id = ?`, did); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if n := countCandidates(); n != 1 {
		t.Fatalf("after confirmation: expected 1 chase candidate, got %d", n)
	}

	if _, err := pool.ExecContext(ctx,
		`UPDATE documents SET does_not_expire = 1 WHERE id = ?`, did); err != nil {
		t.Fatalf("toggle dne: %v", err)
	}
	if n := countCandidates(); n != 0 {
		t.Fatalf("does_not_expire: expected 0, got %d", n)
	}
}

// TestUploadAudit_TableExists verifies the migration produced upload_audit
// with the expected columns. Insert with every NOT NULL field populated;
// if the migration drifted, SQLite raises a binding error.
func TestUploadAudit_TableExists(t *testing.T) {
	pool := testhelp.OpenDB(t)
	if _, err := pool.Exec(`
		INSERT INTO upload_audit (id, magic_link_id, document_id, provider_id,
		                          ip, user_agent, file_bytes, file_mime, created_at)
		VALUES (?, NULL, NULL, NULL, '1.2.3.4', 'curl/8', 4096, 'image/jpeg', CURRENT_TIMESTAMP)`,
		"a_test01"); err != nil {
		t.Fatalf("insert audit: %v", err)
	}
	var n int
	_ = pool.QueryRow(`SELECT COUNT(*) FROM upload_audit`).Scan(&n)
	if n != 1 {
		t.Fatalf("expected 1 audit row, got %d", n)
	}
}

// TestRequiredDocs_FloridaChildrenSeeded verifies migration 000028 seeded
// the Handbook § 7.3 child document set.
func TestRequiredDocs_FloridaChildrenSeeded(t *testing.T) {
	pool := testhelp.OpenDB(t)
	var n int
	if err := pool.QueryRow(
		`SELECT COUNT(*) FROM document_requirements WHERE state_code = 'FL' AND entity_type = 'child'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n < 11 {
		t.Fatalf("expected >= 11 FL child requirements, got %d", n)
	}
	// Spot-check DH 680 row.
	var label string
	if err := pool.QueryRow(`SELECT label FROM document_requirements WHERE id = 'req_fl_child_dh680'`).Scan(&label); err != nil {
		t.Fatalf("dh680 row missing: %v", err)
	}
	if label == "" {
		t.Fatalf("dh680 label empty")
	}
}

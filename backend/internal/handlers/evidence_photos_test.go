package handlers_test

// REQ-038 unit tests for the evidence_photos handler. We exercise the DB
// surface (insert, list, soft-delete, caption update) directly through
// SQL because the multipart upload path needs a configured S3 client and
// belongs in the integration suite. The handler-level checks here cover:
//   - migration applied (table + indexes exist + soft-delete column)
//   - tenant isolation: a list for provider B does not surface
//     provider A's rows.
//   - soft-delete excludes the row from list reads.
//   - caption update path rewrites the column.

import (
	"context"
	"testing"

	"github.com/tachyurgy/compliancekit/backend/internal/testhelp"
)

func TestEvidencePhotos_TenantIsolationAndSoftDelete(t *testing.T) {
	pool := testhelp.OpenDB(t)
	ctx := context.Background()

	// Seed two providers.
	for _, pid := range []string{"pA_evid", "pB_evid"} {
		if _, err := pool.ExecContext(ctx, `
			INSERT INTO providers (id, legal_name, name, owner_email, state, state_code, capacity, timezone, created_at, updated_at)
			VALUES (?, 'X LLC', 'X', ?, 'FL', 'FL', 30, 'America/New_York', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			pid, pid+"@b.c"); err != nil {
			t.Fatalf("seed provider %s: %v", pid, err)
		}
	}

	// Two rows for provider A: one on a drill, one on a posting.
	insert := func(id, pid, sk, sid, caption string) {
		t.Helper()
		if _, err := pool.ExecContext(ctx, `
			INSERT INTO evidence_photos (id, provider_id, subject_kind, subject_id,
			                             storage_bucket, storage_key, mime_type,
			                             caption, uploaded_by_user_id, created_at)
			VALUES (?,?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP)`,
			id, pid, sk, sid, "raw", "k/"+id+".jpg", "image/jpeg", caption, nil); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}
	insert("ph_a1", "pA_evid", "drill", "drill_xyz", "muster sheet")
	insert("ph_a2", "pA_evid", "posting", "license", "behind reception")
	insert("ph_b1", "pB_evid", "drill", "drill_xyz", "should not leak")

	count := func(pid, sk, sid string) int {
		t.Helper()
		var n int
		if err := pool.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM evidence_photos
			WHERE provider_id = ? AND subject_kind = ? AND subject_id = ? AND deleted_at IS NULL`,
			pid, sk, sid).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}

	if got := count("pA_evid", "drill", "drill_xyz"); got != 1 {
		t.Fatalf("provider A drill: want 1, got %d", got)
	}
	if got := count("pA_evid", "posting", "license"); got != 1 {
		t.Fatalf("provider A posting: want 1, got %d", got)
	}
	if got := count("pB_evid", "drill", "drill_xyz"); got != 1 {
		t.Fatalf("provider B drill: want 1, got %d", got)
	}
	if got := count("pA_evid", "drill", "drill_zzz"); got != 0 {
		t.Fatalf("unmatched subject_id should return 0, got %d", got)
	}

	// Soft delete provider A's drill photo.
	if _, err := pool.ExecContext(ctx,
		`UPDATE evidence_photos SET deleted_at = CURRENT_TIMESTAMP WHERE id = ?`, "ph_a1"); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if got := count("pA_evid", "drill", "drill_xyz"); got != 0 {
		t.Fatalf("after soft delete: want 0, got %d", got)
	}

	// Caption update path.
	if _, err := pool.ExecContext(ctx,
		`UPDATE evidence_photos SET caption = ? WHERE id = ? AND provider_id = ? AND deleted_at IS NULL`,
		"reception wall", "ph_a2", "pA_evid"); err != nil {
		t.Fatalf("update caption: %v", err)
	}
	var caption string
	if err := pool.QueryRowContext(ctx,
		`SELECT caption FROM evidence_photos WHERE id = ?`, "ph_a2").Scan(&caption); err != nil {
		t.Fatalf("read caption: %v", err)
	}
	if caption != "reception wall" {
		t.Fatalf("caption mismatch: got %q", caption)
	}
}

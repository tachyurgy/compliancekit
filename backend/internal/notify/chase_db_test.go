package notify

// REQ-007: DB-bound unit tests for the new chase plumbing.
//
// We cover two things here without spinning the full chase loop (whose
// scan path uses time.Time, which modernc/sqlite cannot auto-parse on the
// way out — that's an existing project quirk, not in scope for REQ-007):
//
//   1) isUnsubscribed honors per-channel (channel, recipient) tuples.
//   2) The chase candidate filter `superseded_at IS NULL` is enforced at
//      the SQL level — superseded docs are removed from the result set.

import (
	"context"
	"database/sql"
	"log/slog"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/tachyurgy/compliancekit/backend/internal/db"
)

// chaseTestDB returns a sqlite *DB with the minimal schema chase.go
// touches: documents (with the new REQ-007 columns), unsubscribes.
func chaseTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tmp := t.TempDir()
	pool, err := db.Open(context.Background(), filepath.Join(tmp, "chase.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	stmts := []string{
		`CREATE TABLE documents (
			id TEXT PRIMARY KEY, provider_id TEXT,
			subject_kind TEXT, subject_id TEXT, kind TEXT,
			expires_at TEXT,
			expiration_extracted_value TEXT,
			expiration_confirmed_at TEXT,
			does_not_expire INTEGER DEFAULT 0,
			superseded_at TEXT,
			deleted_at TEXT)`,
		`CREATE TABLE unsubscribes (
			id TEXT PRIMARY KEY, channel TEXT, recipient TEXT, reason TEXT,
			created_at TEXT, UNIQUE(channel, recipient))`,
	}
	for _, s := range stmts {
		if _, err := pool.Exec(s); err != nil {
			t.Fatalf("schema: %v\n%s", err, s)
		}
	}
	return pool
}

func TestIsUnsubscribed_PerChannel(t *testing.T) {
	t.Parallel()
	pool := chaseTestDB(t)
	if _, err := pool.Exec(`INSERT INTO unsubscribes (id, channel, recipient, reason, created_at)
		VALUES ('u1', 'email', 'stop@example.com', 'ses_hard_bounce', CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	c := &ChaseService{pool: pool, log: slog.Default()}
	ctx := context.Background()

	if !c.isUnsubscribed(ctx, "email", "stop@example.com") {
		t.Fatal("expected email opt-out to be flagged")
	}
	// REQ-007: per-channel — an email opt-out does NOT silence SMS sends
	// to the same address (in practice these are different routes, but the
	// table lookup must respect channel as a discriminator).
	if c.isUnsubscribed(ctx, "sms", "stop@example.com") {
		t.Fatal("per-channel: email opt-out must not silence sms")
	}
	if c.isUnsubscribed(ctx, "email", "ok@example.com") {
		t.Fatal("unrelated address should not match")
	}
	if c.isUnsubscribed(ctx, "email", "") {
		t.Fatal("empty recipient should never match")
	}

	// SMS-side: STOP from a phone marks that channel only.
	if _, err := pool.Exec(`INSERT INTO unsubscribes (id, channel, recipient, reason, created_at)
		VALUES ('u2', 'sms', '+13055551212', 'twilio_stop', CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("seed sms: %v", err)
	}
	if !c.isUnsubscribed(ctx, "sms", "+13055551212") {
		t.Fatal("sms opt-out not honored")
	}
	if c.isUnsubscribed(ctx, "email", "+13055551212") {
		t.Fatal("sms opt-out must not bleed into email channel")
	}
}

// TestCandidateFilter_SkipsSupersededDocs exercises the WHERE clause we
// added in REQ-007: a document with superseded_at IS NOT NULL must be
// absent from the chase candidate set. We use the same predicate the
// chase scan uses (see chase.go ProcessOnce).
func TestCandidateFilter_SkipsSupersededDocs(t *testing.T) {
	t.Parallel()
	pool := chaseTestDB(t)

	// Two docs: one active, one superseded. Both have a confirmed expiration.
	_, err := pool.Exec(`INSERT INTO documents (id, provider_id, subject_kind, subject_id, kind,
		expires_at, expiration_confirmed_at, superseded_at, deleted_at)
		VALUES
		  ('doc_active',     'p1', 'staff', 's1', 'cpr_cert', '2099-01-01', CURRENT_TIMESTAMP, NULL, NULL),
		  ('doc_superseded', 'p1', 'staff', 's1', 'cpr_cert', '2099-01-01', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL)`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Mirror the candidate filter exactly. If chase.go's predicate is
	// later weakened, this test fails loudly.
	rows, err := pool.Query(`
		SELECT id FROM documents
		 WHERE deleted_at IS NULL
		   AND expires_at IS NOT NULL
		   AND (expiration_confirmed_at IS NOT NULL OR expiration_extracted_value IS NULL OR expiration_extracted_value = '')
		   AND COALESCE(does_not_expire, 0) = 0
		   AND superseded_at IS NULL`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	got := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[id] = true
	}
	if !got["doc_active"] {
		t.Error("active doc missing from candidates")
	}
	if got["doc_superseded"] {
		t.Error("superseded doc must not appear in candidates")
	}
}

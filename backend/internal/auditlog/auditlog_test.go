package auditlog_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tachyurgy/compliancekit/backend/internal/auditlog"
	"github.com/tachyurgy/compliancekit/backend/internal/testhelp"
)

// seed inserts a default provider "p1" so the FK to audit_log.provider_id is
// satisfied when we assert on it.
func seed(t *testing.T) *sql.DB {
	t.Helper()
	pool := testhelp.OpenDB(t)
	if _, err := pool.Exec(`INSERT INTO providers (id, legal_name, state) VALUES ('p1','T','CA')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return pool
}

func TestEmit_NilPool_ReturnsNil(t *testing.T) {
	t.Parallel()
	if err := auditlog.Emit(context.Background(), nil, auditlog.Entry{Action: "test"}); err != nil {
		t.Fatalf("Emit(nil) = %v, want nil", err)
	}
}

func TestEmit_MissingAction_Skipped(t *testing.T) {
	t.Parallel()
	pool := seed(t)
	if err := auditlog.Emit(context.Background(), pool, auditlog.Entry{ProviderID: "p1"}); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	var n int
	_ = pool.QueryRow(`SELECT COUNT(*) FROM audit_log`).Scan(&n)
	if n != 0 {
		t.Fatalf("audit_log count = %d, want 0 (empty action is a no-op)", n)
	}
}

func TestEmit_WritesRow_AllFields(t *testing.T) {
	t.Parallel()
	pool := seed(t)
	err := auditlog.Emit(context.Background(), pool, auditlog.Entry{
		ProviderID: "p1",
		ActorKind:  auditlog.ActorKindProviderAdmin,
		ActorID:    "u1",
		Action:     auditlog.ActionChildCreate,
		TargetKind: auditlog.TargetKindChild,
		TargetID:   "c1",
		Metadata:   map[string]any{"source": "ui", "count": 3},
		IP:         "1.2.3.4",
		UserAgent:  "curl/8.0",
	})
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	var pid, actorKind, actorID, action, targetKind, targetID, meta, ip, ua string
	err = pool.QueryRow(
		`SELECT provider_id, actor_kind, actor_id, action, target_kind, target_id, metadata, ip, user_agent FROM audit_log`).
		Scan(&pid, &actorKind, &actorID, &action, &targetKind, &targetID, &meta, &ip, &ua)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if pid != "p1" || actorKind != auditlog.ActorKindProviderAdmin || actorID != "u1" ||
		action != auditlog.ActionChildCreate || targetKind != auditlog.TargetKindChild || targetID != "c1" ||
		ip != "1.2.3.4" || ua != "curl/8.0" {
		t.Fatalf("field mismatch: %+v", []string{pid, actorKind, actorID, action, targetKind, targetID, ip, ua})
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(meta), &parsed); err != nil {
		t.Fatalf("metadata not valid JSON: %v", err)
	}
	if parsed["source"] != "ui" {
		t.Fatalf("metadata[source] = %v", parsed["source"])
	}
}

func TestEmit_EmptyMetadata_StoresEmptyObject(t *testing.T) {
	t.Parallel()
	pool := seed(t)
	_ = auditlog.Emit(context.Background(), pool, auditlog.Entry{ProviderID: "p1", Action: "x.test"})
	var meta string
	_ = pool.QueryRow(`SELECT metadata FROM audit_log`).Scan(&meta)
	if meta != "{}" {
		t.Fatalf("metadata = %q, want {}", meta)
	}
}

func TestEmit_DefaultActorKind_IsSystem(t *testing.T) {
	t.Parallel()
	pool := seed(t)
	_ = auditlog.Emit(context.Background(), pool, auditlog.Entry{ProviderID: "p1", Action: "x"})
	var kind string
	_ = pool.QueryRow(`SELECT actor_kind FROM audit_log`).Scan(&kind)
	if kind != auditlog.ActorKindSystem {
		t.Fatalf("actor_kind = %q, want system", kind)
	}
}

// ---- ClientIP ----

func TestClientIP_XForwardedFor_Priority(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest("GET", "/x", nil)
	req.RemoteAddr = "10.0.0.1:4000"
	req.Header.Set("X-Forwarded-For", "1.1.1.1, 2.2.2.2, 3.3.3.3")
	if got := auditlog.ClientIP(req); got != "1.1.1.1" {
		t.Fatalf("XFF priority: got %q, want 1.1.1.1", got)
	}
}

func TestClientIP_XForwardedFor_SingleHop(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("X-Forwarded-For", "9.9.9.9")
	if got := auditlog.ClientIP(req); got != "9.9.9.9" {
		t.Fatalf("got %q", got)
	}
}

func TestClientIP_FallsBackToRemoteAddr(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest("GET", "/x", nil)
	req.RemoteAddr = "10.0.0.1:4321"
	if got := auditlog.ClientIP(req); got != "10.0.0.1" {
		t.Fatalf("got %q, want 10.0.0.1", got)
	}
}

func TestClientIP_NilRequest(t *testing.T) {
	t.Parallel()
	if got := auditlog.ClientIP(nil); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestClientIP_EmptyRemoteAddr(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest("GET", "/x", nil)
	req.RemoteAddr = ""
	if got := auditlog.ClientIP(req); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

// ---- per-event helpers ----

func makeRequest() *http.Request {
	r := httptest.NewRequest("GET", "/x", nil)
	r.RemoteAddr = "7.7.7.7:1234"
	r.Header.Set("User-Agent", "test-agent/1.0")
	return r
}

// helperCases table-drives every EmitXxx helper against a fresh DB, asserting
// that the row lands with the expected action/target_kind and that ClientIP /
// UserAgent plumbing works.
func TestEmitHelpers(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		action     string
		targetKind string
		run        func(context.Context, *sql.DB, *http.Request)
	}{
		{"Login", auditlog.ActionLogin, auditlog.TargetKindUser,
			func(ctx context.Context, p *sql.DB, r *http.Request) { auditlog.EmitLogin(ctx, p, "p1", "u1", r) }},
		{"Signup", auditlog.ActionSignup, auditlog.TargetKindProvider,
			func(ctx context.Context, p *sql.DB, r *http.Request) {
				auditlog.EmitSignup(ctx, p, "p1", map[string]any{"email": "x@y"}, r)
			}},
		{"MeUpdate", auditlog.ActionMeUpdate, auditlog.TargetKindProvider,
			func(ctx context.Context, p *sql.DB, r *http.Request) { auditlog.EmitMeUpdate(ctx, p, "p1", "u1", r) }},
		{"ChildCreate", auditlog.ActionChildCreate, auditlog.TargetKindChild,
			func(ctx context.Context, p *sql.DB, r *http.Request) {
				auditlog.EmitChildCreate(ctx, p, "p1", "u1", "c1", r)
			}},
		{"ChildUpdate", auditlog.ActionChildUpdate, auditlog.TargetKindChild,
			func(ctx context.Context, p *sql.DB, r *http.Request) {
				auditlog.EmitChildUpdate(ctx, p, "p1", "u1", "c1", r)
			}},
		{"ChildDelete", auditlog.ActionChildDelete, auditlog.TargetKindChild,
			func(ctx context.Context, p *sql.DB, r *http.Request) {
				auditlog.EmitChildDelete(ctx, p, "p1", "u1", "c1", r)
			}},
		{"StaffCreate", auditlog.ActionStaffCreate, auditlog.TargetKindStaff,
			func(ctx context.Context, p *sql.DB, r *http.Request) {
				auditlog.EmitStaffCreate(ctx, p, "p1", "u1", "s1", r)
			}},
		{"StaffUpdate", auditlog.ActionStaffUpdate, auditlog.TargetKindStaff,
			func(ctx context.Context, p *sql.DB, r *http.Request) {
				auditlog.EmitStaffUpdate(ctx, p, "p1", "u1", "s1", r)
			}},
		{"StaffDelete", auditlog.ActionStaffDelete, auditlog.TargetKindStaff,
			func(ctx context.Context, p *sql.DB, r *http.Request) {
				auditlog.EmitStaffDelete(ctx, p, "p1", "u1", "s1", r)
			}},
		{"DocumentUpload", auditlog.ActionDocumentUpload, auditlog.TargetKindDocument,
			func(ctx context.Context, p *sql.DB, r *http.Request) {
				auditlog.EmitDocumentUpload(ctx, p, "p1", "u1", "d1", map[string]any{"conf": 0.9}, r)
			}},
		{"DocumentDelete", auditlog.ActionDocumentDelete, auditlog.TargetKindDocument,
			func(ctx context.Context, p *sql.DB, r *http.Request) {
				auditlog.EmitDocumentDelete(ctx, p, "p1", "u1", "d1", r)
			}},
		{"DrillCreate", auditlog.ActionDrillCreate, auditlog.TargetKindDrill,
			func(ctx context.Context, p *sql.DB, r *http.Request) {
				auditlog.EmitDrillCreate(ctx, p, "p1", "u1", "dr1", map[string]any{"kind": "fire"}, r)
			}},
		{"DrillDelete", auditlog.ActionDrillDelete, auditlog.TargetKindDrill,
			func(ctx context.Context, p *sql.DB, r *http.Request) {
				auditlog.EmitDrillDelete(ctx, p, "p1", "u1", "dr1", r)
			}},
		{"PostingUpdate", auditlog.ActionPostingUpdate, auditlog.TargetKindPosting,
			func(ctx context.Context, p *sql.DB, r *http.Request) {
				auditlog.EmitPostingUpdate(ctx, p, "p1", "u1", "license", nil, r)
			}},
		{"RatioCheck", auditlog.ActionRatioCheck, auditlog.TargetKindRatio,
			func(ctx context.Context, p *sql.DB, r *http.Request) {
				auditlog.EmitRatioCheck(ctx, p, "p1", "u1", map[string]any{"ok": true}, r)
			}},
		{"InspectionStart", auditlog.ActionInspectionStart, auditlog.TargetKindInspection,
			func(ctx context.Context, p *sql.DB, r *http.Request) {
				auditlog.EmitInspectionStart(ctx, p, "p1", "u1", "run1", r)
			}},
		{"InspectionFinalize", auditlog.ActionInspectionFinalize, auditlog.TargetKindInspection,
			func(ctx context.Context, p *sql.DB, r *http.Request) {
				auditlog.EmitInspectionFinalize(ctx, p, "p1", "u1", "run1", map[string]any{"score": 92}, r)
			}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pool := seed(t)
			tc.run(context.Background(), pool, makeRequest())

			var action, targetKind, ip, ua string
			err := pool.QueryRow(
				`SELECT action, COALESCE(target_kind,''), COALESCE(ip,''), COALESCE(user_agent,'') FROM audit_log ORDER BY created_at DESC LIMIT 1`).
				Scan(&action, &targetKind, &ip, &ua)
			if err != nil {
				t.Fatalf("select: %v", err)
			}
			if action != tc.action {
				t.Fatalf("action = %q, want %q", action, tc.action)
			}
			if targetKind != tc.targetKind {
				t.Fatalf("target_kind = %q, want %q", targetKind, tc.targetKind)
			}
			if ip != "7.7.7.7" {
				t.Fatalf("ip = %q, want 7.7.7.7", ip)
			}
			if ua != "test-agent/1.0" {
				t.Fatalf("ua = %q", ua)
			}
		})
	}
}

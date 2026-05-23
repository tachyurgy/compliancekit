package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
	"github.com/tachyurgy/compliancekit/backend/internal/testhelp"
)

func readerString(s string) io.Reader { return strings.NewReader(s) }

// withCtx returns a context with provider + user IDs set, mirroring what
// mw.RequireProviderSession would do for an authenticated request.
func withCtx(pid, uid string) context.Context {
	ctx := context.WithValue(context.Background(), mw.CtxKeyProviderID, pid)
	if uid != "" {
		ctx = context.WithValue(ctx, mw.CtxKeyUserID, uid)
	}
	return ctx
}

// TestChase_State_HappyPath seeds one staff cert that is missing and one
// child doc that is expired, then asserts /api/chase/state returns both
// rows with the expected metadata.
func TestChase_State_HappyPath(t *testing.T) {
	pool := testhelp.OpenDB(t)
	ctx := context.Background()

	pid := "p_chase_a"
	if _, err := pool.ExecContext(ctx, `
		INSERT INTO providers (id, legal_name, name, owner_email, state, state_code, capacity, timezone, created_at, updated_at)
		VALUES (?, 'Acme', 'Acme', 'a@b.c', 'FL', 'FL', 20, 'America/New_York', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, pid); err != nil {
		t.Fatalf("seed provider: %v", err)
	}
	// Staff with missing CPR.
	if _, err := pool.ExecContext(ctx, `
		INSERT INTO staff (id, provider_id, first_name, last_name, email, phone, role, status, preferred_contact_channel, created_at, updated_at)
		VALUES ('st_a', ?, 'Maria', 'Vasquez', 'maria@x.com', '+15551110000', 'lead_teacher', 'active', 'email', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, pid); err != nil {
		t.Fatalf("seed staff: %v", err)
	}
	if _, err := pool.ExecContext(ctx, `
		INSERT INTO staff_certifications_required (id, staff_id, cert_type, status, required_by, created_at, updated_at)
		VALUES ('scr_a', 'st_a', 'cpr_cert', 'missing', '2026-06-15', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("seed scr: %v", err)
	}
	// Child with expired DH-680.
	if _, err := pool.ExecContext(ctx, `
		INSERT INTO children (id, provider_id, first_name, last_name, date_of_birth, enrollment_date,
		                      parent_email, parent_phone, parent_name, preferred_contact_channel,
		                      status, created_at, updated_at)
		VALUES ('ch_a', ?, 'Liam', 'Davis', '2022-03-01', '2024-09-01',
		        'karina@x.com', '+15551112222', 'Karina Davis', 'sms',
		        'enrolled', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, pid); err != nil {
		t.Fatalf("seed child: %v", err)
	}
	if _, err := pool.ExecContext(ctx, `
		INSERT INTO child_documents_required (id, child_id, doc_type, status, required_by, created_at, updated_at)
		VALUES ('cdr_a', 'ch_a', 'immunization_record', 'expired', '2026-04-01', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("seed cdr: %v", err)
	}

	h := &ChaseHandler{Pool: pool, Log: slog.Default()}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/chase/state", nil).
		WithContext(withCtx(pid, "u1"))
	h.State(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("state: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var got chaseStateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Outstanding) != 2 {
		t.Fatalf("expected 2 outstanding rows, got %d (%+v)", len(got.Outstanding), got.Outstanding)
	}
	// First row by ORDER BY: cdr (required_by 2026-04-01) before scr
	// (required_by 2026-06-15). Both come back; check both names present.
	names := map[string]bool{}
	for _, row := range got.Outstanding {
		names[row.RecipientKind+":"+row.ItemLabel] = true
		if row.NagCount != 0 {
			t.Errorf("no nags seeded; nagCount = %d for %+v", row.NagCount, row)
		}
	}
	if !names["staff:CPR certification"] {
		t.Errorf("missing staff CPR row in %+v", got.Outstanding)
	}
	if !names["parent:DH-680 immunization record"] {
		t.Errorf("missing parent DH-680 row in %+v", got.Outstanding)
	}
	if len(got.Stuck) != 0 {
		t.Errorf("expected empty stuck, got %d", len(got.Stuck))
	}
}

// TestChase_State_StuckThreshold seeds 4 nag rows for one staff cert in
// the last 60 days and asserts the row appears in `stuck` as well.
func TestChase_State_StuckThreshold(t *testing.T) {
	pool := testhelp.OpenDB(t)
	ctx := context.Background()

	pid := "p_chase_b"
	if _, err := pool.ExecContext(ctx, `
		INSERT INTO providers (id, legal_name, name, owner_email, state, state_code, capacity, timezone, created_at, updated_at)
		VALUES (?, 'B', 'B', 'b@b.c', 'FL', 'FL', 20, 'America/New_York', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, pid); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := pool.ExecContext(ctx, `
		INSERT INTO staff (id, provider_id, first_name, last_name, email, role, status, preferred_contact_channel, created_at, updated_at)
		VALUES ('st_b', ?, 'Sam', 'Lee', 's@x.com', 'aide', 'active', 'email', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, pid); err != nil {
		t.Fatalf("staff: %v", err)
	}
	if _, err := pool.ExecContext(ctx, `
		INSERT INTO staff_certifications_required (id, staff_id, cert_type, status, created_at, updated_at)
		VALUES ('scr_b', 'st_b', 'tb_test', 'missing', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("scr: %v", err)
	}
	for i, id := range []string{"n1", "n2", "n3", "n4"} {
		if _, err := pool.ExecContext(ctx, `
			INSERT INTO staff_nag_sends (id, provider_id, staff_id, reason, doc_kind,
			                             channel, email_status, sms_status, triggered_by, created_at)
			VALUES (?, ?, 'st_b', 'manual', 'tb_test', 'email', 'sent', '', 'u1', CURRENT_TIMESTAMP)`,
			id, pid); err != nil {
			t.Fatalf("nag %d: %v", i, err)
		}
	}

	h := &ChaseHandler{Pool: pool, Log: slog.Default()}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/chase/state", nil).
		WithContext(withCtx(pid, "u1"))
	h.State(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("state: expected 200, got %d", rr.Code)
	}
	var got chaseStateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Outstanding) != 1 || got.Outstanding[0].NagCount != 4 {
		t.Fatalf("expected one outstanding row with nagCount=4, got %+v", got.Outstanding)
	}
	if len(got.Stuck) != 1 {
		t.Fatalf("expected stuck row, got %d (%+v)", len(got.Stuck), got.Stuck)
	}
}

// TestChase_State_Unauthorized asserts missing provider ID -> 401.
func TestChase_State_Unauthorized(t *testing.T) {
	h := &ChaseHandler{Pool: nil, Log: slog.Default()}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/chase/state", nil)
	h.State(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

// TestChase_Activity_HappyPath seeds two nag rows and one outside the
// default 30-day window; the activity endpoint should return the two
// in-window rows in chronological-DESC order.
func TestChase_Activity_HappyPath(t *testing.T) {
	pool := testhelp.OpenDB(t)
	ctx := context.Background()

	pid := "p_chase_c"
	if _, err := pool.ExecContext(ctx, `
		INSERT INTO providers (id, legal_name, name, owner_email, state, state_code, capacity, timezone, created_at, updated_at)
		VALUES (?, 'C', 'C', 'c@b.c', 'FL', 'FL', 20, 'America/New_York', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, pid); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := pool.ExecContext(ctx, `
		INSERT INTO staff (id, provider_id, first_name, last_name, email, role, status, preferred_contact_channel, created_at, updated_at)
		VALUES ('st_c', ?, 'Jay', 'Patel', 'j@x.com', 'aide', 'active', 'email', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, pid); err != nil {
		t.Fatalf("staff: %v", err)
	}
	// Two recent rows, one ancient row.
	if _, err := pool.ExecContext(ctx, `
		INSERT INTO staff_nag_sends (id, provider_id, staff_id, reason, doc_kind, channel, email_status, sms_status, triggered_by, created_at)
		VALUES
		  ('n1', ?, 'st_c', 'manual', 'cpr_cert', 'email', 'sent', '', 'u1', datetime('now', '-1 day')),
		  ('n2', ?, 'st_c', 'manual', 'first_aid_cert', 'sms', '', 'sent', 'u1', datetime('now', '-3 days')),
		  ('n_old', ?, 'st_c', 'manual', 'cpr_cert', 'email', 'sent', '', 'u1', datetime('now', '-365 days'))`,
		pid, pid, pid); err != nil {
		t.Fatalf("nag rows: %v", err)
	}

	h := &ChaseHandler{Pool: pool, Log: slog.Default()}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/chase/activity", nil).
		WithContext(withCtx(pid, "u1"))
	h.Activity(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("activity: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var got struct {
		Items []chaseActivityItem `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Items) != 2 {
		t.Fatalf("expected 2 in-window rows, got %d (%+v)", len(got.Items), got.Items)
	}
	if got.Items[0].ID != "n1" {
		t.Fatalf("expected newest row first (n1), got %s", got.Items[0].ID)
	}
	if got.Items[0].RecipientName != "Jay Patel" {
		t.Errorf("expected resolved staff name, got %q", got.Items[0].RecipientName)
	}
	if got.Items[0].ItemLabel != "CPR certification" {
		t.Errorf("expected humanized label, got %q", got.Items[0].ItemLabel)
	}
}

// TestChase_Trigger_BadInput asserts missing recipientId returns 400.
func TestChase_Trigger_BadInput(t *testing.T) {
	h := &ChaseHandler{Pool: nil, Nag: &NagHandler{}, Log: slog.Default()}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chase/trigger",
		readerString(`{"recipientKind":"staff"}`)).
		WithContext(withCtx("p1", "u1"))
	h.Trigger(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

// TestHumanItemLabel checks the small lookup table behaves on known and
// unknown slugs without panicking on empty input.
func TestHumanItemLabel(t *testing.T) {
	cases := map[string]string{
		"":                    "Missing item",
		"cpr_cert":            "CPR certification",
		"immunization_record": "DH-680 immunization record",
		"foo_bar":             "Foo bar",
	}
	for in, want := range cases {
		if got := humanItemLabel(in); got != want {
			t.Errorf("humanItemLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tachyurgy/compliancekit/backend/internal/testhelp"
)

// TestMessages_Send_BadInput covers the input validation branches: bad
// channel, empty body, empty recipients.
func TestMessages_Send_BadInput(t *testing.T) {
	pool := testhelp.OpenDB(t)
	pid := "p_msg_a"
	if _, err := pool.ExecContext(context.Background(), `
		INSERT INTO providers (id, legal_name, name, owner_email, state, state_code, capacity, timezone, created_at, updated_at)
		VALUES (?, 'A', 'A', 'a@b.c', 'FL', 'FL', 20, 'America/New_York', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, pid); err != nil {
		t.Fatalf("seed: %v", err)
	}
	h := &MessagesHandler{Pool: pool, Log: slog.Default()}

	cases := []struct {
		name string
		body string
	}{
		{"bad channel", `{"recipients":[{"kind":"staff","id":"x"}],"channel":"voicemail","body":"hi"}`},
		{"empty body", `{"recipients":[{"kind":"staff","id":"x"}],"channel":"email","body":"   "}`},
		{"no recipients", `{"recipients":[],"channel":"email","body":"hi"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/messages/send", strings.NewReader(c.body)).
				WithContext(withCtx(pid, "u1"))
			h.Send(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d (%s)", rr.Code, rr.Body.String())
			}
		})
	}
}

// TestMessages_Send_HappyPath asserts that with no emailer/sms configured
// (the test scaffolding can't reach SES or Twilio) the handler still
// records per-channel rows with status='failed' and a clear error.
func TestMessages_Send_RecordsFailedWhenDisabled(t *testing.T) {
	pool := testhelp.OpenDB(t)
	ctx := context.Background()
	pid := "p_msg_b"
	if _, err := pool.ExecContext(ctx, `
		INSERT INTO providers (id, legal_name, name, owner_email, state, state_code, capacity, timezone, created_at, updated_at)
		VALUES (?, 'B', 'B', 'b@b.c', 'FL', 'FL', 20, 'America/New_York', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, pid); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := pool.ExecContext(ctx, `
		INSERT INTO staff (id, provider_id, first_name, last_name, email, phone, role, status, preferred_contact_channel, created_at, updated_at)
		VALUES ('st_b', ?, 'Jane', 'Doe', 'jane@x.com', '+15551110000', 'lead_teacher', 'active', 'email', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, pid); err != nil {
		t.Fatalf("staff: %v", err)
	}

	h := &MessagesHandler{Pool: pool, Log: slog.Default()} // no Emailer / SMS
	rr := httptest.NewRecorder()
	body := `{"recipients":[{"kind":"staff","id":"st_b"}],"channel":"both","subject":"hi","body":"hello"}`
	// Use an empty user id so the FK on sender_user_id stays NULL (the
	// users table is unseeded in this test).
	req := httptest.NewRequest(http.MethodPost, "/api/messages/send", strings.NewReader(body)).
		WithContext(withCtx(pid, ""))
	h.Send(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	var got struct {
		Results []sendResult `json:"results"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Results) != 2 {
		t.Fatalf("expected 2 results (email + sms), got %d (%+v)", len(got.Results), got.Results)
	}
	for _, r := range got.Results {
		if r.Status != "failed" {
			t.Errorf("expected failed (emailer/sms disabled), got %s for %s", r.Status, r.Channel)
		}
	}

	// Persistence check: 2 rows now exist for that staff member.
	var count int
	if err := pool.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM outgoing_messages WHERE provider_id = ? AND recipient_id = 'st_b'`,
		pid).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 rows, got %d", count)
	}
}

// TestMessages_History returns rows for the right recipient and sorts
// descending.
func TestMessages_History(t *testing.T) {
	pool := testhelp.OpenDB(t)
	ctx := context.Background()
	pid := "p_msg_c"
	if _, err := pool.ExecContext(ctx, `
		INSERT INTO providers (id, legal_name, name, owner_email, state, state_code, capacity, timezone, created_at, updated_at)
		VALUES (?, 'C', 'C', 'c@b.c', 'FL', 'FL', 20, 'America/New_York', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, pid); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := pool.ExecContext(ctx, `
		INSERT INTO outgoing_messages (id, provider_id, sender_user_id, recipient_kind, recipient_id, channel, subject, body, status, created_at)
		VALUES
		  ('m1', ?, NULL, 'staff', 'st_c', 'email', 'Subject 1', 'Body 1', 'sent', datetime('now', '-2 days')),
		  ('m2', ?, NULL, 'staff', 'st_c', 'sms',   NULL,         'Body 2', 'sent', datetime('now', '-1 day'))`,
		pid, pid); err != nil {
		t.Fatalf("seed messages: %v", err)
	}
	// Add a noise row for a different recipient that must not appear.
	if _, err := pool.ExecContext(ctx, `
		INSERT INTO outgoing_messages (id, provider_id, sender_user_id, recipient_kind, recipient_id, channel, subject, body, status, created_at)
		VALUES ('noise', ?, NULL, 'parent', 'ch_x', 'email', 'X', 'X', 'sent', CURRENT_TIMESTAMP)`, pid); err != nil {
		t.Fatalf("seed noise: %v", err)
	}

	h := &MessagesHandler{Pool: pool, Log: slog.Default()}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/messages?recipientKind=staff&recipientId=st_c", nil).
		WithContext(withCtx(pid, "u1"))
	h.History(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var got struct {
		Items []messageItem `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Items) != 2 {
		t.Fatalf("expected 2 items, got %d (%+v)", len(got.Items), got.Items)
	}
	if got.Items[0].ID != "m2" {
		t.Errorf("expected m2 first (newer), got %s", got.Items[0].ID)
	}
}

// TestMessages_Recipients returns staff and parents separately.
func TestMessages_Recipients(t *testing.T) {
	pool := testhelp.OpenDB(t)
	ctx := context.Background()
	pid := "p_msg_d"
	if _, err := pool.ExecContext(ctx, `
		INSERT INTO providers (id, legal_name, name, owner_email, state, state_code, capacity, timezone, created_at, updated_at)
		VALUES (?, 'D', 'D', 'd@b.c', 'FL', 'FL', 20, 'America/New_York', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, pid); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := pool.ExecContext(ctx, `
		INSERT INTO staff (id, provider_id, first_name, last_name, email, role, status, preferred_contact_channel, created_at, updated_at)
		VALUES
		  ('st_d', ?, 'Anna', 'Lim', 'anna@x.com', 'lead_teacher', 'active', 'email', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		  ('st_old', ?, 'X', 'Y', 'old@x.com', 'cook', 'terminated', 'email', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, pid, pid); err != nil {
		t.Fatalf("staff: %v", err)
	}
	if _, err := pool.ExecContext(ctx, `
		INSERT INTO children (id, provider_id, first_name, last_name, date_of_birth, enrollment_date,
		                      parent_email, parent_phone, parent_name, status, created_at, updated_at)
		VALUES ('ch_d', ?, 'Liam', 'Davis', '2022-01-01', '2024-01-01', 'k@x.com', '+15551110000', 'Karina Davis', 'enrolled', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, pid); err != nil {
		t.Fatalf("child: %v", err)
	}

	h := &MessagesHandler{Pool: pool, Log: slog.Default()}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/messages/recipients", nil).
		WithContext(withCtx(pid, "u1"))
	h.Recipients(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var got struct {
		Staff   []recipientOption `json:"staff"`
		Parents []recipientOption `json:"parents"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Only the active staff member shows up.
	if len(got.Staff) != 1 || got.Staff[0].ID != "st_d" {
		t.Errorf("expected one active staff, got %+v", got.Staff)
	}
	if len(got.Parents) != 1 || got.Parents[0].Name != "Karina Davis" {
		t.Errorf("expected one parent named Karina Davis, got %+v", got.Parents)
	}
}

// TestRenderComposerHTML autolinks URLs and preserves line breaks.
func TestRenderComposerHTML(t *testing.T) {
	html := renderComposerHTML("Sunny Days", "See https://example.com\nThanks")
	if !strings.Contains(html, `<a href="https://example.com"`) {
		t.Errorf("expected autolinked URL in: %s", html)
	}
	if !strings.Contains(html, "<br>") {
		t.Errorf("expected <br> from newline in: %s", html)
	}
	if !strings.Contains(html, "Sunny Days") {
		t.Errorf("expected provider name in footer: %s", html)
	}
}

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
)

// auditLogPage mirrors AuditLogHandler.List's response shape.
type auditLogPage struct {
	Items []struct {
		ID         string                 `json:"id"`
		ProviderID string                 `json:"provider_id"`
		ActorKind  string                 `json:"actor_kind"`
		ActorID    string                 `json:"actor_id,omitempty"`
		Action     string                 `json:"action"`
		TargetKind string                 `json:"target_kind,omitempty"`
		TargetID   string                 `json:"target_id,omitempty"`
		Metadata   map[string]interface{} `json:"metadata"`
		IP         string                 `json:"ip,omitempty"`
		UserAgent  string                 `json:"user_agent,omitempty"`
		CreatedAt  time.Time              `json:"created_at"`
	} `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// seedAuditRow inserts one audit_log row directly and returns its id.
func seedAuditRow(t *testing.T, h *Harness, providerID, action, targetKind, targetID string, meta map[string]any, createdAt time.Time) string {
	t.Helper()
	id := base62.NewID()[:22]
	metaJSON := "{}"
	if len(meta) > 0 {
		b, err := json.Marshal(meta)
		if err != nil {
			t.Fatalf("marshal meta: %v", err)
		}
		metaJSON = string(b)
	}
	if _, err := h.DB.Exec(`
		INSERT INTO audit_log (id, provider_id, actor_kind, actor_id, action, target_kind, target_id, metadata, ip, user_agent, created_at)
		VALUES (?, ?, 'provider_admin', NULL, ?, ?, ?, ?, '127.0.0.1', 'test-agent', ?)`,
		id, providerID, action, targetKind, targetID, metaJSON, createdAt.UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("seed audit row: %v", err)
	}
	return id
}

// TestAuditLog_RequiresAdmin confirms the admin-only gate. Covered also by
// rbac_test.go; re-tested here to keep this file self-contained.
func TestAuditLog_RequiresAdmin(t *testing.T) {
	h := NewHarness(t)
	staffClient, _, _ := h.AuthAsStaff(t, "CA")
	resp, err := staffClient.Get(h.URL("/api/audit-log"))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("staff GET /api/audit-log: expected 403, got %d", resp.StatusCode)
	}
}

func TestAuditLog_RequiresAuth(t *testing.T) {
	h := NewHarness(t)
	resp, err := http.Get(h.URL("/api/audit-log"))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

// TestAuditLog_ReturnsSeededRows seeds 3 rows and verifies the admin list
// returns all 3 with correct provider scoping.
func TestAuditLog_ReturnsSeededRows(t *testing.T) {
	h := NewHarness(t)
	client, providerID, _ := h.AuthAs(t, "CA")
	_, otherPID, _ := h.AuthAs(t, "FL")

	base := time.Now()
	seedAuditRow(t, h, providerID, "child.create", "child", "c1", map[string]any{"name": "Ada"}, base.Add(-3*time.Minute))
	seedAuditRow(t, h, providerID, "child.update", "child", "c1", map[string]any{"field": "classroom"}, base.Add(-2*time.Minute))
	seedAuditRow(t, h, providerID, "child.delete", "child", "c1", nil, base.Add(-1*time.Minute))
	// Row belonging to a different provider.
	seedAuditRow(t, h, otherPID, "child.create", "child", "cx", map[string]any{"foreign": true}, base)

	resp, err := client.Get(h.URL("/api/audit-log"))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", resp.StatusCode, readAll(t, resp))
	}
	var page auditLogPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		resp.Body.Close()
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()

	if len(page.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(page.Items))
	}
	for _, it := range page.Items {
		if it.ProviderID != providerID {
			t.Fatalf("tenant isolation broken: item has provider_id %s, expected %s", it.ProviderID, providerID)
		}
	}
	// Newest-first order: delete, update, create.
	wantActions := []string{"child.delete", "child.update", "child.create"}
	for i, want := range wantActions {
		if page.Items[i].Action != want {
			t.Fatalf("expected action[%d]=%q, got %q", i, want, page.Items[i].Action)
		}
	}
}

// TestAuditLog_FilterByAction verifies ?action=foo narrows the result set.
func TestAuditLog_FilterByAction(t *testing.T) {
	h := NewHarness(t)
	client, providerID, _ := h.AuthAs(t, "CA")

	base := time.Now()
	seedAuditRow(t, h, providerID, "child.create", "child", "c1", nil, base.Add(-3*time.Minute))
	seedAuditRow(t, h, providerID, "child.update", "child", "c1", nil, base.Add(-2*time.Minute))
	seedAuditRow(t, h, providerID, "staff.create", "staff", "s1", nil, base.Add(-1*time.Minute))

	resp, err := client.Get(h.URL("/api/audit-log?action=staff.create"))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	var page auditLogPage
	mustDecode(t, resp, &page)
	if len(page.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(page.Items))
	}
	if page.Items[0].Action != "staff.create" {
		t.Fatalf("filter mismatch: got %q", page.Items[0].Action)
	}
}

// TestAuditLog_FilterByTargetKind narrows by target_kind.
func TestAuditLog_FilterByTargetKind(t *testing.T) {
	h := NewHarness(t)
	client, providerID, _ := h.AuthAs(t, "CA")

	base := time.Now()
	seedAuditRow(t, h, providerID, "child.create", "child", "c1", nil, base.Add(-2*time.Minute))
	seedAuditRow(t, h, providerID, "staff.create", "staff", "s1", nil, base.Add(-1*time.Minute))

	resp, err := client.Get(h.URL("/api/audit-log?target_kind=staff"))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	var page auditLogPage
	mustDecode(t, resp, &page)
	if len(page.Items) != 1 || page.Items[0].TargetKind != "staff" {
		t.Fatalf("target_kind filter mismatch: %+v", page.Items)
	}
}

// TestAuditLog_FilterBySinceUntil confines results to a time range.
func TestAuditLog_FilterBySinceUntil(t *testing.T) {
	h := NewHarness(t)
	client, providerID, _ := h.AuthAs(t, "CA")

	base := time.Now().Add(-1 * time.Hour).Truncate(time.Second)
	seedAuditRow(t, h, providerID, "a.first", "t", "1", nil, base)
	seedAuditRow(t, h, providerID, "a.middle", "t", "2", nil, base.Add(10*time.Minute))
	seedAuditRow(t, h, providerID, "a.last", "t", "3", nil, base.Add(30*time.Minute))

	// since = base+5m, until = base+20m → should include only 'a.middle'.
	q := url.Values{}
	q.Set("since", base.Add(5*time.Minute).UTC().Format(time.RFC3339))
	q.Set("until", base.Add(20*time.Minute).UTC().Format(time.RFC3339))
	resp, err := client.Get(h.URL("/api/audit-log?" + q.Encode()))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	var page auditLogPage
	mustDecode(t, resp, &page)
	if len(page.Items) != 1 || page.Items[0].Action != "a.middle" {
		t.Fatalf("since/until filter mismatch: %+v", page.Items)
	}
}

// TestAuditLog_InvalidSince returns 400.
func TestAuditLog_InvalidSince(t *testing.T) {
	h := NewHarness(t)
	client, _, _ := h.AuthAs(t, "CA")
	resp, err := client.Get(h.URL("/api/audit-log?since=notatime"))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad since: expected 400, got %d", resp.StatusCode)
	}
}

// TestAuditLog_PaginationLimitOffset verifies limit/offset give disjoint
// pages.
func TestAuditLog_PaginationLimitOffset(t *testing.T) {
	h := NewHarness(t)
	client, providerID, _ := h.AuthAs(t, "CA")

	base := time.Now().Add(-1 * time.Hour)
	for i := 0; i < 5; i++ {
		seedAuditRow(t, h, providerID, fmt.Sprintf("a.%d", i), "t", "x", nil, base.Add(time.Duration(i)*time.Minute))
	}

	// limit=2, offset=0 → 2 newest (a.4, a.3).
	first := getAuditPage(t, client, h.URL("/api/audit-log?limit=2&offset=0"))
	if len(first.Items) != 2 || first.Items[0].Action != "a.4" || first.Items[1].Action != "a.3" {
		t.Fatalf("first page mismatch: %+v", first.Items)
	}
	if first.NextCursor == "" {
		t.Fatalf("expected non-empty next_cursor on full page")
	}

	// offset=2 → next 2 (a.2, a.1).
	second := getAuditPage(t, client, h.URL("/api/audit-log?limit=2&offset=2"))
	if len(second.Items) != 2 || second.Items[0].Action != "a.2" || second.Items[1].Action != "a.1" {
		t.Fatalf("second page mismatch: %+v", second.Items)
	}

	// The two pages must be fully disjoint.
	for _, a := range first.Items {
		for _, b := range second.Items {
			if a.ID == b.ID {
				t.Fatalf("pages overlap on id=%s", a.ID)
			}
		}
	}
}

// TestAuditLog_MetadataIsObject confirms the metadata field is returned as a
// JSON object (not a string) so the frontend can render it without
// JSON.parse.
func TestAuditLog_MetadataIsObject(t *testing.T) {
	h := NewHarness(t)
	client, providerID, _ := h.AuthAs(t, "CA")

	seedAuditRow(t, h, providerID, "child.update", "child", "c1",
		map[string]any{"field": "classroom", "new_value": "Butterflies"},
		time.Now())

	resp, err := client.Get(h.URL("/api/audit-log"))
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	var page auditLogPage
	mustDecode(t, resp, &page)
	if len(page.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(page.Items))
	}
	meta := page.Items[0].Metadata
	if v, ok := meta["field"].(string); !ok || v != "classroom" {
		t.Fatalf("metadata.field not decoded as string: %+v", meta)
	}
}

// TestAuditLog_CorruptMetadata: if an operator manages to write an
// un-parseable metadata blob (shouldn't happen given json_valid CHECK, but
// the handler defends anyway), the handler falls back to {"_raw": "..."}
// rather than 500'ing.
//
// We can't easily insert invalid JSON because of the json_valid() CHECK
// constraint, so this test is a documentation of expected handler behaviour
// and is skipped.
func TestAuditLog_CorruptMetadata(t *testing.T) {
	t.Skip("json_valid() CHECK constraint prevents inserting corrupt metadata; handler fallback is covered by code inspection.")
}

// getAuditPage helper — GETs + decodes the paginated response.
func getAuditPage(t *testing.T, client *http.Client, url string) auditLogPage {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", resp.StatusCode, readAll(t, resp))
	}
	var page auditLogPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return page
}

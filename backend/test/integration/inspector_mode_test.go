package integration

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
)

// TestInspectorMode_Sample_ExcludesArchivedAndWithdrawn is the load-bearing
// guarantee for the Personnel sample / Children sample tabs: the sample
// must NEVER include archived staff or withdrawn children, even if the
// active pool is small. The inspector must not see ghosts.
func TestInspectorMode_Sample_ExcludesArchivedAndWithdrawn(t *testing.T) {
	h := NewHarness(t)
	client, providerID, _ := h.AuthAs(t, "FL")

	// Seed: 3 active staff, 2 terminated, 1 soft-deleted.
	activeIDs := map[string]bool{}
	for i, name := range []string{"Active1", "Active2", "Active3"} {
		id := base62.NewID()[:22]
		activeIDs[id] = true
		_, err := h.DB.Exec(`
			INSERT INTO staff (id, provider_id, first_name, last_name, status, created_at, updated_at)
			VALUES (?, ?, ?, 'Doe', 'active', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			id, providerID, name)
		if err != nil {
			t.Fatalf("insert active staff %d: %v", i, err)
		}
	}
	for i, name := range []string{"Termin1", "Termin2"} {
		id := base62.NewID()[:22]
		_, err := h.DB.Exec(`
			INSERT INTO staff (id, provider_id, first_name, last_name, status, created_at, updated_at)
			VALUES (?, ?, ?, 'Doe', 'terminated', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			id, providerID, name)
		if err != nil {
			t.Fatalf("insert terminated staff %d: %v", i, err)
		}
	}
	id := base62.NewID()[:22]
	if _, err := h.DB.Exec(`
		INSERT INTO staff (id, provider_id, first_name, last_name, status, created_at, updated_at, deleted_at)
		VALUES (?, ?, 'Deleted', 'Doe', 'active', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		id, providerID); err != nil {
		t.Fatalf("insert deleted staff: %v", err)
	}

	// Children: 2 enrolled, 1 withdrawn, 1 deleted.
	enrolledIDs := map[string]bool{}
	for _, name := range []string{"Aiden", "Bella"} {
		id := base62.NewID()[:22]
		enrolledIDs[id] = true
		_, err := h.DB.Exec(`
			INSERT INTO children (id, provider_id, first_name, last_name, date_of_birth, enrollment_date, created_at, updated_at)
			VALUES (?, ?, ?, 'Smith', '2022-01-01', '2025-08-01', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			id, providerID, name)
		if err != nil {
			t.Fatalf("insert child: %v", err)
		}
	}
	withdrawnID := base62.NewID()[:22]
	if _, err := h.DB.Exec(`
		INSERT INTO children (id, provider_id, first_name, last_name, date_of_birth, enrollment_date, withdrawal_date, created_at, updated_at)
		VALUES (?, ?, 'Withdrew', 'Smith', '2022-01-01', '2025-01-01', '2026-04-15', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		withdrawnID, providerID); err != nil {
		t.Fatalf("insert withdrawn child: %v", err)
	}
	deletedChildID := base62.NewID()[:22]
	if _, err := h.DB.Exec(`
		INSERT INTO children (id, provider_id, first_name, last_name, date_of_birth, enrollment_date, deleted_at, created_at, updated_at)
		VALUES (?, ?, 'Deleted', 'Smith', '2022-01-01', '2025-01-01', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		deletedChildID, providerID); err != nil {
		t.Fatalf("insert deleted child: %v", err)
	}

	// Start a session.
	resp := postJSON(t, client, h.URL("/api/inspection/sessions"), map[string]any{
		"counselor_name": "J. Carter",
		"type":           "routine",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("start session: expected 201, got %d (%s)", resp.StatusCode, readAll(t, resp))
	}
	var sess struct {
		ID string `json:"id"`
	}
	mustDecode(t, resp, &sess)
	if sess.ID == "" {
		t.Fatal("missing session id")
	}

	// Sample 5 staff — should return only the 3 active ones, no terminated/deleted.
	staffResp := postJSON(t, client, h.URL("/api/inspection/sessions/"+sess.ID+"/sample"),
		map[string]any{"kind": "staff", "count": 5})
	if staffResp.StatusCode != http.StatusOK {
		t.Fatalf("staff sample: expected 200, got %d (%s)", staffResp.StatusCode, readAll(t, staffResp))
	}
	var staffOut struct {
		Kind string   `json:"kind"`
		IDs  []string `json:"ids"`
	}
	mustDecode(t, staffResp, &staffOut)
	if len(staffOut.IDs) != 3 {
		t.Fatalf("expected 3 active staff sampled, got %d (%v)", len(staffOut.IDs), staffOut.IDs)
	}
	for _, id := range staffOut.IDs {
		if !activeIDs[id] {
			t.Fatalf("sampled non-active staff id %q", id)
		}
	}

	// Sample 3 children — only 2 enrolled exist.
	childResp := postJSON(t, client, h.URL("/api/inspection/sessions/"+sess.ID+"/sample"),
		map[string]any{"kind": "child", "count": 3})
	if childResp.StatusCode != http.StatusOK {
		t.Fatalf("child sample: expected 200, got %d (%s)", childResp.StatusCode, readAll(t, childResp))
	}
	var childOut struct {
		IDs []string `json:"ids"`
	}
	mustDecode(t, childResp, &childOut)
	if len(childOut.IDs) != 2 {
		t.Fatalf("expected 2 enrolled children sampled, got %d (%v)", len(childOut.IDs), childOut.IDs)
	}
	for _, id := range childOut.IDs {
		if !enrolledIDs[id] {
			t.Fatalf("sampled non-enrolled child id %q", id)
		}
	}
}

// TestInspectorMode_FindingThenEnd_GeneratesPDF covers the exit-conference
// happy path: add 2 findings, end the session, download the PDF, assert
// the bytes parse as a PDF and contain both findings.
func TestInspectorMode_FindingThenEnd_GeneratesPDF(t *testing.T) {
	h := NewHarness(t)
	client, _, _ := h.AuthAs(t, "FL")

	resp := postJSON(t, client, h.URL("/api/inspection/sessions"), map[string]any{
		"counselor_name": "J. Carter",
	})
	var sess struct {
		ID string `json:"id"`
	}
	mustDecode(t, resp, &sess)

	// Add 2 findings.
	for i, body := range []map[string]any{
		{"line_number": "5316 #11", "class": "Class II", "text": "Background screening missing for staff member."},
		{"line_number": "5316 #14", "class": "Class II", "text": "DH 680 not on file for child."},
	} {
		r := postJSON(t, client, h.URL("/api/inspection/sessions/"+sess.ID+"/finding"), body)
		if r.StatusCode != http.StatusCreated {
			t.Fatalf("finding %d: expected 201, got %d (%s)", i, r.StatusCode, readAll(t, r))
		}
		r.Body.Close()
	}

	// End session — should set ended_at and write the PDF path.
	endResp := postJSON(t, client, h.URL("/api/inspection/sessions/"+sess.ID+"/end"), map[string]any{})
	if endResp.StatusCode != http.StatusOK {
		t.Fatalf("end session: expected 200, got %d (%s)", endResp.StatusCode, readAll(t, endResp))
	}
	var ended struct {
		EndedAt     *string `json:"ended_at,omitempty"`
		PlanPDFPath string  `json:"plan_of_correction_pdf_path,omitempty"`
	}
	mustDecode(t, endResp, &ended)
	if ended.EndedAt == nil || *ended.EndedAt == "" {
		t.Fatal("expected ended_at to be set")
	}
	if ended.PlanPDFPath == "" {
		t.Fatal("expected plan_of_correction_pdf_path to be set")
	}

	// Trying to add another finding on an ended session must 409.
	postEnd := postJSON(t, client, h.URL("/api/inspection/sessions/"+sess.ID+"/finding"),
		map[string]any{"text": "Late finding"})
	if postEnd.StatusCode != http.StatusConflict {
		t.Fatalf("post-end finding: expected 409, got %d", postEnd.StatusCode)
	}
	postEnd.Body.Close()

	// Download the PDF.
	pdfResp, err := client.Get(h.URL("/api/inspection/sessions/" + sess.ID + "/plan-of-correction.pdf"))
	if err != nil {
		t.Fatalf("get plan: %v", err)
	}
	defer pdfResp.Body.Close()
	if pdfResp.StatusCode != http.StatusOK {
		t.Fatalf("plan pdf: expected 200, got %d", pdfResp.StatusCode)
	}
	if ct := pdfResp.Header.Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("unexpected content type %q", ct)
	}
	body := readAll(t, pdfResp)
	if !strings.HasPrefix(body, "%PDF-1.") {
		t.Fatalf("plan pdf does not start with PDF header: %q", body[:min(20, len(body))])
	}
	if !strings.Contains(body, "Background screening missing") {
		t.Fatal("expected first finding in PDF stream")
	}
	if !strings.Contains(body, "DH 680 not on file") {
		t.Fatal("expected second finding in PDF stream")
	}
}

// TestInspectorMode_Snapshot_HonestEmptyStates verifies the snapshot endpoint
// renders a baseline payload even when there is no data — no staff, no
// children, no postings, no drills. Inspector Mode must show those gaps,
// not 500.
func TestInspectorMode_Snapshot_HonestEmptyStates(t *testing.T) {
	h := NewHarness(t)
	client, _, _ := h.AuthAs(t, "FL")

	var snap map[string]any
	getJSON(t, client, h.URL("/api/inspection/snapshot"), &snap)

	// Facility card is always populated (provider row always exists).
	fc, ok := snap["facility"].(map[string]any)
	if !ok || fc["id"] == "" {
		t.Fatalf("expected facility card; got %+v", snap["facility"])
	}
	if fc["state_code"] != "FL" {
		t.Fatalf("expected state_code=FL, got %v", fc["state_code"])
	}

	// Drills block is present with an empty Logged slice and a 365-day window.
	drills, ok := snap["drills"].(map[string]any)
	if !ok {
		t.Fatalf("expected drills block; got %+v", snap["drills"])
	}
	if win, _ := drills["window_days"].(float64); win != 365 {
		t.Fatalf("expected drills window 365, got %v", drills["window_days"])
	}

	// Postings rollup includes every required FL posting kind even when
	// no posting_photos rows exist (they show as empty).
	postings, ok := snap["postings"].([]any)
	if !ok || len(postings) == 0 {
		t.Fatalf("expected postings list; got %+v", snap["postings"])
	}

	// Facility records 49-item rollup: every row Missing because nothing uploaded.
	records, ok := snap["facility_records"].([]any)
	if !ok || len(records) == 0 {
		t.Fatalf("expected facility_records list; got %+v", snap["facility_records"])
	}
	for _, r := range records {
		row := r.(map[string]any)
		if row["status"] != "Missing" {
			t.Fatalf("expected every record Missing on empty facility; got %v for %v", row["status"], row["label"])
		}
	}

	// generated_at is a valid ISO-8601 timestamp.
	g, _ := snap["generated_at"].(string)
	if _, err := time.Parse(time.RFC3339, g); err != nil {
		t.Fatalf("generated_at not RFC3339: %q", g)
	}
}

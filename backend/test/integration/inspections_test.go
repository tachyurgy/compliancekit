package integration

import (
	"net/http"
	"testing"
	"time"
)

// inspectionItem is a subset of inspection.Item for decoding the response.
type inspectionItem struct {
	ID       string `json:"id"`
	Domain   string `json:"domain"`
	Question string `json:"question"`
	Severity string `json:"severity"`
}

// runDTO is a subset of handlers.inspectionRunDTO for decoding.
type runDTO struct {
	ID          string     `json:"id"`
	State       string     `json:"state"`
	TotalItems  int        `json:"total_items"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Score       *int       `json:"score,omitempty"`
}

// checklistDTO mirrors inspection.Checklist for the subset we need.
type checklistDTO struct {
	State   string           `json:"state"`
	FormRef string           `json:"form_ref"`
	Items   []inspectionItem `json:"items"`
}

// startResponse is the shape of POST /api/inspections (wraps run + checklist).
type startResponse struct {
	Run       runDTO       `json:"run"`
	Checklist checklistDTO `json:"checklist"`
}

// finalizeResponse adds domains + citations to the detail DTO.
type finalizeResponse struct {
	Run             runDTO          `json:"run"`
	Checklist       checklistDTO    `json:"checklist"`
	DomainBreakdown []struct {
		Name   string `json:"name"`
		Total  int    `json:"total"`
		Passed int    `json:"passed"`
		Failed int    `json:"failed"`
	} `json:"domain_breakdown"`
	PredictedCitations []struct {
		ItemID   string `json:"item_id"`
		Domain   string `json:"domain"`
		Severity string `json:"severity"`
	} `json:"predicted_citations"`
}

// TestInspections_CA_FullLifecycle covers the whole flow:
//  1. POST starts a run with CA checklist (>30 items).
//  2. GET fetches the same run + checklist.
//  3. PATCH ~5 items to pass, ~2 items to fail.
//  4. POST finalize produces a score, domain breakdown, and citations.
//  5. GET list includes the run with completed_at set.
func TestInspections_CA_FullLifecycle(t *testing.T) {
	h := NewHarness(t)

	client, _, _ := h.AuthAs(t, "CA")

	// --- Start a run ---
	startResp := postJSON(t, client, h.URL("/api/inspections"), map[string]any{})
	if startResp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/inspections: expected 201, got %d (body=%s)",
			startResp.StatusCode, readAll(t, startResp))
	}
	var start startResponse
	mustDecode(t, startResp, &start)

	if start.Run.ID == "" {
		t.Fatalf("start response missing run.id")
	}
	if start.Run.State != "CA" {
		t.Fatalf("expected state=CA, got %q", start.Run.State)
	}
	if start.Run.TotalItems <= 30 {
		t.Fatalf("expected total_items > 30 for CA; got %d", start.Run.TotalItems)
	}
	if len(start.Checklist.Items) == 0 {
		t.Fatalf("checklist items should not be empty")
	}
	runID := start.Run.ID

	// --- GET the run ---
	var getOut startResponse
	getJSON(t, client, h.URL("/api/inspections/"+runID), &getOut)
	if getOut.Run.ID != runID {
		t.Fatalf("GET run mismatch: want %s, got %s", runID, getOut.Run.ID)
	}
	if len(getOut.Checklist.Items) != len(start.Checklist.Items) {
		t.Fatalf("GET checklist length %d != start checklist length %d",
			len(getOut.Checklist.Items), len(start.Checklist.Items))
	}

	// --- Answer items: 5 pass, 2 fail ---
	if len(start.Checklist.Items) < 7 {
		t.Fatalf("need at least 7 checklist items to exercise finalize; got %d", len(start.Checklist.Items))
	}
	passIDs := make([]string, 0, 5)
	failIDs := make([]string, 0, 2)
	for i := 0; i < 5; i++ {
		passIDs = append(passIDs, start.Checklist.Items[i].ID)
	}
	for i := 5; i < 7; i++ {
		failIDs = append(failIDs, start.Checklist.Items[i].ID)
	}

	for _, id := range passIDs {
		r := patchJSON(t, client, h.URL("/api/inspections/"+runID+"/items/"+id), map[string]any{
			"answer": "pass",
		})
		if r.StatusCode != http.StatusOK {
			t.Fatalf("PATCH pass %s: expected 200, got %d (body=%s)",
				id, r.StatusCode, readAll(t, r))
		}
		r.Body.Close()
	}
	for _, id := range failIDs {
		r := patchJSON(t, client, h.URL("/api/inspections/"+runID+"/items/"+id), map[string]any{
			"answer": "fail",
			"note":   "needs work",
		})
		if r.StatusCode != http.StatusOK {
			t.Fatalf("PATCH fail %s: expected 200, got %d (body=%s)",
				id, r.StatusCode, readAll(t, r))
		}
		r.Body.Close()
	}

	// --- Finalize ---
	finResp := postJSON(t, client, h.URL("/api/inspections/"+runID+"/finalize"), map[string]any{})
	if finResp.StatusCode != http.StatusOK {
		t.Fatalf("POST finalize: expected 200, got %d (body=%s)",
			finResp.StatusCode, readAll(t, finResp))
	}
	var fin finalizeResponse
	mustDecode(t, finResp, &fin)

	if fin.Run.Score == nil {
		t.Fatalf("finalize: score should be set")
	}
	if *fin.Run.Score < 0 || *fin.Run.Score > 100 {
		t.Fatalf("finalize: score out of range: %d", *fin.Run.Score)
	}
	if fin.Run.CompletedAt == nil {
		t.Fatalf("finalize: completed_at should be set")
	}
	if len(fin.DomainBreakdown) == 0 {
		t.Fatalf("finalize: domain_breakdown should be populated")
	}
	// All failed items we submitted should show up as predicted citations.
	if len(fin.PredictedCitations) < len(failIDs) {
		t.Fatalf("predicted_citations: expected at least %d, got %d",
			len(failIDs), len(fin.PredictedCitations))
	}
	for _, failID := range failIDs {
		matched := false
		for _, c := range fin.PredictedCitations {
			if c.ItemID == failID {
				matched = true
				break
			}
		}
		if !matched {
			t.Fatalf("predicted_citations missing failed item %s; got %+v",
				failID, fin.PredictedCitations)
		}
	}

	// --- List: the run should appear with completed_at set ---
	var list []runDTO
	getJSON(t, client, h.URL("/api/inspections"), &list)
	var matched *runDTO
	for i := range list {
		if list[i].ID == runID {
			matched = &list[i]
			break
		}
	}
	if matched == nil {
		t.Fatalf("list: did not find run %s", runID)
	}
	if matched.CompletedAt == nil {
		t.Fatalf("list: run %s should have completed_at set", runID)
	}
}

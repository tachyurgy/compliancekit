package integration

import (
	"net/http"
	"testing"
	"time"
)

// postingItem is the subset of handlers.PostingItem we assert against.
type postingItem struct {
	Key      string     `json:"key"`
	Label    string     `json:"label"`
	Required bool       `json:"required"`
	PostedAt *time.Time `json:"posted_at,omitempty"`
}

type postingsResponse struct {
	Items             []postingItem `json:"items"`
	AllRequiredPosted bool          `json:"all_required_posted"`
}

// TestPostings_FL_List_Upsert_CompletionFlag covers listing, per-key upsert,
// the providers.postings_complete flag flip, and state-specific checklists.
func TestPostings_FL_List_Upsert_CompletionFlag(t *testing.T) {
	h := NewHarness(t)

	client, providerID, _ := h.AuthAs(t, "FL")

	// --- GET: FL-specific items must be present. ---
	var resp postingsResponse
	getJSON(t, client, h.URL("/api/facility/postings"), &resp)

	wantFL := []string{
		"license",
		"ratio-poster",
		"evac-map",
		"menu",
		"dcf-hotline",
		"mandated-reporter",
	}
	for _, key := range wantFL {
		if !postingsContains(resp.Items, key) {
			t.Fatalf("FL postings missing required key %q; got %+v", key, resp.Items)
		}
	}

	if resp.AllRequiredPosted {
		t.Fatalf("initial FL postings: all_required_posted should be false before any PATCHes")
	}

	// --- PATCH just "license" with posted_at ---
	now := time.Now().UTC().Format(time.RFC3339)
	patchResp := patchJSON(t, client, h.URL("/api/facility/postings/license"), map[string]any{
		"posted_at": now,
	})
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH license: expected 200, got %d (body=%s)", patchResp.StatusCode, readAll(t, patchResp))
	}
	patchResp.Body.Close()

	// --- GET again: license has posted_at; all_required_posted still false. ---
	var afterOne postingsResponse
	getJSON(t, client, h.URL("/api/facility/postings"), &afterOne)
	if afterOne.AllRequiredPosted {
		t.Fatalf("all_required_posted should still be false with only license posted")
	}
	licItem := findPosting(afterOne.Items, "license")
	if licItem == nil || licItem.PostedAt == nil {
		t.Fatalf("license item missing posted_at after patch: %+v", licItem)
	}

	// --- Mark every *required* item. ---
	for _, it := range afterOne.Items {
		if !it.Required {
			continue
		}
		pResp := patchJSON(t, client, h.URL("/api/facility/postings/"+it.Key), map[string]any{
			"posted_at": now,
		})
		if pResp.StatusCode != http.StatusOK {
			t.Fatalf("PATCH %s: expected 200, got %d (body=%s)", it.Key, pResp.StatusCode, readAll(t, pResp))
		}
		pResp.Body.Close()
	}

	// --- GET: all_required_posted should now be true. ---
	var afterAll postingsResponse
	getJSON(t, client, h.URL("/api/facility/postings"), &afterAll)
	if !afterAll.AllRequiredPosted {
		t.Fatalf("after marking all required: expected all_required_posted=true, got %+v", afterAll)
	}

	// --- providers.postings_complete should be 1 in DB. ---
	var flag int
	if err := h.DB.QueryRow(
		`SELECT postings_complete FROM providers WHERE id = ?`, providerID,
	).Scan(&flag); err != nil {
		t.Fatalf("query postings_complete: %v", err)
	}
	if flag != 1 {
		t.Fatalf("providers.postings_complete: expected 1, got %d", flag)
	}

	// --- State-specific check: a CA provider sees different items. ---
	caClient, _, _ := h.AuthAs(t, "CA")
	var caResp postingsResponse
	getJSON(t, caClient, h.URL("/api/facility/postings"), &caResp)
	// LIC 203 is a CA-only key; dcf-hotline is FL-only.
	if !postingsContains(caResp.Items, "license-LIC203") {
		t.Fatalf("CA postings missing CA-only key license-LIC203; got %+v", caResp.Items)
	}
	if postingsContains(caResp.Items, "dcf-hotline") {
		t.Fatalf("CA postings unexpectedly contain FL-only key dcf-hotline")
	}
}

// postingsContains reports whether any item has the given key.
func postingsContains(items []postingItem, key string) bool {
	for _, it := range items {
		if it.Key == key {
			return true
		}
	}
	return false
}

// findPosting returns a pointer to the item with the given key, or nil.
func findPosting(items []postingItem, key string) *postingItem {
	for i := range items {
		if items[i].Key == key {
			return &items[i]
		}
	}
	return nil
}

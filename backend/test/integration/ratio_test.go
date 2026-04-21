package integration

import (
	"net/http"
	"testing"
)

// ratioCheckResponse mirrors handlers.ratioCheckResponse for JSON decoding.
type ratioCheckResponse struct {
	OK    bool `json:"ok"`
	Rooms []struct {
		Label       string  `json:"label"`
		RatioCap    int     `json:"ratio_cap"`
		ActualRatio float64 `json:"actual_ratio"`
		InRatio     bool    `json:"in_ratio"`
	} `json:"rooms"`
	ViolatedRooms []string `json:"violated_rooms"`
}

// TestRatio_TX_InfantFailsThenPasses covers:
//  - TX infant room (0-11mo, 5 kids / 1 staff) is out of ratio (cap 1:4).
//  - TX preschool 3 (36-47mo, 15 kids / 1 staff) is in ratio (cap 1:15).
//  - providers.ratio_ok is 0 after the mixed result, 1 once infant is fixed.
func TestRatio_TX_InfantFailsThenPasses(t *testing.T) {
	h := NewHarness(t)

	client, providerID, _ := h.AuthAs(t, "TX")

	body := map[string]any{
		"rooms": []map[string]any{
			{
				"label":            "Infant",
				"age_months_low":   0,
				"age_months_high":  11,
				"children_present": 5,
				"staff_present":    1,
			},
			{
				"label":            "Preschool 3",
				"age_months_low":   36,
				"age_months_high":  47,
				"children_present": 15,
				"staff_present":    1,
			},
		},
	}
	resp := postJSON(t, client, h.URL("/api/facility/ratio-check"), body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST ratio-check: expected 200, got %d (body=%s)", resp.StatusCode, readAll(t, resp))
	}
	var out ratioCheckResponse
	mustDecode(t, resp, &out)

	if out.OK {
		t.Fatalf("mixed ratio check: expected ok=false, got true")
	}

	// Find rooms by label.
	var infant, preschool *struct {
		Label       string  `json:"label"`
		RatioCap    int     `json:"ratio_cap"`
		ActualRatio float64 `json:"actual_ratio"`
		InRatio     bool    `json:"in_ratio"`
	}
	for i := range out.Rooms {
		switch out.Rooms[i].Label {
		case "Infant":
			infant = &out.Rooms[i]
		case "Preschool 3":
			preschool = &out.Rooms[i]
		}
	}
	if infant == nil || preschool == nil {
		t.Fatalf("expected Infant + Preschool 3 in response, got %+v", out.Rooms)
	}
	if infant.InRatio {
		t.Fatalf("infant room: expected in_ratio=false, got true (cap=%d)", infant.RatioCap)
	}
	if !preschool.InRatio {
		t.Fatalf("preschool 3 room: expected in_ratio=true, got false (cap=%d)", preschool.RatioCap)
	}

	// Confirm ViolatedRooms lists Infant.
	found := false
	for _, v := range out.ViolatedRooms {
		if v == "Infant" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("violated_rooms should include Infant; got %+v", out.ViolatedRooms)
	}

	// providers.ratio_ok should be 0.
	var flag int
	if err := h.DB.QueryRow(
		`SELECT ratio_ok FROM providers WHERE id = ?`, providerID,
	).Scan(&flag); err != nil {
		t.Fatalf("query ratio_ok: %v", err)
	}
	if flag != 0 {
		t.Fatalf("after mixed ratio check: expected providers.ratio_ok=0, got %d", flag)
	}

	// --- Fix the infant room to 4/1; re-check. ---
	body2 := map[string]any{
		"rooms": []map[string]any{
			{
				"label":            "Infant",
				"age_months_low":   0,
				"age_months_high":  11,
				"children_present": 4,
				"staff_present":    1,
			},
			{
				"label":            "Preschool 3",
				"age_months_low":   36,
				"age_months_high":  47,
				"children_present": 15,
				"staff_present":    1,
			},
		},
	}
	resp2 := postJSON(t, client, h.URL("/api/facility/ratio-check"), body2)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("POST ratio-check (fixed): expected 200, got %d", resp2.StatusCode)
	}
	var out2 ratioCheckResponse
	mustDecode(t, resp2, &out2)
	if !out2.OK {
		t.Fatalf("after fix: expected ok=true, got false (%+v)", out2)
	}

	var flag2 int
	if err := h.DB.QueryRow(
		`SELECT ratio_ok FROM providers WHERE id = ?`, providerID,
	).Scan(&flag2); err != nil {
		t.Fatalf("query ratio_ok (after fix): %v", err)
	}
	if flag2 != 1 {
		t.Fatalf("after fix: expected providers.ratio_ok=1, got %d", flag2)
	}
}

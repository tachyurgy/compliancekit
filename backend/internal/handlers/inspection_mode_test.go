package handlers

import (
	"strings"
	"testing"
	"time"
)

// TestPickRandom_ExcludesNothingWhenPoolFitsInCount documents the rule the
// inspector cares about: if the operator only has 2 active staff and asks
// for a sample of 3, we don't 500 — we just return both. We also don't
// shuffle, so re-running with the same input produces the same order.
func TestPickRandom_ExcludesNothingWhenPoolFitsInCount(t *testing.T) {
	in := []string{"a", "b"}
	out := pickRandom(in, 3)
	if len(out) != 2 {
		t.Fatalf("expected 2, got %d (%v)", len(out), out)
	}
	if out[0] != "a" || out[1] != "b" {
		t.Fatalf("expected stable order [a b], got %v", out)
	}
}

// TestPickRandom_DistinctElements asserts the sample never duplicates.
func TestPickRandom_DistinctElements(t *testing.T) {
	in := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	for trial := 0; trial < 25; trial++ {
		out := pickRandom(in, 3)
		seen := map[string]bool{}
		for _, id := range out {
			if seen[id] {
				t.Fatalf("trial %d: duplicate id %q in %v", trial, id, out)
			}
			seen[id] = true
		}
		if len(out) != 3 {
			t.Fatalf("trial %d: want len 3, got %d", trial, len(out))
		}
	}
}

// TestPickRandom_EmptyPool returns empty slice (not nil).
func TestPickRandom_EmptyPool(t *testing.T) {
	out := pickRandom(nil, 3)
	if out == nil {
		t.Fatal("expected non-nil empty slice")
	}
	if len(out) != 0 {
		t.Fatalf("expected empty, got %v", out)
	}
}

// TestExpirationStatus covers the four cases the inspector sees:
// no expiration captured (we say "On file" — honest), 90+ days out, within
// 90 days, and past.
func TestExpirationStatus(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
	}{
		{"empty", "", "On file"},
		{"future_far", time.Now().AddDate(1, 0, 0).Format("2006-01-02"), "On file"},
		{"future_near", time.Now().AddDate(0, 1, 0).Format("2006-01-02"), "Expiring soon"},
		{"past", time.Now().AddDate(-1, 0, 0).Format("2006-01-02"), "Expired"},
		{"unparseable", "tomorrow", "On file"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := expirationStatus(c.input)
			if got != c.want {
				t.Fatalf("expirationStatus(%q) = %q, want %q", c.input, got, c.want)
			}
		})
	}
}

// TestBuildPlanOfCorrectionPDF_ProducesValidPDF asserts byte-level structure
// of the generated Plan of Correction. The inspector's iPad must open this in
// Safari/Files; that requires the %PDF header, an xref, and %%EOF marker.
func TestBuildPlanOfCorrectionPDF_ProducesValidPDF(t *testing.T) {
	fc := facilityCard{
		ID:            "p_test",
		Name:          "Sunshine Academy",
		LicenseNumber: "C20HE0099",
		LicenseExpiry: "2027-01-15",
		Address:       "123 Palm Way, Miami, FL, 33101",
		DirectorName:  "Maria Lopez",
		StateCode:     "FL",
	}
	ev := &inspectionEventDTO{
		ID:            "ev_demo",
		FacilityID:    "p_test",
		StartedAt:     "2026-05-09T14:00:00Z",
		CounselorName: "J. Carter (DCF)",
		Type:          "routine",
		Findings: []finding{
			{LineNumber: "5316 #11", Class: "Class II", Text: "Background screening eligible-result document missing for Sarah Johnson.", Note: "Re-running screening today."},
			{LineNumber: "5316 #14", Class: "Class II", Text: "DH 680 not on file for child Aiden M.", Note: ""},
		},
	}
	out := buildPlanOfCorrectionPDF(fc, ev)
	s := string(out)
	if !strings.HasPrefix(s, "%PDF-1.") {
		t.Fatalf("expected PDF header; got %q", s[:min(20, len(s))])
	}
	if !strings.Contains(s, "%%EOF") {
		t.Fatal(`expected EOF trailer marker`)
	}
	if !strings.Contains(s, "xref") {
		t.Fatal("expected xref table")
	}
	if !strings.Contains(s, "Sunshine Academy") {
		t.Fatal("expected facility name in stream")
	}
	if !strings.Contains(s, "C20HE0099") {
		t.Fatal("expected license number in stream")
	}
	if !strings.Contains(s, "Maria Lopez") {
		t.Fatal("expected director name")
	}
	// Findings text needs to make it through pdfEscape unchanged.
	if !strings.Contains(s, "Background screening eligible-result document") {
		t.Fatal("expected finding text in stream")
	}
}

// TestPDFEscape_ParensBackslashes covers the three characters the PDF spec
// requires escaping inside string literals.
func TestPDFEscape_ParensBackslashes(t *testing.T) {
	got := pdfEscape("Smith (J.) \\ note")
	want := `Smith \(J.\) \\ note`
	if got != want {
		t.Fatalf("pdfEscape mismatch: got %q want %q", got, want)
	}
}

// TestPDFEscape_NonASCIIDowngrade verifies typography substitutions so
// "Handbook § 7.4" renders as "Handbook Sec. 7.4" in the PDF (we don't
// embed a Unicode-capable font in v1).
func TestPDFEscape_NonASCIIDowngrade(t *testing.T) {
	got := pdfEscape("Handbook § 7.4 — done")
	if !strings.Contains(got, "Sec.") {
		t.Fatalf("expected '§' to become 'Sec.'; got %q", got)
	}
	if strings.ContainsRune(got, '—') {
		t.Fatalf("expected em-dash to be downgraded; got %q", got)
	}
}

// TestMapToPersonnelItems_MissingHonest verifies that staff with no documents
// at all show every checklist row as "Missing" — Inspector Mode exposes gaps,
// it never hides them.
func TestMapToPersonnelItems_MissingHonest(t *testing.T) {
	items := mapToPersonnelItems(handbookSection74(), nil)
	if len(items) == 0 {
		t.Fatal("expected at least one row for an empty staff doc set")
	}
	for _, it := range items {
		if it.Status != "Missing" {
			t.Fatalf("expected all rows Missing, got %q for %q", it.Status, it.Label)
		}
	}
}

// TestMapToPersonnelItems_MatchesByDocType verifies the doc_type alias list
// — we accept several common spellings per row, so an OCR doc with
// `bg_screening` matches the same Handbook row as `background_check`.
func TestMapToPersonnelItems_MatchesByDocType(t *testing.T) {
	docs := []ownedDoc{
		{ID: "d1", DocType: "background_check", ExpiresAt: time.Now().AddDate(2, 0, 0).Format("2006-01-02")},
		{ID: "d2", DocType: "cpr_certification", ExpiresAt: time.Now().AddDate(0, 1, 0).Format("2006-01-02")},
	}
	items := mapToPersonnelItems(handbookSection74(), docs)
	var bgFound, cprFound bool
	for _, it := range items {
		if strings.Contains(it.Label, "Background screening") {
			bgFound = true
			if it.Status != "On file" {
				t.Fatalf("BG row: want On file, got %q", it.Status)
			}
			if it.DocumentID != "d1" {
				t.Fatalf("BG row: want d1, got %q", it.DocumentID)
			}
		}
		if strings.Contains(it.Label, "Pediatric CPR") {
			cprFound = true
			if it.Status != "Expiring soon" {
				t.Fatalf("CPR row: want Expiring soon, got %q", it.Status)
			}
		}
	}
	if !bgFound || !cprFound {
		t.Fatalf("expected both BG and CPR rows present")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

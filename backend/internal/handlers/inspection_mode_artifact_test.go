//go:build artifact

// Run: go test -tags=artifact -run TestArtifact_PlanOfCorrection ./internal/handlers/
//
// Emits a sample Plan of Correction PDF for the human-review packet. Tagged
// `artifact` so it doesn't run as part of the normal test suite.
package handlers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestArtifact_PlanOfCorrection(t *testing.T) {
	fc := facilityCard{
		ID:            "p_demo",
		Name:          "Sunshine Academy of Miami",
		LicenseNumber: "C20HE0099",
		LicenseExpiry: "2027-01-15",
		Address:       "1450 Palm Avenue, Miami, FL, 33101",
		DirectorName:  "Maria Lopez",
		StateCode:     "FL",
		Capacity:      62,
	}
	ev := &inspectionEventDTO{
		ID:            "ev_demo20260509",
		FacilityID:    "p_demo",
		StartedAt:     "2026-05-09T14:00:00Z",
		CounselorName: "J. Carter (DCF Region 11)",
		Type:          "routine",
		Findings: []finding{
			{
				LineNumber: "5316 #14",
				Class:      "Class II",
				Text:       "DH 680 Florida Certificate of Immunization not on file for child Aiden Martinez (Handbook § 7.1, § 21).",
				Note:       "Pediatrician fax requested 5/9; expected 5/12.",
				AddedAt:    "2026-05-09T14:18:00Z",
			},
			{
				LineNumber: "5316 #08",
				Class:      "Class II",
				Text:       "10-hour annual in-service training (CF-FSP 5268) not documented for current fiscal year for Sarah Johnson (Handbook § 4.2.7).",
				Note:       "Sarah will complete 10 hours by 5/30 via FL DCF training portal.",
				AddedAt:    "2026-05-09T14:22:00Z",
			},
			{
				LineNumber: "5316 #05",
				Class:      "Class III",
				Text:       "Current DCF license not posted in conspicuous location near the main entrance (Handbook § 2.1).",
				Note:       "License re-mounted next to lobby intercom 5/9.",
				AddedAt:    "2026-05-09T14:25:00Z",
			},
		},
	}
	out := buildPlanOfCorrectionPDF(fc, ev)
	dir := os.Getenv("ARTIFACT_DIR")
	if dir == "" {
		dir = "/Users/levelbrookteam/Desktop/daycare/.claude/worktrees/agent-aeaaface8e93cb2a3/human-review/2026-05-09-inspector-mode/artifacts"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "sample-plan-of-correction.pdf")
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Logf("wrote %s (%d bytes)", path, len(out))
}

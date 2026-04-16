package compliance

import (
	"fmt"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/models"
)

// RulesFL returns the Florida DCF Child Care Facility Handbook rule pack.
// References: F.A.C. 65C-22 (child care facilities); CF-FSP 5274 (immunization DH 680/681),
// CF-FSP 5316 (staff credentials), CF 1649A (background screening).
func RulesFL() []Rule {
	return []Rule{
		{
			ID:          "FL-CHILD-IMM",
			Title:       "Child immunization (DH 680) + physical (DH 3040)",
			Description: "FL DH 680 immunization form + DH 3040 student health exam on file for every child within 30 days of enrollment.",
			Category:    "child_files",
			Severity:    SeverityCritical,
			Reference:   "F.A.C. 65C-22.006(4)",
			FormRef:     "DH 680, DH 3040 (CF-FSP 5274 checklist)",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				missing := childrenNeedingImmunization(f, now)
				if len(missing) == 0 {
					return CheckResult{Satisfied: true}
				}
				return CheckResult{Violation: fmt.Sprintf("%d enrolled children missing DH 680.", len(missing)),
					FixHint: "Collect DH 680 (Florida Certification of Immunization)."}
			},
		},
		{
			ID:          "FL-CHILD-ENROLLMENT",
			Title:       "Enrollment packet complete",
			Description: "CF-FSP 5219 (app), CF-FSP 5220 (emergency), discipline policy signature, known-allergies form per child.",
			Category:    "child_files",
			Severity:    SeverityMajor,
			Reference:   "F.A.C. 65C-22.006(1)",
			FormRef:     "CF-FSP 5219, 5220",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				var missing int
				for _, c := range f.Children {
					if c.Status != "enrolled" {
						continue
					}
					if f.MostRecent("child", c.ID, models.DocEnrollmentForm, now) == nil {
						missing++
					}
				}
				if missing == 0 {
					return CheckResult{Satisfied: true}
				}
				return CheckResult{Violation: fmt.Sprintf("%d children missing enrollment packet.", missing),
					FixHint: "Collect CF-FSP 5219/5220 signed packet."}
			},
		},
		{
			ID:          "FL-STAFF-BACKGROUND",
			Title:       "Level 2 background screening",
			Description: "Level 2 screening via Clearinghouse + CF 1649A affidavit; rescreen every 5 years.",
			Category:    "staff_files",
			Severity:    SeverityCritical,
			Reference:   "F.S. §435.04; F.A.C. 65C-22.006",
			FormRef:     "CF 1649A; AHCA Clearinghouse",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				missing := staffMissingDoc(f, models.DocBackgroundCheck, now)
				if len(missing) == 0 {
					return CheckResult{Satisfied: true}
				}
				return CheckResult{Violation: fmt.Sprintf("%d staff missing Level 2 screening.", len(missing)),
					FixHint: "Submit Level 2 via Clearinghouse and CF 1649A."}
			},
		},
		{
			ID:          "FL-STAFF-CREDENTIAL",
			Title:       "DCF 40/45-hour training + 10 in-service hours/year",
			Description: "Staff must complete DCF-mandated 40/45-hour training + 10 hours annual in-service.",
			Category:    "staff_files",
			Severity:    SeverityMajor,
			Reference:   "F.A.C. 65C-22.003",
			FormRef:     "CF-FSP 5316 Staff Credential Form",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				missing := 0
				for _, s := range f.Staff {
					if s.Status != "active" {
						continue
					}
					if f.MostRecent("staff", s.ID, models.DocCPRCert, now) == nil {
						missing++
					}
				}
				if missing == 0 {
					return CheckResult{Satisfied: true}
				}
				return CheckResult{Violation: fmt.Sprintf("%d staff missing CPR/First Aid or in-service hours.", missing),
					FixHint: "Update staff CF-FSP 5316 with current training."}
			},
		},
		{
			ID:          "FL-STAFF-HEALTH",
			Title:       "Staff TB / health screening",
			Description: "TB screening upon hire; annual health re-verification per F.A.C. 65C-22.006.",
			Category:    "staff_files",
			Severity:    SeverityMajor,
			Reference:   "F.A.C. 65C-22.006(2)",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				missing := staffMissingDoc(f, models.DocTBTest, now)
				if len(missing) == 0 {
					return CheckResult{Satisfied: true}
				}
				return CheckResult{Violation: fmt.Sprintf("%d staff missing TB / health screen.", len(missing)),
					FixHint: "Collect current TB test results."}
			},
		},
		{
			ID:          "FL-FACILITY-LICENSE",
			Title:       "DCF license displayed",
			Description: "Current DCF license posted in public view.",
			Category:    "facility",
			Severity:    SeverityCritical,
			Reference:   "F.A.C. 65C-22.002",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				return facilityDocCheck(f, models.DocLicense, now, 90)
			},
		},
		{
			ID:          "FL-FACILITY-FIRE",
			Title:       "Annual fire inspection",
			Description: "Annual fire marshal inspection certificate on file.",
			Category:    "facility",
			Severity:    SeverityMajor,
			Reference:   "F.A.C. 65C-22.002(5)",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				return facilityDocCheck(f, models.DocFireInspection, now, 60)
			},
		},
		{
			ID:          "FL-DRILLS",
			Title:       "Monthly fire drills + quarterly emergency drills",
			Description: "Fire drills monthly; additional emergency (hurricane, lockdown) drills quarterly.",
			Category:    "operations",
			Severity:    SeverityMajor,
			Reference:   "F.A.C. 65C-22.001(8)",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				if f.DrillsLast90d >= 3 {
					return CheckResult{Satisfied: true}
				}
				return CheckResult{Violation: fmt.Sprintf("Only %d drills in last 90 days.", f.DrillsLast90d),
					FixHint: "Schedule fire drill now; log outcome."}
			},
		},
		{
			ID:          "FL-RATIOS",
			Title:       "Staff:child ratios",
			Description: "Ratios per F.A.C. 65C-22.001(4): 1:4 under 1yr, 1:6 1yr, 1:11 2yr, 1:15 3yr, 1:20 4yr, 1:25 5yr+.",
			Category:    "operations",
			Severity:    SeverityCritical,
			Reference:   "F.A.C. 65C-22.001(4)",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				if f.RatioOK {
					return CheckResult{Satisfied: true}
				}
				return CheckResult{Violation: "Staff:child ratios below FL minimums.",
					FixHint: "Add staff or close an age group."}
			},
		},
		{
			ID:          "FL-POSTINGS",
			Title:       "Required postings",
			Description: "License, last inspection report, menu, discipline policy, abuse hotline (1-800-96-ABUSE), sick-child policy.",
			Category:    "facility",
			Severity:    SeverityMinor,
			Reference:   "F.A.C. 65C-22.001(6)",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				if f.PostingsComplete {
					return CheckResult{Satisfied: true}
				}
				return CheckResult{Violation: "Required postings incomplete.",
					FixHint: "Post license, menu, discipline policy, abuse hotline, inspection report."}
			},
		},
	}
}

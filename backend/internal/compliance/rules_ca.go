package compliance

import (
	"fmt"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/models"
)

// RulesCA returns the CA Community Care Licensing Title 22 rule pack.
// References: Cal. Code Regs. tit. 22 §§ 101216 (immunization), 101170/101171 (TB/health screen),
// 101216.1 (LIC 9165 Physician's Report), 101212 (CPR/First Aid), 101170 (criminal record clearance).
func RulesCA() []Rule {
	return []Rule{
		{
			ID:          "CA-CHILD-IMM",
			Title:       "Child immunization records",
			Description: "Every enrolled child must have a current immunization record (CA Blue Card / CDPH 286).",
			Category:    "child_files",
			Severity:    SeverityCritical,
			Reference:   "CA H&SC §120335; CCR tit. 17 §6025",
			FormRef:     "CDPH 286 (Blue Card)",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				missing := childrenNeedingImmunization(f, now)
				if len(missing) == 0 {
					return CheckResult{Satisfied: true}
				}
				return CheckResult{
					Violation: fmt.Sprintf("%d enrolled children missing immunization records.", len(missing)),
					FixHint:   "Upload Blue Cards (CDPH 286) for listed children.",
				}
			},
		},
		{
			ID:          "CA-CHILD-ADMISSION",
			Title:       "Child admission agreement + emergency info",
			Description: "LIC 627 Admission Agreement + LIC 700 Identification & Emergency Info per child.",
			Category:    "child_files",
			Severity:    SeverityMajor,
			Reference:   "22 CCR §101220",
			FormRef:     "LIC 627, LIC 700",
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
				return CheckResult{Violation: fmt.Sprintf("%d children missing LIC 627/700.", missing),
					FixHint: "Collect signed admission + emergency forms."}
			},
		},
		{
			ID:          "CA-CHILD-PHYSICIAN-REPORT",
			Title:       "Physician's Report for each child",
			Description: "LIC 701 or LIC 9165 physician's report on file.",
			Category:    "child_files",
			Severity:    SeverityMajor,
			Reference:   "22 CCR §101220.1",
			FormRef:     "LIC 701 / LIC 9165",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				var missing int
				for _, c := range f.Children {
					if c.Status != "enrolled" {
						continue
					}
					if f.MostRecent("child", c.ID, models.DocPhysicalExam, now) == nil {
						missing++
					}
				}
				if missing == 0 {
					return CheckResult{Satisfied: true}
				}
				return CheckResult{Violation: fmt.Sprintf("%d children missing physician's report.", missing),
					FixHint: "Request parents submit LIC 701."}
			},
		},
		{
			ID:          "CA-STAFF-TB",
			Title:       "Staff TB clearance",
			Description: "Every staff member must have a valid TB risk-assessment or skin-test result.",
			Category:    "staff_files",
			Severity:    SeverityCritical,
			Reference:   "22 CCR §101216",
			FormRef:     "LIC 503",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				missing := staffMissingDoc(f, models.DocTBTest, now)
				if len(missing) == 0 {
					return CheckResult{Satisfied: true}
				}
				return CheckResult{Violation: fmt.Sprintf("%d staff missing or expired TB clearance.", len(missing)),
					FixHint: "Staff must update TB test and upload LIC 503."}
			},
		},
		{
			ID:          "CA-STAFF-CPR-FIRSTAID",
			Title:       "Director/teacher CPR + First Aid",
			Description: "CPR and Pediatric First Aid certification for every teacher/director (CCR §101216.3).",
			Category:    "staff_files",
			Severity:    SeverityMajor,
			Reference:   "22 CCR §101216.3",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				missing := 0
				for _, s := range f.Staff {
					if s.Status != "active" || (s.Role != "teacher" && s.Role != "director") {
						continue
					}
					cpr := f.MostRecent("staff", s.ID, models.DocCPRCert, now)
					fa := f.MostRecent("staff", s.ID, models.DocFirstAidCert, now)
					if cpr == nil || fa == nil || (cpr.ExpiresAt != nil && !cpr.ExpiresAt.After(now)) ||
						(fa.ExpiresAt != nil && !fa.ExpiresAt.After(now)) {
						missing++
					}
				}
				if missing == 0 {
					return CheckResult{Satisfied: true}
				}
				return CheckResult{Violation: fmt.Sprintf("%d teaching staff missing current CPR/First Aid.", missing),
					FixHint: "Renew CPR/First Aid certs (ARC or AHA)."}
			},
		},
		{
			ID:          "CA-STAFF-BACKGROUND",
			Title:       "Criminal record clearance (fingerprint)",
			Description: "LIC 508 Criminal Record Statement + TrustLine/DOJ clearance for every employee before contact with children.",
			Category:    "staff_files",
			Severity:    SeverityCritical,
			Reference:   "CA H&SC §1596.871; 22 CCR §101170",
			FormRef:     "LIC 508",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				missing := staffMissingDoc(f, models.DocBackgroundCheck, now)
				if len(missing) == 0 {
					return CheckResult{Satisfied: true}
				}
				return CheckResult{Violation: fmt.Sprintf("%d staff missing criminal clearance.", len(missing)),
					FixHint: "Submit LIC 508 + fingerprint rolling (BCIA 8016)."}
			},
		},
		{
			ID:          "CA-FACILITY-LICENSE",
			Title:       "Facility license displayed",
			Description: "Current CCL license (LIC 203) displayed at the facility.",
			Category:    "facility",
			Severity:    SeverityCritical,
			Reference:   "22 CCR §101205",
			FormRef:     "LIC 203",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				return facilityDocCheck(f, models.DocLicense, now, 90)
			},
		},
		{
			ID:          "CA-DRILLS",
			Title:       "Monthly emergency drills",
			Description: "Fire/earthquake/lockdown drills logged at least monthly.",
			Category:    "operations",
			Severity:    SeverityMajor,
			Reference:   "22 CCR §101174",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				if f.DrillsLast90d >= 3 {
					return CheckResult{Satisfied: true}
				}
				return CheckResult{Violation: fmt.Sprintf("Only %d drills logged in last 90 days; need 3+.", f.DrillsLast90d),
					FixHint: "Run a drill this week and log it."}
			},
		},
		{
			ID:          "CA-RATIOS",
			Title:       "Staff:child ratios",
			Description: "Ratios per 22 CCR §101216.3 (1:12 preschool, 1:4 infant, etc.).",
			Category:    "operations",
			Severity:    SeverityCritical,
			Reference:   "22 CCR §101216.3 / §101416.5",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				if f.RatioOK {
					return CheckResult{Satisfied: true}
				}
				return CheckResult{Violation: "Current staffing below required ratios.",
					FixHint: "Add qualified staff or reduce group sizes."}
			},
		},
		{
			ID:          "CA-POSTINGS",
			Title:       "Required wall postings",
			Description: "Evacuation map, license, nutrition statement, parents' rights (LIC 995), caregiver background checks policy posted.",
			Category:    "facility",
			Severity:    SeverityMinor,
			Reference:   "22 CCR §101229",
			FormRef:     "LIC 995, LIC 9212",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				if f.PostingsComplete {
					return CheckResult{Satisfied: true}
				}
				return CheckResult{Violation: "Wall postings incomplete.",
					FixHint: "Post LIC 995 parents' rights, evac map, license."}
			},
		},
	}
}

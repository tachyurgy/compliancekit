package compliance

import (
	"fmt"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/models"
)

// RulesTX returns Texas HHSC Chapter 746 Minimum Standards rule pack.
// Primary forms: 2935 (Staff Health Statement), 2941 (Employment App),
// 7259-7263 (child admission / permission / health), 1100 (background check).
func RulesTX() []Rule {
	return []Rule{
		{
			ID:          "TX-CHILD-IMM",
			Title:       "Child immunization records",
			Description: "Current immunization record for every enrolled child (TX DSHS schedule).",
			Category:    "child_files",
			Severity:    SeverityCritical,
			Reference:   "26 TAC §746.3607",
			FormRef:     "DSHS Official Immunization Record",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				missing := childrenNeedingImmunization(f, now)
				if len(missing) == 0 {
					return CheckResult{Satisfied: true}
				}
				return CheckResult{Violation: fmt.Sprintf("%d children missing TX immunization record.", len(missing)),
					FixHint: "Collect DSHS immunization record from each parent."}
			},
		},
		{
			ID:          "TX-CHILD-ADMISSION",
			Title:       "Child admission / emergency forms",
			Description: "Form 7259 (Admission Information) + 7260 (Health Statement) on file per child.",
			Category:    "child_files",
			Severity:    SeverityMajor,
			Reference:   "26 TAC §746.603",
			FormRef:     "HHSC Form 7259, 7260",
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
				return CheckResult{Violation: fmt.Sprintf("%d children missing admission/health forms.", missing),
					FixHint: "Collect signed HHSC 7259/7260."}
			},
		},
		{
			ID:          "TX-STAFF-HEALTH",
			Title:       "Staff Health Statement (Form 2935)",
			Description: "Every staff member must have a signed HHSC Form 2935 on file within 12 months of hire.",
			Category:    "staff_files",
			Severity:    SeverityMajor,
			Reference:   "26 TAC §746.1305",
			FormRef:     "HHSC Form 2935",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				missing := staffMissingDoc(f, models.DocTBTest, now)
				if len(missing) == 0 {
					return CheckResult{Satisfied: true}
				}
				return CheckResult{Violation: fmt.Sprintf("%d staff missing Form 2935 health statement.", len(missing)),
					FixHint: "Collect signed HHSC Form 2935."}
			},
		},
		{
			ID:          "TX-STAFF-BACKGROUND",
			Title:       "Background check (Form 1100)",
			Description: "HHSC background check + FBI fingerprint for every staff and volunteer >=14 years.",
			Category:    "staff_files",
			Severity:    SeverityCritical,
			Reference:   "26 TAC §745.615",
			FormRef:     "HHSC Form 1100",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				missing := staffMissingDoc(f, models.DocBackgroundCheck, now)
				if len(missing) == 0 {
					return CheckResult{Satisfied: true}
				}
				return CheckResult{Violation: fmt.Sprintf("%d staff missing Form 1100 background check.", len(missing)),
					FixHint: "Submit HHSC Form 1100 via Fast Pass."}
			},
		},
		{
			ID:          "TX-STAFF-PREREG-TRAINING",
			Title:       "Pre-service CPR/First Aid training",
			Description: "CPR + pediatric first aid certification within 90 days of hire.",
			Category:    "staff_files",
			Severity:    SeverityMajor,
			Reference:   "26 TAC §746.1309",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				missing := 0
				for _, s := range f.Staff {
					if s.Status != "active" {
						continue
					}
					cpr := f.MostRecent("staff", s.ID, models.DocCPRCert, now)
					fa := f.MostRecent("staff", s.ID, models.DocFirstAidCert, now)
					if cpr == nil || fa == nil {
						missing++
					}
				}
				if missing == 0 {
					return CheckResult{Satisfied: true}
				}
				return CheckResult{Violation: fmt.Sprintf("%d staff missing current CPR/First Aid.", missing),
					FixHint: "Renew CPR + pediatric first aid certifications."}
			},
		},
		{
			ID:          "TX-FACILITY-LICENSE",
			Title:       "Operating permit displayed",
			Description: "Current HHSC permit displayed and not expired.",
			Category:    "facility",
			Severity:    SeverityCritical,
			Reference:   "26 TAC §746.301",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				return facilityDocCheck(f, models.DocLicense, now, 90)
			},
		},
		{
			ID:          "TX-FACILITY-FIRE",
			Title:       "Fire inspection (annual)",
			Description: "Annual fire marshal inspection certificate.",
			Category:    "facility",
			Severity:    SeverityMajor,
			Reference:   "26 TAC §746.5201",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				return facilityDocCheck(f, models.DocFireInspection, now, 60)
			},
		},
		{
			ID:          "TX-DRILLS",
			Title:       "Fire drills monthly + severe weather quarterly",
			Description: "Fire drills each month; severe-weather drills each quarter.",
			Category:    "operations",
			Severity:    SeverityMajor,
			Reference:   "26 TAC §746.5205",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				if f.DrillsLast90d >= 3 {
					return CheckResult{Satisfied: true}
				}
				return CheckResult{Violation: fmt.Sprintf("Only %d drills logged in last 90 days.", f.DrillsLast90d),
					FixHint: "Schedule a fire drill and log it today."}
			},
		},
		{
			ID:          "TX-RATIOS",
			Title:       "Staff:child ratios",
			Description: "Ratios per 26 TAC §746.1601 (e.g. 1:4 infant, 1:11 3yr, 1:18 4yr).",
			Category:    "operations",
			Severity:    SeverityCritical,
			Reference:   "26 TAC §746.1601",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				if f.RatioOK {
					return CheckResult{Satisfied: true}
				}
				return CheckResult{Violation: "Staffing below required TX ratios.",
					FixHint: "Add staff or reduce group sizes immediately."}
			},
		},
		{
			ID:          "TX-POSTINGS",
			Title:       "Required wall postings",
			Description: "Permit, last inspection report, menu, evac diagram, TX abuse-hotline poster.",
			Category:    "facility",
			Severity:    SeverityMinor,
			Reference:   "26 TAC §746.501",
			Check: func(f ProviderFacts, now time.Time) CheckResult {
				if f.PostingsComplete {
					return CheckResult{Satisfied: true}
				}
				return CheckResult{Violation: "Required postings incomplete.",
					FixHint: "Post permit, recent inspection, abuse hotline, menu, evac map."}
			},
		},
	}
}

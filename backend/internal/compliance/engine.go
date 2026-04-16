package compliance

import (
	"sort"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/models"
)

// Severity ranks how badly a missing/expired rule hurts.
type Severity string

const (
	SeverityCritical Severity = "critical" // blocks operation / immediate violation
	SeverityMajor    Severity = "major"    // violation at inspection
	SeverityMinor    Severity = "minor"    // recommendation / best-practice
)

// Rule is a pure data description of a compliance requirement.
// Check is a pure function — no I/O.
type Rule struct {
	ID          string
	Title       string
	Description string
	Category    string // child_files | staff_files | facility | operations | inspection
	Severity    Severity
	Reference   string // e.g. "CA Title 22 §101216", "TX HHSC 746.1305"
	FormRef     string // e.g. "LIC-281A", "HHSC Form 2935", "CF-FSP 5274"
	Check       func(ProviderFacts, time.Time) CheckResult
}

type CheckResult struct {
	Satisfied  bool
	Violation  string     // human-readable reason, empty if satisfied
	UpcomingAt *time.Time // non-nil if NOT yet violated but will be within the horizon
	FixHint    string     // "Upload immunization record for Maya M."
}

// ProviderFacts is the snapshot the engine evaluates against.
// Built by the caller from DB rows; engine never touches the DB itself.
type ProviderFacts struct {
	Provider models.Provider
	Children []models.Child
	Staff    []models.Staff
	// key: subjectKind|subjectID|kind
	Documents map[string][]models.Document
	// Facility-level (subjectKind = "facility") docs are under "facility||<kind>"
	RatioOK         bool // caller computed; if false engine flags
	PostingsComplete bool
	DrillsLast90d   int
}

// DocsFor returns every non-deleted document of the given kind for a subject.
func (f ProviderFacts) DocsFor(subjectKind, subjectID string, kind models.DocumentKind) []models.Document {
	if f.Documents == nil {
		return nil
	}
	key := subjectKind + "|" + subjectID + "|" + string(kind)
	return f.Documents[key]
}

// MostRecent returns the latest non-expired document of a kind for a subject,
// or the most-recently-expired one if none are current.
func (f ProviderFacts) MostRecent(subjectKind, subjectID string, kind models.DocumentKind, now time.Time) *models.Document {
	docs := f.DocsFor(subjectKind, subjectID, kind)
	if len(docs) == 0 {
		return nil
	}
	var current, stale *models.Document
	for i := range docs {
		d := &docs[i]
		if d.DeletedAt != nil {
			continue
		}
		if d.ExpiresAt == nil || d.ExpiresAt.After(now) {
			if current == nil || (d.ExpiresAt != nil && (current.ExpiresAt == nil || d.ExpiresAt.After(*current.ExpiresAt))) {
				current = d
			}
		} else if stale == nil || d.ExpiresAt.After(*stale.ExpiresAt) {
			stale = d
		}
	}
	if current != nil {
		return current
	}
	return stale
}

// Report is the engine's full output.
type Report struct {
	Score                 int                `json:"score"` // 0..100
	Violations            []Violation        `json:"violations"`
	UpcomingDeadlines90d  []UpcomingDeadline `json:"upcoming_deadlines_90d"`
	GeneratedAt           time.Time          `json:"generated_at"`
	RulesEvaluated        int                `json:"rules_evaluated"`
}

type Violation struct {
	RuleID      string   `json:"rule_id"`
	Title       string   `json:"title"`
	Severity    Severity `json:"severity"`
	Description string   `json:"description"`
	Reference   string   `json:"reference,omitempty"`
	FormRef     string   `json:"form_ref,omitempty"`
	FixHint     string   `json:"fix_hint,omitempty"`
}

type UpcomingDeadline struct {
	RuleID    string    `json:"rule_id"`
	Title     string    `json:"title"`
	DueAt     time.Time `json:"due_at"`
	DaysAway  int       `json:"days_away"`
	Severity  Severity  `json:"severity"`
	FormRef   string    `json:"form_ref,omitempty"`
	FixHint   string    `json:"fix_hint,omitempty"`
}

// Evaluate runs every rule for the state and returns a Report.
// Pure function: no I/O, deterministic given inputs (aside from `now`).
func Evaluate(state models.StateCode, facts *ProviderFacts) *Report {
	return EvaluateAt(state, facts, time.Now())
}

func EvaluateAt(state models.StateCode, facts *ProviderFacts, now time.Time) *Report {
	rules := rulesFor(state)
	r := &Report{GeneratedAt: now, RulesEvaluated: len(rules)}
	weightedSum := 0.0
	totalWeight := 0.0

	for _, rule := range rules {
		w := severityWeight(rule.Severity)
		totalWeight += w
		cr := rule.Check(*facts, now)
		if cr.Satisfied {
			// Fully satisfied counts toward score even if an upcoming deadline is flagged.
			weightedSum += w
		} else if cr.Violation != "" {
			r.Violations = append(r.Violations, Violation{
				RuleID: rule.ID, Title: rule.Title, Severity: rule.Severity,
				Description: cr.Violation, Reference: rule.Reference, FormRef: rule.FormRef, FixHint: cr.FixHint,
			})
		}
		if cr.UpcomingAt != nil {
			days := int(cr.UpcomingAt.Sub(now).Hours() / 24)
			if days >= 0 && days <= 90 {
				r.UpcomingDeadlines90d = append(r.UpcomingDeadlines90d, UpcomingDeadline{
					RuleID: rule.ID, Title: rule.Title, DueAt: *cr.UpcomingAt, DaysAway: days,
					Severity: rule.Severity, FormRef: rule.FormRef, FixHint: cr.FixHint,
				})
			}
		}
	}
	if totalWeight > 0 {
		r.Score = int((weightedSum / totalWeight) * 100)
	} else {
		r.Score = 100
	}
	sort.Slice(r.Violations, func(i, j int) bool {
		return severityWeight(r.Violations[i].Severity) > severityWeight(r.Violations[j].Severity)
	})
	sort.Slice(r.UpcomingDeadlines90d, func(i, j int) bool {
		return r.UpcomingDeadlines90d[i].DueAt.Before(r.UpcomingDeadlines90d[j].DueAt)
	})
	return r
}

func rulesFor(state models.StateCode) []Rule {
	switch state {
	case models.StateCA:
		return RulesCA()
	case models.StateTX:
		return RulesTX()
	case models.StateFL:
		return RulesFL()
	default:
		return []Rule{}
	}
}

func severityWeight(s Severity) float64 {
	switch s {
	case SeverityCritical:
		return 5
	case SeverityMajor:
		return 3
	case SeverityMinor:
		return 1
	default:
		return 1
	}
}

// --- shared check helpers reused across state rule packs ---

// childrenNeedingImmunization returns children without a current, non-expired immunization record.
func childrenNeedingImmunization(f ProviderFacts, now time.Time) []models.Child {
	var out []models.Child
	for _, c := range f.Children {
		if c.Status != "enrolled" {
			continue
		}
		d := f.MostRecent("child", c.ID, models.DocImmunization, now)
		if d == nil || d.ExpiresAt == nil || !d.ExpiresAt.After(now) {
			out = append(out, c)
		}
	}
	return out
}

// staffMissingDoc returns all active staff lacking a current doc of the given kind.
func staffMissingDoc(f ProviderFacts, kind models.DocumentKind, now time.Time) []models.Staff {
	var out []models.Staff
	for _, s := range f.Staff {
		if s.Status != "active" {
			continue
		}
		d := f.MostRecent("staff", s.ID, kind, now)
		if d == nil || (d.ExpiresAt != nil && !d.ExpiresAt.After(now)) {
			out = append(out, s)
		}
	}
	return out
}

// facilityDocCheck checks a single facility-level document of a given kind.
func facilityDocCheck(f ProviderFacts, kind models.DocumentKind, now time.Time, warnDays int) CheckResult {
	d := f.MostRecent("facility", "", kind, now)
	if d == nil {
		return CheckResult{Violation: "No " + string(kind) + " on file.", FixHint: "Upload current " + string(kind)}
	}
	if d.ExpiresAt == nil {
		return CheckResult{Satisfied: true}
	}
	if !d.ExpiresAt.After(now) {
		return CheckResult{Violation: string(kind) + " expired on " + d.ExpiresAt.Format("2006-01-02"), FixHint: "Upload renewed " + string(kind)}
	}
	days := int(d.ExpiresAt.Sub(now).Hours() / 24)
	if days <= warnDays {
		exp := *d.ExpiresAt
		return CheckResult{Satisfied: true, UpcomingAt: &exp, FixHint: "Renew " + string(kind) + " before " + exp.Format("2006-01-02")}
	}
	return CheckResult{Satisfied: true}
}

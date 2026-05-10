package compliance

import (
	"sort"
	"strings"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/models"
)

// REQ-013: synthesised 0-100 dashboard score derived from the engine Report.
//
// The engine's existing weighted-sum score (engine.go) is correct for the
// CF-FSP 5316 rule pack, but it is *not* the number we want a tired Florida
// director to glance at on the dashboard.  Two reasons:
//
//   1. The engine score weights rules uniformly by Severity.  A center with
//      one missing background check (Critical) and a center with one missing
//      menu posting (Minor) both get a score that is hard to rank by impact.
//   2. The engine has no notion of "top 4 things to fix today".  REQ-013
//      requires four GapAction cards, deterministically ordered by impact
//      and deadline proximity.
//
// `Score` here lives between the engine and the dashboard handler.  It
// consumes the engine's Report (Violations + UpcomingDeadlines90d) and
// produces a Synth result tuned for human glance: integer score, calm band,
// and the four highest-impact GapActions.
//
// Severity-class weights — chosen to make the math reward progress:
//   class-1            -15 each, capped at 4 instances (-60)
//   class-1-procedural  -8 each, capped at 4 instances (-32)
//   class-2             -4 each, capped at 5 instances (-20)
//   class-2-posting     -3 each, capped at 5 instances (-15)
//   class-3             -1 each, no cap                  (advisory)
//
// The cap matters: without it, a center with 12 missing background checks
// scores 0 forever and the dashboard becomes useless. We surface the top
// four; once those are fixed the score recalculates and surfaces the next
// four. See REQ-013 § Non-obvious decisions.
const (
	weightClass1            = 15
	weightClass1Procedural  = 8
	weightClass2            = 4
	weightClass2Posting     = 3
	weightClass3            = 1
	capClass1               = 4
	capClass1Procedural     = 4
	capClass2               = 5
	capClass2Posting        = 5
)

// Band labels — used by the dashboard hero card to pick a color and copy.
const (
	BandExcellent = "excellent" // >= 95
	BandSolid     = "solid"     // 85..94
	BandAttention = "attention" // 70..84
	BandUrgent    = "urgent"    // < 70
)

// GapAction is one tap-card on the dashboard hero. The frontend renders it as
// a one-line title + subtitle + CTA that deep-links into the relevant flow.
type GapAction struct {
	Kind      string `json:"kind"`                // staff_doc_expired, child_imm_missing, posting_missing, drill_missing, ...
	Title     string `json:"title"`               // <= 6 words, plain English
	Subtitle  string `json:"subtitle"`            // <= 14 words
	CTALabel  string `json:"cta_label"`           // "Send reminder" / "Mark posted" / "Log drill"
	CTARoute  string `json:"cta_route"`           // app deep link, e.g. /staff/abc?action=nag
	StaffID   string `json:"staff_id,omitempty"`
	ChildID   string `json:"child_id,omitempty"`
	DueAt     *time.Time `json:"due_at,omitempty"`
	severity  Severity
	weight    int
}

// BreakdownRow lets the dashboard render a "why isn't it 100?" expander
// without a second round-trip.
type BreakdownRow struct {
	Category  string `json:"category"`
	Deduction int    `json:"deduction"` // negative number
	Count     int    `json:"count"`
}

// Synth is the synthesised score result that the dashboard handler returns.
type Synth struct {
	Score             int            `json:"score"`
	Band              string         `json:"band"`
	LastCalculatedAt  time.Time      `json:"last_calculated_at"`
	TopGaps           []GapAction    `json:"top_gaps"`
	Breakdown         []BreakdownRow `json:"breakdown"`
}

// classify maps an engine Violation to a (severity-class, weight, cap, kind)
// tuple. Inputs we have available: Severity (Critical/Major/Minor), RuleID
// prefix, Category. We use a small lookup keyed on RuleID prefix because
// the existing engine doesn't carry an explicit class-1/class-2 tag.
func classifyViolation(v Violation) (weight, cap int, category, kind string) {
	id := v.RuleID
	switch {
	case strings.Contains(id, "BACKGROUND"):
		return weightClass1, capClass1, "Background screening", "staff_background_missing"
	case strings.Contains(id, "STAFF-CREDENTIAL"), strings.Contains(id, "CPR"):
		return weightClass2, capClass2, "Staff credentials (CPR / training)", "staff_credential_expired"
	case strings.Contains(id, "STAFF-HEALTH"), strings.Contains(id, "TB"):
		return weightClass2, capClass2, "Staff health records", "staff_health_missing"
	case strings.Contains(id, "CHILD-IMM"):
		return weightClass1, capClass1, "Child immunization", "child_imm_missing"
	case strings.Contains(id, "CHILD-ENROLL"):
		return weightClass2, capClass2, "Child enrollment packets", "child_enrollment_incomplete"
	case strings.Contains(id, "POSTING"):
		return weightClass2Posting, capClass2Posting, "Required wall postings", "posting_missing"
	case strings.Contains(id, "DRILL"), strings.Contains(id, "FIRE"):
		return weightClass1Procedural, capClass1Procedural, "Fire / emergency drills", "drill_missing"
	case strings.Contains(id, "RATIO"):
		return weightClass1Procedural, capClass1Procedural, "Staff:child ratios", "ratio_out"
	case strings.Contains(id, "LICENSE"):
		return weightClass2, capClass2, "Facility license", "license_missing"
	case strings.Contains(id, "INSURANCE"):
		return weightClass2, capClass2, "Insurance", "insurance_missing"
	default:
		// Severity fallback — anything Critical we treat as class-1, Major as
		// class-2, Minor as class-3. Keeps unmapped rules from sliding through
		// at zero deduction.
		switch v.Severity {
		case SeverityCritical:
			return weightClass1, capClass1, v.Title, "violation_critical"
		case SeverityMajor:
			return weightClass2, capClass2, v.Title, "violation_major"
		default:
			return weightClass3, 100, v.Title, "violation_minor"
		}
	}
}

// SynthesizeScore consumes an engine Report + the provider's facts and
// returns the dashboard-shaped Synth result. Pure function — no I/O.
//
// Determinism: GapActions are sorted by (weight desc, deadline-proximity
// asc, RuleID asc). The frontend takes the first four.
func SynthesizeScore(report *Report, facts *ProviderFacts, now time.Time) *Synth {
	if report == nil {
		return &Synth{Score: 100, Band: BandExcellent, LastCalculatedAt: now}
	}

	// 1. Per-category aggregation. We use a category bucket so we don't take
	//    -15 four times for the same kind of issue beyond the cap.
	type bucket struct {
		category string
		weight   int
		cap      int
		count    int
	}
	buckets := map[string]*bucket{}
	gaps := []GapAction{}

	for _, v := range report.Violations {
		w, c, cat, kind := classifyViolation(v)
		b, ok := buckets[cat]
		if !ok {
			b = &bucket{category: cat, weight: w, cap: c}
			buckets[cat] = b
		}
		b.count++

		gaps = append(gaps, GapAction{
			Kind:     kind,
			Title:    plainTitle(v),
			Subtitle: plainSubtitle(v),
			CTALabel: ctaForKind(kind),
			CTARoute: routeForViolation(v, kind),
			severity: v.Severity,
			weight:   w,
		})
	}

	// Add any upcoming deadlines that are not already represented as
	// violations.  These are forward-looking gaps — "Sarah's CPR expires in
	// 12 days".  They don't deduct from the score (the engine has them as
	// satisfied today) but they DO count as candidates for the top-4 list.
	for _, u := range report.UpcomingDeadlines90d {
		if u.DaysAway > 30 {
			continue // only the next month surfaces as a gap card
		}
		gaps = append(gaps, GapAction{
			Kind:     "upcoming_" + slug(u.RuleID),
			Title:    "Coming up: " + truncWords(u.Title, 6),
			Subtitle: u.FixHint,
			CTALabel: "See details",
			CTARoute: "/dashboard?focus=" + slug(u.RuleID),
			DueAt:    &u.DueAt,
			severity: u.Severity,
			weight:   1,
		})
	}

	// 2. Compute total deduction with caps applied per bucket.
	totalDeduction := 0
	breakdown := []BreakdownRow{}
	for _, b := range buckets {
		instances := b.count
		if b.cap > 0 && instances > b.cap {
			instances = b.cap
		}
		ded := -1 * instances * b.weight
		totalDeduction += ded
		breakdown = append(breakdown, BreakdownRow{
			Category:  b.category,
			Deduction: ded,
			Count:     b.count,
		})
	}
	// Stable order: largest deduction first, then alphabetical.
	sort.Slice(breakdown, func(i, j int) bool {
		if breakdown[i].Deduction != breakdown[j].Deduction {
			return breakdown[i].Deduction < breakdown[j].Deduction
		}
		return breakdown[i].Category < breakdown[j].Category
	})

	score := 100 + totalDeduction
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	// 3. Order GapActions by impact and pick the top four.
	sort.SliceStable(gaps, func(i, j int) bool {
		if gaps[i].weight != gaps[j].weight {
			return gaps[i].weight > gaps[j].weight
		}
		// Prefer the most-overdue (DueAt earliest) when weights tie.
		ai := gaps[i].DueAt
		aj := gaps[j].DueAt
		switch {
		case ai != nil && aj != nil:
			return ai.Before(*aj)
		case ai != nil:
			return true
		case aj != nil:
			return false
		}
		return gaps[i].Kind < gaps[j].Kind
	})
	if len(gaps) > 4 {
		gaps = gaps[:4]
	}

	return &Synth{
		Score:            score,
		Band:             BandFor(score),
		LastCalculatedAt: now,
		TopGaps:          gaps,
		Breakdown:        breakdown,
	}
}

// BandFor returns the band string for a numeric score.
func BandFor(score int) string {
	switch {
	case score >= 95:
		return BandExcellent
	case score >= 85:
		return BandSolid
	case score >= 70:
		return BandAttention
	default:
		return BandUrgent
	}
}

// ---- copy helpers (kept here so the brand-voice strings are co-located) ----

func plainTitle(v Violation) string {
	// The engine's Description is operator-facing English already; trim if it
	// runs too long for a card.
	t := v.Description
	if t == "" {
		t = v.Title
	}
	return truncWords(t, 6)
}

func plainSubtitle(v Violation) string {
	if v.FixHint != "" {
		return truncWords(v.FixHint, 14)
	}
	return truncWords(v.Title, 14)
}

func ctaForKind(kind string) string {
	switch {
	case strings.HasPrefix(kind, "staff_") && (strings.Contains(kind, "cred") || strings.Contains(kind, "background") || strings.Contains(kind, "health")):
		return "Send reminder"
	case strings.HasPrefix(kind, "child_"):
		return "Send parent the link"
	case strings.HasPrefix(kind, "posting_"):
		return "Mark posted"
	case strings.HasPrefix(kind, "drill_"):
		return "Log a drill"
	case kind == "ratio_out":
		return "Open ratio coach"
	case kind == "license_missing", kind == "insurance_missing":
		return "Upload it"
	default:
		return "See details"
	}
}

func routeForViolation(v Violation, kind string) string {
	// Best-effort deep links. The frontend honors ?action= to auto-open the
	// relevant modal.
	switch {
	case strings.HasPrefix(kind, "staff_"):
		return "/staff?focus=" + slug(v.RuleID)
	case strings.HasPrefix(kind, "child_"):
		return "/children?focus=" + slug(v.RuleID)
	case strings.HasPrefix(kind, "posting_"):
		return "/operations?tab=postings"
	case strings.HasPrefix(kind, "drill_"):
		return "/operations?tab=drills&action=log"
	case kind == "ratio_out":
		return "/operations?tab=ratio"
	default:
		return "/dashboard?focus=" + slug(v.RuleID)
	}
}

func slug(s string) string {
	out := strings.ToLower(s)
	out = strings.ReplaceAll(out, " ", "-")
	return out
}

func truncWords(s string, n int) string {
	parts := strings.Fields(s)
	if len(parts) <= n {
		return s
	}
	return strings.Join(parts[:n], " ") + "…"
}

// Quiet the unused-import warning until models is needed downstream.
var _ = models.StateFL

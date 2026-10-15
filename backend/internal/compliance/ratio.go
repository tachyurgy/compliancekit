package compliance

// REQ-016: required-staff-floor calculation + the four global warnings.
//
// We deliberately do NOT model rooms (see active/product/inspection-tracking.html
// for the design rationale). Inputs are: every signed-in child's age and
// every clocked-in staff member with their role. Output is the integer
// minimum total staff per FL F.S. § 402.305(4) plus any active warnings.
//
// Pure function — no I/O. The handler in attendance.go calls into here and
// surfaces the result on /api/attendance/today.

import (
	"fmt"
	"time"
)

// ChildPresent is the minimal projection of a signed-in child the ratio
// math needs. AgeMonths is computed from DOB at the call site.
type ChildPresent struct {
	ID        string
	AgeMonths int
}

// StaffPresent is the minimal projection of a clocked-in staff member.
// Role is one of "director", "staff", "volunteer".
type StaffPresent struct {
	ID   string
	Role string
}

// FL ratio bands (F.S. § 402.305(4) + Handbook § 2.3.1):
//   under 12 mo   1:4
//   12–23 mo      1:6
//   24–35 mo      1:11
//   36–47 mo      1:15
//   48–59 mo      1:20
//   60 mo+        1:25
//
// "Required floor" is the ceiling-divide sum across bands: each band's
// minimum is ceil(kids_in_band / per_staff). Mixed-age groups are handled
// at the band level — a 1-year-old in the 1:6 band still costs at least
// 1 staff for that band, regardless of which physical room they sit in.

type bandStats struct {
	Label    string `json:"label"`
	Kids     int    `json:"kids"`
	PerStaff int    `json:"per_staff"`
	Required int    `json:"required"`
}

// ByBand is the per-band breakdown returned alongside the floor; the
// dashboard renders these as a small chip strip.
type ByBand struct {
	Under1  bandStats `json:"under_1"`
	M12To24 bandStats `json:"m12_24"`
	M24To36 bandStats `json:"m24_36"`
	M36To48 bandStats `json:"m36_48"`
	M48To60 bandStats `json:"m48_60"`
	M60Plus bandStats `json:"m60_plus"`
}

// Warning is a single calm coral coaching message surfaced on the dashboard.
type Warning struct {
	Code     string `json:"code"`
	Headline string `json:"headline"`
	Action   string `json:"action"`
}

// RatioResult is what the handler returns inside /api/attendance/today.
type RatioResult struct {
	ChildrenPresent int       `json:"children_present"`
	StaffPresent    int       `json:"staff_present"`
	RequiredFloor   int       `json:"required_floor"`
	ByBand          ByBand    `json:"by_band"`
	Warnings        []Warning `json:"warnings"`
}

// CalcRatio runs the full computation. now is injected so tests are
// deterministic; in production the handler passes time.Now().
func CalcRatio(kids []ChildPresent, staff []StaffPresent, now time.Time) RatioResult {
	bands := computeBands(kids)
	floor := bands.Under1.Required + bands.M12To24.Required + bands.M24To36.Required +
		bands.M36To48.Required + bands.M48To60.Required + bands.M60Plus.Required
	staffCount := len(staff)

	res := RatioResult{
		ChildrenPresent: len(kids),
		StaffPresent:    staffCount,
		RequiredFloor:   floor,
		ByBand:          bands,
		Warnings:        []Warning{},
	}

	// 1. Below the legal floor.
	if staffCount < floor {
		short := floor - staffCount
		res.Warnings = append(res.Warnings, Warning{
			Code:     "below_floor",
			Headline: fmt.Sprintf("You have %d children today and only %d staff clocked in.", len(kids), staffCount),
			Action:   fmt.Sprintf("The minimum required by ratio is %d. Get %s in.", floor, plural(short, "one more", fmt.Sprintf("%d more", short))),
		})
	}

	// 2. Infant present without dedicated coverage.
	if bands.Under1.Kids > 0 {
		// The 1:4 band requires its own coverage; surface a warning when the
		// floor for that band alone exceeds 1 (i.e., > 4 infants) OR when
		// staff is at the bare floor with no margin (= floor exactly), since
		// "dedicated" implies at least one infant-only adult.
		needs := bands.Under1.Required
		if needs >= 1 {
			res.Warnings = append(res.Warnings, Warning{
				Code:     "infant_dedicated",
				Headline: fmt.Sprintf("You have %s today.", plural(bands.Under1.Kids, "1 infant under 1", fmt.Sprintf("%d infants under 1", bands.Under1.Kids))),
				Action:   "State law requires at least 1 staff dedicated to the under-1 group at 1:4 ratio. Make sure your staffing reflects that today.",
			})
		}
	}

	// 3. Toddler-heavy day. (1-2yr olds are the 1:6 band; surface when their
	// alone-floor is high.)
	if bands.M12To24.Required >= 2 {
		res.Warnings = append(res.Warnings, Warning{
			Code:     "toddler_heavy",
			Headline: fmt.Sprintf("You have %d toddlers (1–2) today.", bands.M12To24.Kids),
			Action:   fmt.Sprintf("That band alone needs %d staff at 1:6. Plan your morning rotation accordingly.", bands.M12To24.Required),
		})
	}

	// 4. Director not on-site (Handbook § 4.6). Fire only at midday so we
	// don't spam the early-AM window when the director hasn't arrived yet.
	if now.Hour() >= 12 && now.Hour() < 18 {
		hasDirector := false
		for _, s := range staff {
			if s.Role == "director" {
				hasDirector = true
				break
			}
		}
		if !hasDirector && len(staff) > 0 {
			res.Warnings = append(res.Warnings, Warning{
				Code:     "director_offsite",
				Headline: "No director is clocked in right now.",
				Action:   "Handbook § 4.6 expects a director on-site for the majority of operating hours. Check that your director has clocked in or that a designated alternate is here.",
			})
		}
	}

	return res
}

// RequiredFloor is exported for tests — callers in production use CalcRatio
// directly and read .RequiredFloor off the result.
func RequiredFloor(kids []ChildPresent) int {
	b := computeBands(kids)
	return b.Under1.Required + b.M12To24.Required + b.M24To36.Required +
		b.M36To48.Required + b.M48To60.Required + b.M60Plus.Required
}

func computeBands(kids []ChildPresent) ByBand {
	b := ByBand{
		Under1:  bandStats{Label: "Under 1", PerStaff: 4},
		M12To24: bandStats{Label: "1–2 yr", PerStaff: 6},
		M24To36: bandStats{Label: "2–3 yr", PerStaff: 11},
		M36To48: bandStats{Label: "3–4 yr", PerStaff: 15},
		M48To60: bandStats{Label: "4–5 yr", PerStaff: 20},
		M60Plus: bandStats{Label: "5+ yr", PerStaff: 25},
	}
	for _, c := range kids {
		switch {
		case c.AgeMonths < 12:
			b.Under1.Kids++
		case c.AgeMonths < 24:
			b.M12To24.Kids++
		case c.AgeMonths < 36:
			b.M24To36.Kids++
		case c.AgeMonths < 48:
			b.M36To48.Kids++
		case c.AgeMonths < 60:
			b.M48To60.Kids++
		default:
			b.M60Plus.Kids++
		}
	}
	for _, bs := range []*bandStats{&b.Under1, &b.M12To24, &b.M24To36, &b.M36To48, &b.M48To60, &b.M60Plus} {
		if bs.Kids == 0 {
			bs.Required = 0
			continue
		}
		// ceiling division
		bs.Required = (bs.Kids + bs.PerStaff - 1) / bs.PerStaff
	}
	return b
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

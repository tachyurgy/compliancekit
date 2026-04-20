package compliance

import "github.com/tachyurgy/compliancekit/backend/internal/models"

// Ratio tables per state. Values encode the regulatory MAX children per single
// adult for the given age band. Numbers come from:
//
//	CA: 22 CCR §101216.3 (infant centers), §101416.5 (school-age centers).
//	    Infant <24mo: 1:4. Toddler/preschool 24-72mo: 1:12. School-age 72+mo: 1:14.
//	    (ComplianceKit uses 1:15 for school-age to match the published CCLD
//	    enforcement guidance; 14 is the baseline but licensing permits 15 with
//	    a fully-qualified teacher.)
//	TX: 26 TAC §746.1601 (minimum standards ratios table). Caps scale by age
//	    up to 1:26 for 6+yr. Infant 0-11mo capped at group size 10.
//	FL: 65C-22.001(4), F.A.C. Single-age rooms: <12mo 1:4, 12-<24mo 1:6,
//	    24-<36mo 1:11, 36-<48mo 1:15, 48-<60mo 1:20, 60+mo 1:25.
//
// These tables are deliberately hard-coded here (not DB-driven) because a
// ratio error is a licensing violation — the source of truth must be code that
// can be code-reviewed, not a config row a support rep can edit.

// MaxChildrenPerStaff returns the regulatory cap of children a single adult
// can supervise for the given age (in months) in the given state. A return of
// 0 means we do not know the rule for this jurisdiction — the caller should
// treat it as "unsupported" rather than "no limit".
func MaxChildrenPerStaff(state models.StateCode, ageMonths int) int {
	if ageMonths < 0 {
		ageMonths = 0
	}
	switch state {
	case models.StateCA:
		return maxCA(ageMonths)
	case models.StateTX:
		return maxTX(ageMonths)
	case models.StateFL:
		return maxFL(ageMonths)
	default:
		return 0
	}
}

func maxCA(m int) int {
	switch {
	case m < 24:
		return 4 // infant
	case m < 72:
		return 12 // toddler / preschool (24-72mo)
	default:
		return 15 // school-age
	}
}

func maxTX(m int) int {
	switch {
	case m < 12:
		return 4 // 0-11mo; group size also capped at 10 (enforced separately)
	case m < 18:
		return 5 // 12-17mo
	case m < 24:
		return 9 // 18-23mo
	case m < 36:
		return 11 // 2yr
	case m < 48:
		return 15 // 3yr
	case m < 60:
		return 18 // 4yr
	case m < 72:
		return 22 // 5yr
	default:
		return 26 // 6yr+
	}
}

func maxFL(m int) int {
	switch {
	case m < 12:
		return 4
	case m < 24:
		return 6
	case m < 36:
		return 11
	case m < 48:
		return 15
	case m < 60:
		return 20
	default:
		return 25
	}
}

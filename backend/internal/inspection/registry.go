package inspection

import "github.com/tachyurgy/compliancekit/backend/internal/models"

// For returns the inspector walk-through checklist for a given state.
// Unsupported states get an empty slice — the handler should reject with a
// 400 before calling this, but returning empty keeps the function total.
func For(state models.StateCode) []Item {
	switch state {
	case models.StateCA:
		return ChecklistCA()
	case models.StateTX:
		return ChecklistTX()
	case models.StateFL:
		return ChecklistFL()
	default:
		return []Item{}
	}
}

// FormRefFor returns the official form name used by a state inspector, for
// display in the UI and in the exported report.
func FormRefFor(state models.StateCode) string {
	switch state {
	case models.StateCA:
		return "LIC-9099 Licensing Review"
	case models.StateTX:
		return "HHSC Form 2936 + Records Evaluation"
	case models.StateFL:
		return "CF-FSP 5316 Standards Classification Summary"
	default:
		return ""
	}
}

// ChecklistFor returns the assembled Checklist wrapper with items + computed
// domain spans. Useful for the POST /api/inspections response body.
func ChecklistFor(state models.StateCode) Checklist {
	items := For(state)
	return Checklist{
		State:   string(state),
		FormRef: FormRefFor(state),
		Items:   items,
		Domains: DomainsOf(items),
	}
}

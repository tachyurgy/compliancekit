// Package inspection implements the Inspection Readiness Simulator: a
// walk-through of the same checklist a real state licensor uses, organized
// by domain, with the same wording and regulatory references on each item.
//
// Checklists are data-only. The engine that scores a run lives in handlers;
// this package intentionally has no DB or HTTP surface.
package inspection

// EvidenceKind describes what proof an inspector would expect for an item.
// The UI uses this to choose an input control (none, document picker, photo,
// or attestation checkbox).
type EvidenceKind string

const (
	EvidenceNone        EvidenceKind = "none"
	EvidenceDocument    EvidenceKind = "document"
	EvidencePhoto       EvidenceKind = "photo"
	EvidenceAttestation EvidenceKind = "attestation"
)

// Severity mirrors compliance.Severity so scores line up across the product.
type Severity string

const (
	SeverityCritical Severity = "critical" // class-1 / immediate-risk / operation-blocking
	SeverityMajor    Severity = "major"    // class-2 / citation-level violation
	SeverityMinor    Severity = "minor"    // class-3 / technical / recommendation
)

// Weight returns the scoring weight for a severity. Must match
// compliance.severityWeight so the simulator score uses the same rubric
// as the dashboard compliance score.
func (s Severity) Weight() float64 {
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

// Item is one checklist question — one line on the inspector's clipboard.
//
// Wording is deliberately plain-English ("Is your license posted where parents
// can see it?") rather than the regulatory text. Reference/FormRef surface the
// original citation for auditability.
type Item struct {
	ID           string       `json:"id"`            // stable per-state slug, e.g. "ca.admin.license_posted"
	Domain       string       `json:"domain"`        // human domain label, e.g. "Physical Plant"
	Question     string       `json:"question"`      // the question the inspector would ask
	Reference    string       `json:"reference"`     // regulation citation, e.g. "22 CCR §101216"
	FormRef      string       `json:"form_ref"`      // state form + item, e.g. "LIC 9099 Item 7"
	EvidenceKind EvidenceKind `json:"evidence_kind"` // what proof the inspector expects
	Severity     Severity     `json:"severity"`      // violation class / weight
}

// Domain is a convenience grouping used by the frontend wizard. It is derived
// from the ordered list of items — a domain ends when the next item's Domain
// differs. This keeps the checklist as a flat, ordered source of truth.
type Domain struct {
	Name       string `json:"name"`
	ItemCount  int    `json:"item_count"`
	StartIndex int    `json:"start_index"` // index of first item in the flat checklist
}

// Checklist is the full inspector walk-through for one state.
type Checklist struct {
	State   string   `json:"state"`
	FormRef string   `json:"form_ref"` // e.g. "LIC-9099", "Form 2936", "CF-FSP 5316"
	Items   []Item   `json:"items"`
	Domains []Domain `json:"domains"`
}

// DomainsOf computes the ordered domain spans from a flat, domain-grouped
// slice of items. Callers must supply items in domain order; mixing domains
// will produce fragmented spans and the UI progress bar will be wrong.
func DomainsOf(items []Item) []Domain {
	if len(items) == 0 {
		return nil
	}
	var domains []Domain
	cur := Domain{Name: items[0].Domain, StartIndex: 0}
	for i, it := range items {
		if it.Domain != cur.Name {
			domains = append(domains, cur)
			cur = Domain{Name: it.Domain, StartIndex: i}
		}
		cur.ItemCount++
	}
	domains = append(domains, cur)
	return domains
}

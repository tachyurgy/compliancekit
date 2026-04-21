package inspection_test

import (
	"strings"
	"testing"

	"github.com/tachyurgy/compliancekit/backend/internal/inspection"
	"github.com/tachyurgy/compliancekit/backend/internal/models"
)

// minItems is the floor we enforce per state. The checklists hand-authored in
// ca_lic9099.go / tx_form2936.go / fl_cffsp5316.go have 30+ items each; we
// refuse to let them regress below 25 so a half-written state doesn't ship.
const minItems = 25

func TestFor_EachState_ReturnsAtLeastMinItems(t *testing.T) {
	t.Parallel()
	cases := []models.StateCode{models.StateCA, models.StateTX, models.StateFL}
	for _, s := range cases {
		s := s
		t.Run(string(s), func(t *testing.T) {
			t.Parallel()
			items := inspection.For(s)
			if len(items) < minItems {
				t.Fatalf("state %s: got %d items, want >= %d", s, len(items), minItems)
			}
		})
	}
}

func TestFor_EachItem_HasRequiredFields(t *testing.T) {
	t.Parallel()
	for _, s := range []models.StateCode{models.StateCA, models.StateTX, models.StateFL} {
		s := s
		t.Run(string(s), func(t *testing.T) {
			t.Parallel()
			for i, it := range inspection.For(s) {
				if strings.TrimSpace(it.ID) == "" {
					t.Fatalf("[%s#%d] empty ID", s, i)
				}
				if strings.TrimSpace(it.Question) == "" {
					t.Fatalf("[%s#%d] empty Question: %+v", s, i, it)
				}
				if strings.TrimSpace(it.Domain) == "" {
					t.Fatalf("[%s#%d] empty Domain: %+v", s, i, it)
				}
				switch it.Severity {
				case inspection.SeverityCritical, inspection.SeverityMajor, inspection.SeverityMinor:
					// ok
				default:
					t.Fatalf("[%s#%d] invalid Severity %q", s, i, it.Severity)
				}
			}
		})
	}
}

func TestFor_NoDuplicateIDs(t *testing.T) {
	t.Parallel()
	for _, s := range []models.StateCode{models.StateCA, models.StateTX, models.StateFL} {
		s := s
		t.Run(string(s), func(t *testing.T) {
			t.Parallel()
			seen := map[string]int{}
			for i, it := range inspection.For(s) {
				if prev, dup := seen[it.ID]; dup {
					t.Fatalf("duplicate ID %q at indexes %d and %d", it.ID, prev, i)
				}
				seen[it.ID] = i
			}
		})
	}
}

func TestFor_IDPrefixMatchesState(t *testing.T) {
	t.Parallel()
	cases := []struct {
		s      models.StateCode
		prefix string
	}{
		{models.StateCA, "ca."},
		{models.StateTX, "tx."},
		{models.StateFL, "fl."},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(string(tc.s), func(t *testing.T) {
			t.Parallel()
			for _, it := range inspection.For(tc.s) {
				if !strings.HasPrefix(it.ID, tc.prefix) {
					t.Fatalf("id %q missing prefix %q", it.ID, tc.prefix)
				}
			}
		})
	}
}

func TestFormRefFor_KnownStates(t *testing.T) {
	t.Parallel()
	cases := map[models.StateCode]string{
		models.StateCA: "LIC-9099 Licensing Review",
		models.StateTX: "HHSC Form 2936 + Records Evaluation",
		models.StateFL: "CF-FSP 5316 Standards Classification Summary",
	}
	for s, want := range cases {
		if got := inspection.FormRefFor(s); got != want {
			t.Fatalf("FormRefFor(%s) = %q, want %q", s, got, want)
		}
	}
}

func TestFormRefFor_UnknownState_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	if got := inspection.FormRefFor(models.StateCode("NY")); got != "" {
		t.Fatalf("FormRefFor(NY) = %q, want empty", got)
	}
	if got := inspection.FormRefFor(models.StateCode("")); got != "" {
		t.Fatalf("FormRefFor('') = %q, want empty", got)
	}
}

func TestFor_UnknownState_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	if got := inspection.For(models.StateCode("NY")); len(got) != 0 {
		t.Fatalf("For(NY) len = %d, want 0", len(got))
	}
	if got := inspection.For(models.StateCode("")); len(got) != 0 {
		t.Fatalf("For('') len = %d, want 0", len(got))
	}
}

func TestChecklistFor_WrapsItemsAndDomains(t *testing.T) {
	t.Parallel()
	cl := inspection.ChecklistFor(models.StateCA)
	if cl.State != "CA" {
		t.Fatalf("State = %q", cl.State)
	}
	if cl.FormRef == "" {
		t.Fatal("FormRef empty")
	}
	if len(cl.Items) < minItems {
		t.Fatalf("items = %d, want >= %d", len(cl.Items), minItems)
	}
	if len(cl.Domains) == 0 {
		t.Fatal("domains empty")
	}
	// domains' item_counts should sum to len(items).
	total := 0
	for _, d := range cl.Domains {
		total += d.ItemCount
		if d.Name == "" {
			t.Fatal("domain with empty name")
		}
	}
	if total != len(cl.Items) {
		t.Fatalf("domains total = %d, items = %d", total, len(cl.Items))
	}
}

func TestChecklistFor_UnknownState(t *testing.T) {
	t.Parallel()
	cl := inspection.ChecklistFor(models.StateCode("ZZ"))
	if len(cl.Items) != 0 {
		t.Fatalf("items len = %d, want 0", len(cl.Items))
	}
	if len(cl.Domains) != 0 {
		t.Fatalf("domains len = %d, want 0", len(cl.Domains))
	}
}

func TestDomainsOf_EmptyInput(t *testing.T) {
	t.Parallel()
	if got := inspection.DomainsOf(nil); got != nil {
		t.Fatalf("DomainsOf(nil) = %v, want nil", got)
	}
	if got := inspection.DomainsOf([]inspection.Item{}); got != nil {
		t.Fatalf("DomainsOf([]) = %v, want nil", got)
	}
}

func TestDomainsOf_SpansComputedCorrectly(t *testing.T) {
	t.Parallel()
	items := []inspection.Item{
		{ID: "a.1", Domain: "A"},
		{ID: "a.2", Domain: "A"},
		{ID: "b.1", Domain: "B"},
		{ID: "c.1", Domain: "C"},
		{ID: "c.2", Domain: "C"},
		{ID: "c.3", Domain: "C"},
	}
	ds := inspection.DomainsOf(items)
	want := []inspection.Domain{
		{Name: "A", ItemCount: 2, StartIndex: 0},
		{Name: "B", ItemCount: 1, StartIndex: 2},
		{Name: "C", ItemCount: 3, StartIndex: 3},
	}
	if len(ds) != len(want) {
		t.Fatalf("domains = %+v, want %+v", ds, want)
	}
	for i, w := range want {
		if ds[i] != w {
			t.Fatalf("ds[%d] = %+v, want %+v", i, ds[i], w)
		}
	}
}

func TestSeverity_Weight(t *testing.T) {
	t.Parallel()
	cases := map[inspection.Severity]float64{
		inspection.SeverityCritical: 5,
		inspection.SeverityMajor:    3,
		inspection.SeverityMinor:    1,
		inspection.Severity("???"):  1, // default fallback
	}
	for s, want := range cases {
		if got := s.Weight(); got != want {
			t.Fatalf("Severity(%q).Weight() = %v, want %v", s, got, want)
		}
	}
}

package immunization

import "testing"

func TestRequired_Newborn(t *testing.T) {
	r := Required("CA", 0)
	if len(r) == 0 {
		t.Fatalf("expected at least HepB at birth")
	}
	found := false
	for _, i := range r {
		if i.Vaccine == "HepB" && i.DoseNumber == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected HepB dose 1 at age 0: %+v", r)
	}
}

func TestRequired_Kindergarten(t *testing.T) {
	r := Required("TX", 60) // 5 years
	vaccines := map[string]int{}
	for _, i := range r {
		vaccines[i.Vaccine]++
	}
	want := []string{"DTaP", "IPV", "MMR", "Varicella", "HepA", "HepB", "Hib", "PCV13", "Influenza"}
	for _, v := range want {
		if _, ok := vaccines[v]; !ok {
			t.Errorf("kindergarten child missing %s in schedule", v)
		}
	}
}

func TestAllVaccines(t *testing.T) {
	v := AllVaccines()
	// 10 vaccines required by spec
	if len(v) < 10 {
		t.Fatalf("expected >= 10 vaccines, got %d: %v", len(v), v)
	}
}

func TestDoseOrdering(t *testing.T) {
	// Every vaccine's doses must be monotonically ordered by MinAgeMonth.
	byVaccine := map[string][]int{}
	for _, i := range fullSchedule {
		byVaccine[i.Vaccine] = append(byVaccine[i.Vaccine], i.MinAgeMonth)
	}
	for v, ages := range byVaccine {
		for i := 1; i < len(ages); i++ {
			if ages[i] < ages[i-1] {
				t.Errorf("vaccine %s doses out of order: %v", v, ages)
			}
		}
	}
}

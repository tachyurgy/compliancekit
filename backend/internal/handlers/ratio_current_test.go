package handlers

import (
	"testing"
)

// Pure-function tests on classifyRoom — the load-bearing decision in
// REQ-039. Live in package handlers (not handlers_test) so we can hit
// unexported symbols.
func TestClassifyRoom(t *testing.T) {
	cases := []struct {
		name                      string
		children, staff, mandated int
		wantStatus                string
		wantSubstr                string
	}{
		{"no children", 0, 1, 4, "compliant", "No children"},
		{"children no staff", 3, 0, 4, "out_of_ratio", "at least one staff"},
		{"exactly at cap with 1 staff", 4, 1, 4, "tight", "edge of ratio"},
		{"under cap with margin", 2, 1, 4, "compliant", "within the state ratio"},
		{"over cap by one", 5, 1, 4, "out_of_ratio", "more staff"},
		{"over cap big spread", 12, 1, 4, "out_of_ratio", "more staff"},
		{"two-staff room comfortable", 5, 2, 4, "compliant", "within the state ratio"},
		{"two-staff room tight", 7, 2, 4, "tight", "edge of ratio"},
		{"infant 1:4 at 4-kid tight", 4, 1, 4, "tight", "edge of ratio"},
		{"5+ band lots of staff", 24, 2, 25, "compliant", "within the state ratio"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotStatus, gotMsg := classifyRoom(c.children, c.staff, c.mandated)
			if gotStatus != c.wantStatus {
				t.Errorf("status: want %q got %q (msg=%q)", c.wantStatus, gotStatus, gotMsg)
			}
			if c.wantSubstr != "" && !contains(gotMsg, c.wantSubstr) {
				t.Errorf("message %q does not contain %q", gotMsg, c.wantSubstr)
			}
		})
	}
}

func TestFindBandForAge(t *testing.T) {
	cases := []struct {
		months int
		want   string
	}{
		{0, "infant_0_11"},
		{5, "infant_0_11"},
		{11, "infant_0_11"},
		{12, "ones"},
		{23, "ones"},
		{24, "twos"},
		{36, "threes"},
		{48, "fours"},
		{60, "fives"},
		{71, "fives"},
		{72, "school_age"},
		{120, "school_age"},
	}
	for _, c := range cases {
		got := findBandForAge(c.months)
		if got == nil {
			t.Fatalf("findBandForAge(%d): nil", c.months)
		}
		if got.Value != c.want {
			t.Errorf("findBandForAge(%d): want %s got %s", c.months, c.want, got.Value)
		}
	}
}

func contains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	if len(needle) > len(haystack) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

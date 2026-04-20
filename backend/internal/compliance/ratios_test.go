package compliance

import (
	"testing"

	"github.com/tachyurgy/compliancekit/backend/internal/models"
)

// Table-driven tests for the per-state ratio lookup. Each row is a real
// age-band → cap pair taken from the state's published regulations.
// These tables ARE the product — if one of these asserts fails, we are
// quoting the wrong ratio to a customer, which is a liability.
func TestMaxChildrenPerStaff_CA(t *testing.T) {
	cases := []struct {
		ageMonths int
		want      int
		label     string
	}{
		{0, 4, "newborn"},
		{6, 4, "6-month-old"},
		{23, 4, "just-under-2y (still infant)"},
		{24, 12, "2y exact (toddler/preschool band begins)"},
		{48, 12, "4y (preschool)"},
		{71, 12, "5y11m (still preschool)"},
		{72, 15, "6y exact (school-age)"},
		{144, 15, "12y (school-age)"},
	}
	for _, c := range cases {
		got := MaxChildrenPerStaff(models.StateCA, c.ageMonths)
		if got != c.want {
			t.Errorf("CA %dmo (%s): got 1:%d, want 1:%d", c.ageMonths, c.label, got, c.want)
		}
	}
}

func TestMaxChildrenPerStaff_TX(t *testing.T) {
	// 26 TAC §746.1601 — Texas is the most age-banded of the three.
	cases := []struct {
		ageMonths int
		want      int
	}{
		{0, 4}, {6, 4}, {11, 4}, // 0-11mo
		{12, 5}, {17, 5}, // 12-17mo
		{18, 9}, {23, 9}, // 18-23mo
		{24, 11}, {35, 11}, // 2y
		{36, 15}, {47, 15}, // 3y
		{48, 18}, {59, 18}, // 4y
		{60, 22}, {71, 22}, // 5y
		{72, 26}, {144, 26}, // 6y+
	}
	for _, c := range cases {
		got := MaxChildrenPerStaff(models.StateTX, c.ageMonths)
		if got != c.want {
			t.Errorf("TX %dmo: got 1:%d, want 1:%d", c.ageMonths, got, c.want)
		}
	}
}

func TestMaxChildrenPerStaff_FL(t *testing.T) {
	// F.S. §402.305(4) / F.A.C. 65C-22.
	cases := []struct {
		ageMonths int
		want      int
	}{
		{0, 4}, {11, 4}, // <12mo
		{12, 6}, {23, 6}, // 1y
		{24, 11}, {35, 11}, // 2y
		{36, 15}, {47, 15}, // 3y
		{48, 20}, {59, 20}, // 4y
		{60, 25}, {144, 25}, // 5y+
	}
	for _, c := range cases {
		got := MaxChildrenPerStaff(models.StateFL, c.ageMonths)
		if got != c.want {
			t.Errorf("FL %dmo: got 1:%d, want 1:%d", c.ageMonths, got, c.want)
		}
	}
}

// Unsupported state returns 0 so callers can treat as "unknown jurisdiction"
// rather than falsely assuming an unbounded ratio.
func TestMaxChildrenPerStaff_UnsupportedState(t *testing.T) {
	for _, code := range []models.StateCode{"NY", "OR", "WA", "IL", ""} {
		if got := MaxChildrenPerStaff(code, 24); got != 0 {
			t.Errorf("unsupported state %q should return 0, got %d", code, got)
		}
	}
}

// Negative ages are clamped to 0 (treated as newborn). Defensive against UI
// bugs that might submit weird inputs.
func TestMaxChildrenPerStaff_NegativeAgeClampedToNewborn(t *testing.T) {
	if got := MaxChildrenPerStaff(models.StateCA, -5); got != 4 {
		t.Errorf("negative age should clamp to newborn band (1:4), got 1:%d", got)
	}
}

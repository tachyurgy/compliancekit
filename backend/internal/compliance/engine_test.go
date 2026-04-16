package compliance

import (
	"testing"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/models"
)

func mkDoc(kind models.DocumentKind, subjectKind, subjectID string, expires *time.Time) models.Document {
	return models.Document{
		ID: "doc-" + string(kind) + "-" + subjectID, Kind: kind,
		SubjectKind: subjectKind, SubjectID: subjectID, ExpiresAt: expires,
	}
}

func factsWithChild(expiresAt *time.Time) *ProviderFacts {
	child := models.Child{ID: "c1", FirstName: "Maya", LastName: "M", Status: "enrolled"}
	docs := map[string][]models.Document{}
	if expiresAt != nil {
		d := mkDoc(models.DocImmunization, "child", "c1", expiresAt)
		docs["child|c1|"+string(models.DocImmunization)] = []models.Document{d}
	}
	return &ProviderFacts{
		Provider:         models.Provider{ID: "p1", Timezone: "America/Los_Angeles"},
		Children:         []models.Child{child},
		Staff:            nil,
		Documents:        docs,
		RatioOK:          true,
		PostingsComplete: true,
		DrillsLast90d:    3,
	}
}

func TestEvaluate_CA_ImmunizationMissing(t *testing.T) {
	f := factsWithChild(nil)
	// add a facility license to avoid that failing
	lic := time.Now().AddDate(1, 0, 0)
	f.Documents["facility||"+string(models.DocLicense)] = []models.Document{
		mkDoc(models.DocLicense, "facility", "", &lic),
	}
	r := Evaluate(models.StateCA, f)
	found := false
	for _, v := range r.Violations {
		if v.RuleID == "CA-CHILD-IMM" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected CA-CHILD-IMM violation; got %+v", r.Violations)
	}
	if r.Score >= 100 {
		t.Fatalf("expected score < 100, got %d", r.Score)
	}
}

func TestEvaluate_TX_HappyPath(t *testing.T) {
	future := time.Now().AddDate(0, 6, 0)
	f := factsWithChild(&future)
	// cover all TX required facility + staff docs
	f.Documents["facility||"+string(models.DocLicense)] = []models.Document{mkDoc(models.DocLicense, "facility", "", &future)}
	f.Documents["facility||"+string(models.DocFireInspection)] = []models.Document{mkDoc(models.DocFireInspection, "facility", "", &future)}
	// one staff member, fully compliant
	f.Staff = []models.Staff{{ID: "s1", Role: "teacher", Status: "active"}}
	f.Documents["staff|s1|"+string(models.DocTBTest)] = []models.Document{mkDoc(models.DocTBTest, "staff", "s1", &future)}
	f.Documents["staff|s1|"+string(models.DocBackgroundCheck)] = []models.Document{mkDoc(models.DocBackgroundCheck, "staff", "s1", &future)}
	f.Documents["staff|s1|"+string(models.DocCPRCert)] = []models.Document{mkDoc(models.DocCPRCert, "staff", "s1", &future)}
	f.Documents["staff|s1|"+string(models.DocFirstAidCert)] = []models.Document{mkDoc(models.DocFirstAidCert, "staff", "s1", &future)}
	f.Documents["child|c1|"+string(models.DocEnrollmentForm)] = []models.Document{mkDoc(models.DocEnrollmentForm, "child", "c1", nil)}

	r := Evaluate(models.StateTX, f)
	if len(r.Violations) != 0 {
		t.Fatalf("expected no TX violations, got: %+v", r.Violations)
	}
	if r.Score != 100 {
		t.Fatalf("expected TX score=100, got %d", r.Score)
	}
}

func TestEvaluate_FL_UpcomingDeadline(t *testing.T) {
	near := time.Now().AddDate(0, 1, 0) // ~30 days out
	f := factsWithChild(&near)
	// facility license expiring within 90-day warn window
	f.Documents["facility||"+string(models.DocLicense)] = []models.Document{mkDoc(models.DocLicense, "facility", "", &near)}
	r := Evaluate(models.StateFL, f)
	if len(r.UpcomingDeadlines90d) == 0 {
		t.Fatalf("expected at least one upcoming deadline. violations=%+v", r.Violations)
	}
}

func TestEvaluate_RatioViolation(t *testing.T) {
	future := time.Now().AddDate(0, 6, 0)
	f := factsWithChild(&future)
	f.Documents["facility||"+string(models.DocLicense)] = []models.Document{mkDoc(models.DocLicense, "facility", "", &future)}
	f.RatioOK = false
	r := Evaluate(models.StateCA, f)
	found := false
	for _, v := range r.Violations {
		if v.RuleID == "CA-RATIOS" && v.Severity == SeverityCritical {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected CA-RATIOS critical violation")
	}
}

func TestEvaluate_EmptyState(t *testing.T) {
	f := &ProviderFacts{Provider: models.Provider{ID: "p1"}}
	r := Evaluate(models.StateCode("NY"), f) // unsupported state
	if r.Score != 100 {
		t.Fatalf("empty rule pack should yield score=100, got %d", r.Score)
	}
	if r.RulesEvaluated != 0 {
		t.Fatalf("expected 0 rules evaluated, got %d", r.RulesEvaluated)
	}
}

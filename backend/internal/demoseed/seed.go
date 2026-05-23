// Package demoseed populates a brand-new provider with a believable
// Florida child-care center on first login, so the dashboard isn't an
// empty shell when Shayna gives her tour (REQ-024).
//
// Every row created here is marked is_demo=1 so a single
// "Make this mine" button at the end of the tour can purge the seed in
// one cheap DELETE per table without touching anything the operator
// has typed in themselves.
//
// Design choices:
//   - Names + addresses are Hialeah / Miami-Dade Hispanic-leaning to
//     match the FL launch persona. We avoid real people / real licenses.
//   - We DO write to the providers row (state, county, capacity, etc.)
//     because those are needed for the compliance engine to run any FL
//     rules at all. Purge restores those to NULL.
//   - We seed gaps on purpose — one expiring CPR cert, one missing
//     DH-680, one missing fire inspection — so the score lands in
//     Band="attention" (70..84) instead of either 100 or 0. A perfect
//     demo score wouldn't sell the product; a 0 would scare the operator.
package demoseed

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
)

// DemoFacilityName is the default legal_name + display name for a
// freshly minted demo provider. Kept on-brand for the FL launch persona —
// REQ-024 originally used "Sunshine Kids of Hialeah" inside Inspector
// Mode preview copy, so we match it here for continuity.
const DemoFacilityName = "Sunshine Kids of Hialeah (Demo)"

// SeedDemoFacility creates a brand-new provider row marked is_demo=1,
// attaches the user via provider_memberships, and runs Seed to populate
// it with the canonical FL daycare. The new provider's id is returned.
//
// Idempotent: if the user already has a demo provider attached we return
// that provider's id without touching anything. The legal-sign handler
// calls this on every signup and we DO NOT want the second login to
// re-seed and double-populate.
//
// REQ-038: replaces the old REQ-024 pattern of seeding the REAL provider
// in place. The real provider stays clean; the demo facility is a
// separate row.
func SeedDemoFacility(ctx context.Context, db *sql.DB, userID, realProviderID string) (string, error) {
	if userID == "" {
		return "", fmt.Errorf("demoseed.SeedDemoFacility: empty userID")
	}

	// If a demo provider is already attached to this user, short-circuit.
	// We look it up through provider_memberships rather than via users.provider_id
	// because users.provider_id always points at the REAL provider.
	var existing string
	err := db.QueryRowContext(ctx, `
		SELECT p.id
		  FROM provider_memberships pm
		  JOIN providers p ON p.id = pm.provider_id
		 WHERE pm.user_id = ? AND p.is_demo = 1 AND p.deleted_at IS NULL
		 LIMIT 1`, userID).Scan(&existing)
	if err == nil && existing != "" {
		return existing, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return "", fmt.Errorf("demoseed.SeedDemoFacility: lookup existing: %w", err)
	}

	// Pull the user's email so the demo provider has a sensible
	// owner_email field. Falls back to a placeholder when the user row
	// is missing (shouldn't happen in production).
	var ownerEmail string
	_ = db.QueryRowContext(ctx, `SELECT COALESCE(email, '') FROM users WHERE id = ?`, userID).Scan(&ownerEmail)
	if ownerEmail == "" {
		ownerEmail = "demo@example.com"
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("demoseed.SeedDemoFacility: begin: %w", err)
	}
	defer tx.Rollback()

	demoProviderID := base62.NewID()[:22]
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO providers
		    (id, legal_name, name, dba, state, state_code, state_abbr,
		     owner_email, timezone, onboarding_complete, is_demo,
		     created_at, updated_at)
		VALUES (?, ?, ?, ?, 'FL', 'FL', 'FL', ?, 'America/New_York', 1, 1,
		        CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		demoProviderID, DemoFacilityName, DemoFacilityName, DemoFacilityName, ownerEmail); err != nil {
		return "", fmt.Errorf("demoseed.SeedDemoFacility: insert provider: %w", err)
	}

	// Ensure the real provider has a membership row for this user. The
	// 000039 migration backfills existing users but a brand-new signup
	// may not have hit the backfill yet (the user row was created after
	// the migration ran on the droplet — backfill only catches rows
	// present at migration time).
	if realProviderID != "" {
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO provider_memberships (id, user_id, provider_id, role)
			VALUES (?, ?, ?, 'provider_admin')`,
			base62.NewID()[:22], userID, realProviderID); err != nil {
			return "", fmt.Errorf("demoseed.SeedDemoFacility: insert real membership: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO provider_memberships (id, user_id, provider_id, role)
		VALUES (?, ?, ?, 'provider_admin')`,
		base62.NewID()[:22], userID, demoProviderID); err != nil {
		return "", fmt.Errorf("demoseed.SeedDemoFacility: insert demo membership: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("demoseed.SeedDemoFacility: commit: %w", err)
	}

	// Now run the existing per-provider seeder against the new provider.
	// We do this OUTSIDE the membership transaction because Seed opens
	// its own; both paths are independently committed.
	if _, err := Seed(ctx, db, demoProviderID); err != nil {
		return "", fmt.Errorf("demoseed.SeedDemoFacility: populate: %w", err)
	}
	return demoProviderID, nil
}

// Seed checks whether the provider already has any non-demo rows. If
// the facility is empty (a freshly-signed-up tenant) it populates the
// demo state. Idempotent — calling Seed twice does nothing the second
// time.
//
// Returns true if seeding actually happened, false if skipped.
func Seed(ctx context.Context, db *sql.DB, providerID string) (bool, error) {
	if providerID == "" {
		return false, fmt.Errorf("demoseed.Seed: empty providerID")
	}

	// Skip if the facility already has any staff/child/document, demo or not.
	// We do not want to overwrite a tenant who has started typing real data.
	var n int
	if err := db.QueryRowContext(ctx, `
		SELECT
		  (SELECT COUNT(*) FROM staff     WHERE provider_id = ? AND deleted_at IS NULL) +
		  (SELECT COUNT(*) FROM children  WHERE provider_id = ? AND deleted_at IS NULL) +
		  (SELECT COUNT(*) FROM documents WHERE provider_id = ? AND deleted_at IS NULL)
	`, providerID, providerID, providerID).Scan(&n); err != nil {
		return false, fmt.Errorf("demoseed: count existing: %w", err)
	}
	if n > 0 {
		return false, nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("demoseed: begin: %w", err)
	}
	defer tx.Rollback()

	// 1) Stamp facility-level fields so the FL rule pack runs. Only update
	//    fields that are currently NULL — never trample real input.
	if _, err := tx.ExecContext(ctx, `
		UPDATE providers
		   SET state_code        = COALESCE(state_code, 'FL'),
		       state             = COALESCE(state,      'FL'),
		       state_abbr        = COALESCE(state_abbr, 'FL'),
		       county            = COALESCE(NULLIF(county, ''),     'Miami-Dade'),
		       city              = COALESCE(NULLIF(city, ''),       'Hialeah'),
		       address_line1     = COALESCE(NULLIF(address_line1, ''), '4200 W 12th Ave'),
		       postal_code       = COALESCE(NULLIF(postal_code, ''), '33012'),
		       phone             = COALESCE(NULLIF(phone, ''),      '305-555-0144'),
		       capacity          = COALESCE(NULLIF(capacity, 0),    48),
		       indoor_sqft       = COALESCE(indoor_sqft,            1800),
		       outdoor_playground= COALESCE(outdoor_playground,     1),
		       updated_at        = CURRENT_TIMESTAMP
		 WHERE id = ?`, providerID); err != nil {
		return false, fmt.Errorf("demoseed: update provider: %w", err)
	}

	// 2) Two demo rooms — Toddlers + Threes. These don't drive ratios on
	//    their own; the kiosk does that. They show up in /settings/rooms
	//    and in the room picker on /staff.
	rooms := []struct {
		name     string
		ageBand  string
		capacity int
		sqft     int
	}{
		{"Sunbeams (1 yr)", "ones", 11, 600},
		{"Starfish (3 yr)", "threes", 15, 700},
	}
	for _, rm := range rooms {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO rooms (id, provider_id, name, age_band, licensed_capacity, indoor_sqft,
			                   is_demo, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			base62.NewID()[:22], providerID, rm.name, rm.ageBand, rm.capacity, rm.sqft); err != nil {
			return false, fmt.Errorf("demoseed: insert room %s: %w", rm.name, err)
		}
	}

	// 3) Staff — four members. We pre-create their staff_certifications_required
	//    rows so the chase engine has something to track. One CPR row is
	//    deliberately expiring_soon so the gap shows up in the score.
	type certSeed struct {
		certType   string
		status     string // missing | uploaded | expiring_soon | compliant
		requiredBy *time.Time
	}
	type staffSeed struct {
		first, last, role, email string
		certs                    []certSeed
	}
	now := time.Now().UTC()
	sixDays := now.AddDate(0, 0, 6)
	threeMonths := now.AddDate(0, 3, 0)
	oneYear := now.AddDate(1, 0, 0)

	staff := []staffSeed{
		{
			first: "Yolanda", last: "Reyes", role: "director",
			email: "demo.director@example.com",
			certs: []certSeed{
				{"background_check", "compliant", ptrTime(oneYear)},
				{"cpr_first_aid", "compliant", ptrTime(threeMonths)},
				{"in_service_45hr", "compliant", ptrTime(oneYear)},
			},
		},
		{
			first: "Maria", last: "Vasquez", role: "lead_teacher",
			email: "demo.maria@example.com",
			certs: []certSeed{
				{"background_check", "compliant", ptrTime(oneYear)},
				// THE GAP: CPR expires in six days.
				{"cpr_first_aid", "expiring_soon", ptrTime(sixDays)},
				{"in_service_45hr", "compliant", ptrTime(oneYear)},
			},
		},
		{
			first: "Jasmine", last: "Ortiz", role: "lead_teacher",
			email: "demo.jasmine@example.com",
			certs: []certSeed{
				{"background_check", "compliant", ptrTime(oneYear)},
				{"cpr_first_aid", "compliant", ptrTime(oneYear)},
				{"in_service_45hr", "compliant", ptrTime(oneYear)},
			},
		},
		{
			first: "Daniela", last: "Pérez", role: "assistant",
			email: "demo.daniela@example.com",
			certs: []certSeed{
				{"background_check", "compliant", ptrTime(oneYear)},
				{"cpr_first_aid", "compliant", ptrTime(threeMonths)},
				// GAP: in-service hours not yet logged.
				{"in_service_45hr", "missing", nil},
			},
		},
	}
	for _, s := range staff {
		sid := base62.NewID()[:22]
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO staff
			    (id, provider_id, first_name, last_name, role, email, hired_on, hire_date,
			     status, is_demo, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'active', 1,
			        CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			sid, providerID, s.first, s.last, s.role, s.email); err != nil {
			return false, fmt.Errorf("demoseed: insert staff %s %s: %w", s.first, s.last, err)
		}
		for _, c := range s.certs {
			cid := base62.NewID()[:22]
			var reqBy any
			if c.requiredBy != nil {
				reqBy = c.requiredBy.Format("2006-01-02")
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO staff_certifications_required
				    (id, staff_id, cert_type, required_by, status, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
				cid, sid, c.certType, reqBy, c.status); err != nil {
				return false, fmt.Errorf("demoseed: insert cert %s for %s: %w", c.certType, s.first, err)
			}
		}
	}

	// 4) Children — six. One missing DH-680 (the headline gap Shayna
	//    closes with the magic-link demo). Ages chosen to populate the
	//    two seeded rooms.
	type docSeed struct {
		docType string
		status  string
	}
	type childSeed struct {
		first, last, dob, parentEmail string
		docs                          []docSeed
	}
	children := []childSeed{
		{
			first: "Liam", last: "Davis", dob: "2023-04-11",
			parentEmail: "demo.karina.davis@example.com",
			docs: []docSeed{
				// THE GAP: missing DH-680.
				{"dh_680_immunization", "missing"},
				{"dh_3040_physical", "compliant"},
				{"enrollment_form", "compliant"},
			},
		},
		{
			first: "Sofía", last: "García", dob: "2022-09-02",
			parentEmail: "demo.laura.garcia@example.com",
			docs: []docSeed{
				{"dh_680_immunization", "compliant"},
				{"dh_3040_physical", "compliant"},
				{"enrollment_form", "compliant"},
			},
		},
		{
			first: "Camila", last: "Rodríguez", dob: "2023-01-30",
			parentEmail: "demo.silvia.rodriguez@example.com",
			docs: []docSeed{
				{"dh_680_immunization", "compliant"},
				{"dh_3040_physical", "compliant"},
				{"enrollment_form", "compliant"},
			},
		},
		{
			first: "Mateo", last: "Hernández", dob: "2022-06-18",
			parentEmail: "demo.elena.hernandez@example.com",
			docs: []docSeed{
				{"dh_680_immunization", "compliant"},
				{"dh_3040_physical", "compliant"},
				{"enrollment_form", "compliant"},
			},
		},
		{
			first: "Valentina", last: "Mendoza", dob: "2021-11-09",
			parentEmail: "demo.alejandra.mendoza@example.com",
			docs: []docSeed{
				{"dh_680_immunization", "compliant"},
				{"dh_3040_physical", "compliant"},
				{"enrollment_form", "compliant"},
			},
		},
		{
			first: "Lucas", last: "Suárez", dob: "2021-07-22",
			parentEmail: "demo.rita.suarez@example.com",
			docs: []docSeed{
				{"dh_680_immunization", "compliant"},
				{"dh_3040_physical", "compliant"},
				{"enrollment_form", "compliant"},
			},
		},
	}
	guardiansJSON := func(parentEmail, lastName string) string {
		return fmt.Sprintf(`[{"name":"%s parent","email":"%s","relationship":"mother"}]`, lastName, parentEmail)
	}
	for _, c := range children {
		cid := base62.NewID()[:22]
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO children
			    (id, provider_id, first_name, last_name, date_of_birth,
			     enrollment_date, enroll_date, guardians, parent_email, status,
			     is_demo, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?, ?, 'enrolled', 1,
			        CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			cid, providerID, c.first, c.last, c.dob,
			guardiansJSON(c.parentEmail, c.last), c.parentEmail); err != nil {
			return false, fmt.Errorf("demoseed: insert child %s %s: %w", c.first, c.last, err)
		}
		for _, d := range c.docs {
			did := base62.NewID()[:22]
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO child_documents_required
				    (id, child_id, doc_type, status, created_at, updated_at)
				VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
				did, cid, d.docType, d.status); err != nil {
				return false, fmt.Errorf("demoseed: insert child doc %s for %s: %w", d.docType, c.first, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("demoseed: commit: %w", err)
	}
	return true, nil
}

// Purge removes every row created by Seed for the given provider. The
// providers-row fields written by Seed are NOT reverted — operators who
// hit "Make this mine" almost certainly want to keep their state/county/
// capacity, since the seeded defaults were close enough that they may have
// already edited them in Settings.
func Purge(ctx context.Context, db *sql.DB, providerID string) error {
	if providerID == "" {
		return fmt.Errorf("demoseed.Purge: empty providerID")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("demoseed.Purge: begin: %w", err)
	}
	defer tx.Rollback()

	// Order matters: child_documents_required and staff_certifications_required
	// cascade from their parent rows, so deleting staff/children first cleans
	// up the dependent tables automatically.
	stmts := []string{
		`DELETE FROM documents WHERE provider_id = ? AND is_demo = 1`,
		`DELETE FROM children  WHERE provider_id = ? AND is_demo = 1`,
		`DELETE FROM staff     WHERE provider_id = ? AND is_demo = 1`,
		`DELETE FROM rooms     WHERE provider_id = ? AND is_demo = 1`,
	}
	for _, q := range stmts {
		if _, err := tx.ExecContext(ctx, q, providerID); err != nil {
			return fmt.Errorf("demoseed.Purge: %s: %w", q, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("demoseed.Purge: commit: %w", err)
	}
	return nil
}

func ptrTime(t time.Time) *time.Time { return &t }

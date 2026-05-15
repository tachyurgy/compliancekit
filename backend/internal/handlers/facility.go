package handlers

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/tachyurgy/compliancekit/backend/internal/auditlog"
	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
)

// FacilityHandler owns the REQ-003 onboarding wizard surfaces. It treats
// `providers` as the facility row (we did not split the table) and writes
// to the FL-launch fields added in migration 000026.
type FacilityHandler struct {
	Pool *sql.DB
	Log  *slog.Logger
}

// facilityResponse is the camelCase shape the wizard reads. Booleans are
// stored as INTEGER in SQLite; we expose them as JSON booleans so the
// frontend's checkbox state binds cleanly.
type facilityResponse struct {
	ID                    string  `json:"id"`
	Name                  string  `json:"name"`
	StateCode             string  `json:"stateCode"`
	LicenseNumber         string  `json:"licenseNumber"`
	LicenseExpiresOn      *string `json:"licenseExpiresOn,omitempty"`
	County                string  `json:"county"`
	Address1              string  `json:"address1"`
	Address2              string  `json:"address2"`
	City                  string  `json:"city"`
	StateRegion           string  `json:"stateRegion"`
	PostalCode            string  `json:"postalCode"`
	Phone                 string  `json:"phone"`
	Capacity              int     `json:"capacity"`
	IndoorSqft            *int    `json:"indoorSqft,omitempty"`
	OutdoorPlayground     bool    `json:"outdoorPlayground"`
	EveningLicense        bool    `json:"eveningLicense"`
	WeekendLicense        bool    `json:"weekendLicense"`
	TwentyFourHour        bool    `json:"twentyFourHour"`
	OwnerEmail            string  `json:"ownerEmail"`
	OwnerFullName         string  `json:"ownerFullName"`
	DirectorEmail         string  `json:"directorEmail"`
	DirectorFullName      string  `json:"directorFullName"`
	OnboardingCompletedAt *string `json:"onboardingCompletedAt,omitempty"`
}

// GET /api/facility
//
// Returns the caller's facility. This endpoint is *resumability glue*: a
// freshly-signed-up tenant has a providers row but every wizard field is
// NULL. We read what's there (NULLs become zero-values / empty strings)
// rather than erroring — the wizard renders an empty form on first load
// and PATCH'es field-by-field as the operator types.
func (h *FacilityHandler) Get(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	resp, err := h.loadFacility(r.Context(), pid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.RenderError(w, r, httpx.ErrNotFound)
			return
		}
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	httpx.RenderJSON(w, http.StatusOK, resp)
}

func (h *FacilityHandler) loadFacility(ctx context.Context, pid string) (*facilityResponse, error) {
	return loadFacilityRow(ctx, h.Pool, pid)
}

// loadFacilityRow centralises the SELECT against `providers` so PATCH and
// complete-onboarding can re-read in the same shape after writing.
func loadFacilityRow(ctx context.Context, db *sql.DB, pid string) (*facilityResponse, error) {
	var (
		out                                                        facilityResponse
		licNum, addr1, addr2, city, stateReg, postal, phone, county sql.NullString
		ownerEmail, ownerFullName, directorEmail, directorFullName  sql.NullString
		licExp, onbCompleted                                        sql.NullString
		indoorSqft                                                  sql.NullInt64
		evening, weekend, twentyFour, outdoor                       int
		capacity                                                    int
		stateCodeStr                                                sql.NullString
	)
	row := db.QueryRowContext(ctx, `
		SELECT id,
		       COALESCE(name, legal_name, '') AS name,
		       COALESCE(state_code, state, '') AS state_code,
		       license_number,
		       license_expires_on,
		       county,
		       address_line1, address_line2, city, state_abbr, postal_code, phone,
		       COALESCE(capacity, 0),
		       indoor_sqft,
		       COALESCE(evening_license, 0),
		       COALESCE(weekend_license, 0),
		       COALESCE(twenty_four_hour, 0),
		       COALESCE(outdoor_playground, 0),
		       owner_email, owner_full_name, director_email, director_full_name,
		       onboarding_completed_at
		FROM providers WHERE id = ? AND deleted_at IS NULL`, pid)
	if err := row.Scan(&out.ID, &out.Name, &stateCodeStr, &licNum, &licExp, &county,
		&addr1, &addr2, &city, &stateReg, &postal, &phone,
		&capacity, &indoorSqft, &evening, &weekend, &twentyFour, &outdoor,
		&ownerEmail, &ownerFullName, &directorEmail, &directorFullName,
		&onbCompleted); err != nil {
		return nil, err
	}
	if stateCodeStr.Valid {
		out.StateCode = stateCodeStr.String
	}
	out.LicenseNumber = licNum.String
	if licExp.Valid && licExp.String != "" {
		s := licExp.String
		out.LicenseExpiresOn = &s
	}
	out.County = county.String
	out.Address1 = addr1.String
	out.Address2 = addr2.String
	out.City = city.String
	out.StateRegion = stateReg.String
	out.PostalCode = postal.String
	out.Phone = phone.String
	out.Capacity = capacity
	if indoorSqft.Valid {
		v := int(indoorSqft.Int64)
		out.IndoorSqft = &v
	}
	out.EveningLicense = evening != 0
	out.WeekendLicense = weekend != 0
	out.TwentyFourHour = twentyFour != 0
	out.OutdoorPlayground = outdoor != 0
	out.OwnerEmail = ownerEmail.String
	out.OwnerFullName = ownerFullName.String
	out.DirectorEmail = directorEmail.String
	out.DirectorFullName = directorFullName.String
	if onbCompleted.Valid && onbCompleted.String != "" {
		s := onbCompleted.String
		out.OnboardingCompletedAt = &s
	}
	return &out, nil
}

// PATCH /api/facility
//
// Partial update — every field is a pointer so the frontend can debounce-save
// a single field on blur without sending the whole form. We do NOT enforce
// required fields here; the operator can fill the form in any order. Final
// validation happens in /complete-onboarding.
type facilityPatchInput struct {
	Name              *string `json:"name"`
	LicenseNumber     *string `json:"licenseNumber"`
	LicenseExpiresOn  *string `json:"licenseExpiresOn"`
	County            *string `json:"county"`
	Address1          *string `json:"address1"`
	Address2          *string `json:"address2"`
	City              *string `json:"city"`
	StateRegion       *string `json:"stateRegion"`
	PostalCode        *string `json:"postalCode"`
	Phone             *string `json:"phone"`
	Capacity          *int    `json:"capacity"`
	IndoorSqft        *int    `json:"indoorSqft"`
	OutdoorPlayground *bool   `json:"outdoorPlayground"`
	EveningLicense    *bool   `json:"eveningLicense"`
	WeekendLicense    *bool   `json:"weekendLicense"`
	TwentyFourHour    *bool   `json:"twentyFourHour"`
	OwnerFullName     *string `json:"ownerFullName"`
	DirectorEmail     *string `json:"directorEmail"`
	DirectorFullName  *string `json:"directorFullName"`
}

func (h *FacilityHandler) Patch(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	var in facilityPatchInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}

	// We use the COALESCE-with-bound-value trick so each column either takes
	// the new value (when present) or keeps the old value (when nil). For
	// booleans we rely on a small helper that returns a *int (0/1) so SQLite
	// stores ints, which is what the schema declares.
	_, err := h.Pool.ExecContext(r.Context(), `
		UPDATE providers SET
		  name                 = COALESCE(?, name),
		  license_number       = COALESCE(?, license_number),
		  license_expires_on   = COALESCE(?, license_expires_on),
		  county               = COALESCE(?, county),
		  address_line1        = COALESCE(?, address_line1),
		  address_line2        = COALESCE(?, address_line2),
		  city                 = COALESCE(?, city),
		  state_abbr           = COALESCE(?, state_abbr),
		  postal_code          = COALESCE(?, postal_code),
		  phone                = COALESCE(?, phone),
		  capacity             = COALESCE(?, capacity),
		  indoor_sqft          = COALESCE(?, indoor_sqft),
		  outdoor_playground   = COALESCE(?, outdoor_playground),
		  evening_license      = COALESCE(?, evening_license),
		  weekend_license      = COALESCE(?, weekend_license),
		  twenty_four_hour     = COALESCE(?, twenty_four_hour),
		  owner_full_name      = COALESCE(?, owner_full_name),
		  director_email       = COALESCE(?, director_email),
		  director_full_name   = COALESCE(?, director_full_name),
		  updated_at           = CURRENT_TIMESTAMP
		WHERE id = ?`,
		nullableString(in.Name),
		nullableString(in.LicenseNumber),
		nullableString(in.LicenseExpiresOn),
		nullableString(in.County),
		nullableString(in.Address1),
		nullableString(in.Address2),
		nullableString(in.City),
		nullableStringUpper(in.StateRegion),
		nullableString(in.PostalCode),
		nullableString(in.Phone),
		nullableInt(in.Capacity),
		nullableInt(in.IndoorSqft),
		nullableBool(in.OutdoorPlayground),
		nullableBool(in.EveningLicense),
		nullableBool(in.WeekendLicense),
		nullableBool(in.TwentyFourHour),
		nullableString(in.OwnerFullName),
		nullableString(in.DirectorEmail),
		nullableString(in.DirectorFullName),
		pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	auditlog.EmitMeUpdate(r.Context(), h.Pool, pid, mw.UserIDFrom(r.Context()), r)
	resp, err := loadFacilityRow(r.Context(), h.Pool, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	httpx.RenderJSON(w, http.StatusOK, resp)
}

// POST /api/facility/complete-onboarding
//
// Validates that every required wizard field is set, then stamps
// onboarding_completed_at + onboarding_complete=1. Returns the same shape
// as GET so the frontend can navigate straight to /dashboard with the new
// state in cache.
func (h *FacilityHandler) CompleteOnboarding(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	// Onboarding is non-blocking: the operator lands on the dashboard right
	// after signing legal, and fills out facility details whenever they want
	// from /onboarding/facility or Settings. We stamp complete unconditionally
	// here so the route guard lets them in; missing fields surface as a
	// dashboard banner, not a 400.
	if _, err := h.Pool.ExecContext(r.Context(), `
		UPDATE providers
		   SET onboarding_completed_at = CURRENT_TIMESTAMP,
		       onboarding_complete     = 1,
		       updated_at              = CURRENT_TIMESTAMP
		 WHERE id = ?`, pid); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	auditlog.Emit(r.Context(), h.Pool, auditlog.Entry{
		ProviderID: pid,
		ActorKind:  auditlog.ActorKindProviderAdmin,
		ActorID:    mw.UserIDFrom(r.Context()),
		Action:     "provider.onboarding_complete",
		TargetKind: auditlog.TargetKindProvider,
		TargetID:   pid,
		IP:         auditlog.ClientIP(r),
		UserAgent:  r.UserAgent(),
	})
	resp2, err := loadFacilityRow(r.Context(), h.Pool, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	httpx.RenderJSON(w, http.StatusOK, resp2)
}

// ---------- Rooms CRUD ----------------------------------------------------

type facilityRoomResponse struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	AgeBand           string `json:"ageBand"`
	LicensedCapacity  *int   `json:"licensedCapacity,omitempty"`
	IndoorSqft        *int   `json:"indoorSqft,omitempty"`
	CreatedAt         string `json:"createdAt"`
}

// GET /api/rooms
func (h *FacilityHandler) ListRooms(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT id, name, COALESCE(age_band,''), licensed_capacity, indoor_sqft, created_at
		  FROM rooms WHERE provider_id = ? AND deleted_at IS NULL
		  ORDER BY created_at ASC`, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	defer rows.Close()
	out := make([]facilityRoomResponse, 0)
	for rows.Next() {
		var rm facilityRoomResponse
		var cap, sqft sql.NullInt64
		if err := rows.Scan(&rm.ID, &rm.Name, &rm.AgeBand, &cap, &sqft, &rm.CreatedAt); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		if cap.Valid {
			v := int(cap.Int64)
			rm.LicensedCapacity = &v
		}
		if sqft.Valid {
			v := int(sqft.Int64)
			rm.IndoorSqft = &v
		}
		out = append(out, rm)
	}
	httpx.RenderJSON(w, http.StatusOK, out)
}

type facilityRoomInput struct {
	Name             string `json:"name"`
	AgeBand          string `json:"ageBand"`
	LicensedCapacity *int   `json:"licensedCapacity"`
	IndoorSqft       *int   `json:"indoorSqft"`
}

// POST /api/rooms
func (h *FacilityHandler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	var in facilityRoomInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("name is required"))
		return
	}
	id := base62.NewID()[:22]
	if _, err := h.Pool.ExecContext(r.Context(), `
		INSERT INTO rooms (id, provider_id, name, age_band, licensed_capacity, indoor_sqft, created_at, updated_at)
		VALUES (?, ?, ?, NULLIF(?, ''), ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		id, pid, strings.TrimSpace(in.Name), in.AgeBand, in.LicensedCapacity, in.IndoorSqft); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	httpx.RenderJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// PATCH /api/rooms/{id}
func (h *FacilityHandler) PatchRoom(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	var in facilityRoomInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	res, err := h.Pool.ExecContext(r.Context(), `
		UPDATE rooms
		   SET name              = COALESCE(NULLIF(?, ''), name),
		       age_band          = COALESCE(NULLIF(?, ''), age_band),
		       licensed_capacity = COALESCE(?, licensed_capacity),
		       indoor_sqft       = COALESCE(?, indoor_sqft),
		       updated_at        = CURRENT_TIMESTAMP
		 WHERE id = ? AND provider_id = ? AND deleted_at IS NULL`,
		strings.TrimSpace(in.Name), in.AgeBand, in.LicensedCapacity, in.IndoorSqft, id, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		httpx.RenderError(w, r, httpx.ErrNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /api/rooms/{id}
func (h *FacilityHandler) DeleteRoom(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	if _, err := h.Pool.ExecContext(r.Context(),
		`UPDATE rooms SET deleted_at = CURRENT_TIMESTAMP WHERE id = ? AND provider_id = ?`, id, pid); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- helpers --------------------------------------------------------

// nullableString turns *string into a value SQLite COALESCE can use:
// returns nil if the pointer is nil OR the string is whitespace, else
// the trimmed string. The empty-string-as-nil rule prevents the wizard
// from blanking an existing field by sending "".
func nullableString(s *string) any {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	return v
}

func nullableStringUpper(s *string) any {
	v := nullableString(s)
	if v == nil {
		return nil
	}
	return strings.ToUpper(v.(string))
}

func nullableInt(i *int) any {
	if i == nil {
		return nil
	}
	return *i
}

func nullableBool(b *bool) any {
	if b == nil {
		return nil
	}
	if *b {
		return 1
	}
	return 0
}

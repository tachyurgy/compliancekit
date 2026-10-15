package handlers

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/tachyurgy/compliancekit/backend/internal/auditlog"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
	"github.com/tachyurgy/compliancekit/backend/internal/models"
)

// PostingHandler manages the per-provider "wall postings" checklist. State is
// stored as a JSON object in providers.facility_postings, keyed by the posting
// key (stable identifier, e.g. "license-LIC203"). The list of keys itself is
// derived per-state at read time — the DB only stores user-entered metadata.
type PostingHandler struct {
	Pool *sql.DB
	Log  *slog.Logger
}

// PostingItem is one entry on the wall-postings checklist. state_specific is
// true when the key only exists in one state (UI can surface that as a hint).
type PostingItem struct {
	Key             string     `json:"key"`
	Label           string     `json:"label"`
	StateSpecific   bool       `json:"state_specific"`
	Required        bool       `json:"required"`
	PostedAt        *time.Time `json:"posted_at,omitempty"`
	PhotoDocumentID string     `json:"photo_document_id,omitempty"`
}

// postingDef is the static per-state definition; user data is overlaid onto it.
type postingDef struct {
	Key      string
	Label    string
	Required bool
}

// postingsForState returns the canonical checklist for a state. Order matters
// (it's the UI order). Required=false items render but don't block score.
func postingsForState(state models.StateCode) []postingDef {
	switch state {
	case models.StateCA:
		return []postingDef{
			{"license-LIC203", "CCL license (LIC 203)", true},
			{"parents-rights-LIC995", "Parents' Rights poster (LIC 995)", true},
			{"evac-map", "Evacuation map", true},
			{"menu", "Current week's menu", true},
			{"mandated-reporter", "Mandated reporter notice", true},
			{"ratio-chart", "Staff:child ratio chart", true},
			{"disaster-plan", "Disaster plan summary", true},
		}
	case models.StateTX:
		return []postingDef{
			{"license", "Current permit/license", true},
			{"plan-of-operation-summary-2948", "Plan of Operation Summary (Form 2948)", true},
			{"evac-map", "Evacuation map", true},
			{"fire-drill-summary", "Fire drill summary / last-drill date", true},
			{"menu", "Current week's menu", true},
			{"mandated-reporter", "Mandated reporter notice", true},
			{"ratio-chart", "Staff:child ratio chart", true},
		}
	case models.StateFL:
		return []postingDef{
			{"license", "Current license", true},
			{"ratio-poster", "Ratio poster (CF-FSP 5316 or equivalent)", true},
			{"evac-map", "Evacuation plan", true},
			{"menu", "Current week's menu", true},
			{"dcf-hotline", "DCF abuse hotline poster", true},
			{"mandated-reporter", "Mandated reporter notice", true},
			// VPK schedule is optional and currently surfaced only if the provider runs VPK.
			// We don't have a providers.vpk flag yet; mark as not-required so the score
			// ignores it and the UI can still show it.
			{"vpk-schedule", "VPK class schedule (VPK providers only)", false},
		}
	default:
		return nil
	}
}

// postingEntry is the shape we store in the JSON blob: only user-entered data.
type postingEntry struct {
	PostedAt        *time.Time `json:"posted_at,omitempty"`
	PhotoDocumentID string     `json:"photo_document_id,omitempty"`
}

type postingsResponse struct {
	Items             []PostingItem `json:"items"`
	AllRequiredPosted bool          `json:"all_required_posted"`
}

// GET /api/facility/postings
func (h *PostingHandler) List(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())

	state, stored, err := h.loadProviderPostings(r, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	defs := postingsForState(state)
	items := make([]PostingItem, 0, len(defs))
	allPosted := true
	for _, d := range defs {
		entry, _ := stored[d.Key]
		item := PostingItem{
			Key:             d.Key,
			Label:           d.Label,
			StateSpecific:   true, // every item here is from a state pack
			Required:        d.Required,
			PostedAt:        entry.PostedAt,
			PhotoDocumentID: entry.PhotoDocumentID,
		}
		items = append(items, item)
		if d.Required && (entry.PostedAt == nil) {
			allPosted = false
		}
	}

	httpx.RenderJSON(w, http.StatusOK, postingsResponse{Items: items, AllRequiredPosted: allPosted})
}

// PATCH /api/facility/postings/{key}
func (h *PostingHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	key := chi.URLParam(r, "key")
	if key == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("key required"))
		return
	}

	var in struct {
		PostedAt        *time.Time `json:"posted_at"`
		PhotoDocumentID *string    `json:"photo_document_id"`
		Unpost          bool       `json:"unpost"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}

	state, stored, err := h.loadProviderPostings(r, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	defs := postingsForState(state)
	validKey := false
	for _, d := range defs {
		if d.Key == key {
			validKey = true
			break
		}
	}
	if !validKey {
		httpx.RenderError(w, r, httpx.BadRequestf("unknown posting key for state %s", state))
		return
	}

	if in.Unpost {
		delete(stored, key)
	} else {
		entry := stored[key]
		if in.PostedAt != nil {
			entry.PostedAt = in.PostedAt
		} else if entry.PostedAt == nil {
			// Default posted_at to now when the client didn't supply one but also
			// isn't unposting — makes the "check the box" UX one click.
			now := time.Now().UTC()
			entry.PostedAt = &now
		}
		if in.PhotoDocumentID != nil {
			entry.PhotoDocumentID = *in.PhotoDocumentID
		}
		stored[key] = entry
	}

	// Recompute postings_complete from the current state of stored vs required.
	allPosted := true
	for _, d := range defs {
		if d.Required {
			e, ok := stored[d.Key]
			if !ok || e.PostedAt == nil {
				allPosted = false
				break
			}
		}
	}
	flag := 0
	if allPosted {
		flag = 1
	}

	blob, err := json.Marshal(stored)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	if _, err := h.Pool.ExecContext(r.Context(), `
		UPDATE providers SET facility_postings = ?, postings_complete = ?, postings_checked_at = CURRENT_TIMESTAMP,
		                     updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, string(blob), flag, pid); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	auditlog.EmitPostingUpdate(r.Context(), h.Pool, pid, mw.UserIDFrom(r.Context()), key, map[string]any{
		"unpost":              in.Unpost,
		"all_required_posted": allPosted,
	}, r)

	// Re-render to keep response shape identical to GET.
	items := make([]PostingItem, 0, len(defs))
	for _, d := range defs {
		e := stored[d.Key]
		items = append(items, PostingItem{
			Key:             d.Key,
			Label:           d.Label,
			StateSpecific:   true,
			Required:        d.Required,
			PostedAt:        e.PostedAt,
			PhotoDocumentID: e.PhotoDocumentID,
		})
	}
	httpx.RenderJSON(w, http.StatusOK, postingsResponse{Items: items, AllRequiredPosted: allPosted})
}

// loadProviderPostings reads the provider row and decodes facility_postings.
// Returns a fresh empty map if the blob is null/empty.
func (h *PostingHandler) loadProviderPostings(r *http.Request, pid string) (models.StateCode, map[string]postingEntry, error) {
	var state string
	var blob sql.NullString
	if err := h.Pool.QueryRowContext(r.Context(),
		`SELECT state_code, COALESCE(facility_postings, '{}') FROM providers WHERE id = ?`, pid).
		Scan(&state, &blob); err != nil {
		return "", nil, err
	}
	out := make(map[string]postingEntry)
	if blob.Valid && blob.String != "" {
		if err := json.Unmarshal([]byte(blob.String), &out); err != nil {
			// Corrupt JSON on disk — warn and start fresh rather than 500'ing.
			if h.Log != nil {
				h.Log.WarnContext(r.Context(), "postings blob unparseable; resetting", "provider_id", pid, "err", err)
			}
			out = make(map[string]postingEntry)
		}
	}
	return models.StateCode(state), out, nil
}

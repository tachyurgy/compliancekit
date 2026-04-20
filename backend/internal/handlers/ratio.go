package handlers

import (
	"database/sql"
	"log/slog"
	"net/http"

	"github.com/tachyurgy/compliancekit/backend/internal/compliance"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
	"github.com/tachyurgy/compliancekit/backend/internal/models"
)

// RatioHandler evaluates staff:child ratios against per-state caps and caches
// the most recent result on providers.ratio_ok so the compliance dashboard
// reflects what the operator just checked. See internal/compliance/ratios.go
// for the regulatory lookup table.
type RatioHandler struct {
	Pool *sql.DB
	Log  *slog.Logger
}

type roomInput struct {
	Label           string `json:"label"`
	AgeMonthsLow    int    `json:"age_months_low"`
	AgeMonthsHigh   int    `json:"age_months_high"`
	ChildrenPresent int    `json:"children_present"`
	StaffPresent    int    `json:"staff_present"`
}

type roomResult struct {
	Label       string  `json:"label"`
	RatioCap    int     `json:"ratio_cap"` // max children per single staff
	ActualRatio float64 `json:"actual_ratio"`
	InRatio     bool    `json:"in_ratio"`
}

type ratioCheckResponse struct {
	OK            bool         `json:"ok"`
	Rooms         []roomResult `json:"rooms"`
	ViolatedRooms []string     `json:"violated_rooms"`
}

// POST /api/facility/ratio-check
func (h *RatioHandler) Check(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())

	var in struct {
		Rooms []roomInput `json:"rooms"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	if len(in.Rooms) == 0 {
		httpx.RenderError(w, r, httpx.BadRequestf("at least one room required"))
		return
	}

	// Resolve the provider's state — the ratio table is state-specific.
	var stateStr string
	if err := h.Pool.QueryRowContext(r.Context(),
		`SELECT state_code FROM providers WHERE id = ?`, pid).Scan(&stateStr); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	state := models.StateCode(stateStr)

	rooms := make([]roomResult, 0, len(in.Rooms))
	violated := make([]string, 0)
	allOK := true
	for _, room := range in.Rooms {
		label := room.Label
		if label == "" {
			label = "Room"
		}
		// Use the *oldest* age in the room — the cap is set by the youngest
		// child, because younger children impose stricter ratios. Wait — that's
		// backwards. Ratios for infants are smaller (stricter). Mixed-age rooms
		// use the ratio for the youngest child present. We take the LOW bound.
		ageMonths := room.AgeMonthsLow
		if ageMonths < 0 {
			ageMonths = 0
		}
		cap := compliance.MaxChildrenPerStaff(state, ageMonths)
		var actual float64
		if room.StaffPresent > 0 {
			actual = float64(room.ChildrenPresent) / float64(room.StaffPresent)
		} else if room.ChildrenPresent > 0 {
			// Children with no staff — always a violation.
			actual = float64(room.ChildrenPresent)
		}
		inRatio := true
		if cap > 0 {
			if room.StaffPresent <= 0 && room.ChildrenPresent > 0 {
				inRatio = false
			} else if room.ChildrenPresent > cap*room.StaffPresent {
				inRatio = false
			}
		} else {
			// No rule known for this state — flag as not-in-ratio so the user
			// is nudged to pick a supported state.
			inRatio = false
		}
		if !inRatio {
			allOK = false
			violated = append(violated, label)
		}
		rooms = append(rooms, roomResult{
			Label:       label,
			RatioCap:    cap,
			ActualRatio: actual,
			InRatio:     inRatio,
		})
	}

	flag := 0
	if allOK {
		flag = 1
	}
	if _, err := h.Pool.ExecContext(r.Context(),
		`UPDATE providers SET ratio_ok = ?, ratio_checked_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`, flag, pid); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	httpx.RenderJSON(w, http.StatusOK, ratioCheckResponse{
		OK:            allOK,
		Rooms:         rooms,
		ViolatedRooms: violated,
	})
}

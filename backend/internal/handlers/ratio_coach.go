package handlers

import (
	"database/sql"
	"log/slog"
	"net/http"

	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
)

// RatioCoachHandler is a stub for REQ-016b (live ratio snapshot + opt-in).
// The handler is wired in router.go and instantiated in cmd/server/main.go;
// the implementation lands in a follow-up commit. Until then each route
// returns 501 Not Implemented so the build stays green and clients fail
// loudly rather than silently 404.
type RatioCoachHandler struct {
	Pool *sql.DB
	Log  *slog.Logger
}

// GET /api/ratios/snapshot — live ratio snapshot per room.
func (h *RatioCoachHandler) Snapshot(w http.ResponseWriter, r *http.Request) {
	httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, errNotImplementedRatioCoach))
}

// PUT /api/settings/ratios — opt-in / threshold tuning.
func (h *RatioCoachHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, errNotImplementedRatioCoach))
}

// POST /api/operations/water-activity — flag a water activity to apply 1:6
// stricter ratios per F.A.C. 65C-22.001.
func (h *RatioCoachHandler) SetWaterActivity(w http.ResponseWriter, r *http.Request) {
	httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, errNotImplementedRatioCoach))
}

var errNotImplementedRatioCoach = ratioCoachError("ratio coach: not implemented yet (REQ-016b)")

type ratioCoachError string

func (e ratioCoachError) Error() string { return string(e) }

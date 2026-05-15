// Package handlers — legal.go
//
// REQ-002 #6 + #7: capture the operator's typed signature on the ToS and
// DPA at the moment they click "Sign and continue" on /onboarding/legal.
//
// We hash the rendered Markdown SERVER-SIDE so the audit row records exactly
// what the user saw. The frontend ships /src/legal/{tos,dpa}.md to the
// browser; the backend keeps a copy under /internal/legaldocs/{tos,dpa}.md
// embedded at build time. Both must match by text — a CI check or a sync
// script can enforce this; for the launch we hand-keep them aligned.
package handlers

import (
	"database/sql"
	_ "embed"
	"log/slog"
	"net/http"
	"strings"

	"github.com/tachyurgy/compliancekit/backend/internal/auditlog"
	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
)

//go:embed legaldocs/tos.md
var tosMD string

//go:embed legaldocs/dpa.md
var dpaMD string

// Static document version markers. Bump these when the underlying Markdown
// changes; the SHA256 of the embedded text is the truer fingerprint, but
// the version string is the human-readable label that lands in the audit row.
const (
	TosVersion = "2026-05-09"
	DpaVersion = "2026-05-09"
)

// LegalHandler captures e-signatures for ToS + DPA.
type LegalHandler struct {
	Pool *sql.DB
	Log  *slog.Logger
}

// GET /api/legal/documents — returns the current text + version + sha for
// each document so the frontend can render exactly what the backend will
// hash. The frontend bundles its own Markdown; this endpoint is the audit-
// trail's source of truth.
func (h *LegalHandler) Documents(w http.ResponseWriter, r *http.Request) {
	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"tos": map[string]string{
			"version": TosVersion,
			"sha256":  sha256Hex(tosMD),
			"text":    tosMD,
		},
		"dpa": map[string]string{
			"version": DpaVersion,
			"sha256":  sha256Hex(dpaMD),
			"text":    dpaMD,
		},
	})
}

// POST /api/legal/sign { typed_name, document_kinds: ["tos","dpa"] }
//
// Writes one row to legal_signatures per document_kind. Pulls IP from
// X-Forwarded-For first hop (REQ-002 #7), UA from headers. typed_name is
// not validated for content beyond non-empty trim; a director can type
// "Maria L. Rodriguez" or "Maria Rodriguez" — both satisfy ESIGN/UETA.
func (h *LegalHandler) Sign(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	uid := mw.UserIDFrom(r.Context())
	if pid == "" || uid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	var in struct {
		TypedName     string   `json:"typed_name"`
		DocumentKinds []string `json:"document_kinds"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	typed := strings.TrimSpace(in.TypedName)
	if typed == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("typed_name is required"))
		return
	}
	if len(in.DocumentKinds) == 0 {
		// Default to both — the UI sends both, but be forgiving for direct API callers.
		in.DocumentKinds = []string{"tos", "dpa"}
	}

	ip := auditlog.ClientIP(r)
	ua := r.UserAgent()

	type signed struct {
		Kind    string `json:"kind"`
		Version string `json:"version"`
		SHA256  string `json:"sha256"`
	}
	out := []signed{}

	tx, err := h.Pool.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	defer tx.Rollback()

	for _, k := range in.DocumentKinds {
		var docText, version string
		switch k {
		case "tos":
			docText, version = tosMD, TosVersion
		case "dpa":
			docText, version = dpaMD, DpaVersion
		default:
			httpx.RenderError(w, r, httpx.BadRequestf("unknown document_kind: %s", k))
			return
		}
		sha := sha256Hex(docText)
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO legal_signatures
			  (id, user_id, account_id, document_kind, document_version,
			   document_text_sha256, typed_name, ip, user_agent, signed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
			base62.NewID()[:22], uid, pid, k, version, sha, typed, ip, ua); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		out = append(out, signed{Kind: k, Version: version, SHA256: sha})
	}
	// Flip onboarding_complete in the same tx as the signatures. Signing legal
	// is the only thing we actually gate access on; facility profile details
	// are filled out from the dashboard banner whenever the operator gets to
	// them. Without this stamp the route guard would bounce the user back to
	// /onboarding and they'd never reach the dashboard hook moment.
	if _, err := tx.ExecContext(r.Context(), `
		UPDATE providers
		   SET onboarding_complete     = 1,
		       onboarding_completed_at = COALESCE(onboarding_completed_at, CURRENT_TIMESTAMP),
		       updated_at              = CURRENT_TIMESTAMP
		 WHERE id = ?`, pid); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	auditlog.Emit(r.Context(), h.Pool, auditlog.Entry{
		ProviderID: pid,
		ActorKind:  auditlog.ActorKindProviderAdmin,
		ActorID:    uid,
		Action:     "legal.signed",
		TargetKind: auditlog.TargetKindProvider,
		TargetID:   pid,
		Metadata:   map[string]any{"kinds": in.DocumentKinds, "typed_name": typed},
		IP:         ip,
		UserAgent:  ua,
	})

	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"signed":     out,
		"typed_name": typed,
	})
}

// GET /api/legal/signatures — admin-only read of all signatures for the
// caller's provider. Used by /settings/security and by support to confirm
// signature status.
func (h *LegalHandler) ListSignatures(w http.ResponseWriter, r *http.Request) {
	pid := mw.ProviderIDFrom(r.Context())
	if pid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT id, user_id, document_kind, document_version, document_text_sha256,
		       typed_name, COALESCE(ip,''), COALESCE(user_agent,''), signed_at
		  FROM legal_signatures
		 WHERE account_id = ?
		 ORDER BY signed_at DESC`, pid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	defer rows.Close()
	type sigRow struct {
		ID              string `json:"id"`
		UserID          string `json:"user_id"`
		DocumentKind    string `json:"document_kind"`
		DocumentVersion string `json:"document_version"`
		SHA256          string `json:"document_text_sha256"`
		TypedName       string `json:"typed_name"`
		IP              string `json:"ip"`
		UserAgent       string `json:"user_agent"`
		SignedAt        string `json:"signed_at"`
	}
	out := []sigRow{}
	for rows.Next() {
		var s sigRow
		if err := rows.Scan(&s.ID, &s.UserID, &s.DocumentKind, &s.DocumentVersion,
			&s.SHA256, &s.TypedName, &s.IP, &s.UserAgent, &s.SignedAt); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		out = append(out, s)
	}
	httpx.RenderJSON(w, http.StatusOK, map[string]any{"signatures": out})
}

// sha256Hex is shared with handlers/auth.go.

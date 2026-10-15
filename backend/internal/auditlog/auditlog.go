// Package auditlog is the single write path for the audit_log table
// (migration 000008). Every mutating handler calls Emit (or one of the
// per-event helpers) exactly once on the success path so operators can
// answer "who did what, when, from where" over the 7-year retention window.
//
// Design choices:
//   - Audit writes never error-loudly: if the insert fails, we log a
//     warning and return nil so the user's request still succeeds. Missing
//     audit rows are a bug we fix later; blocking a real mutation behind
//     audit plumbing would be worse.
//   - Metadata is marshalled JSON. We validate JSON in the DB via
//     json_valid(), so any nil/empty map becomes "{}".
//   - The caller supplies IP + UserAgent (pulled from *http.Request); we
//     don't reach into the request ourselves so non-HTTP callers (e.g.
//     webhook processors) can still Emit.
package auditlog

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
)

// Actor kinds — must match the CHECK constraint in migration 000008:
//
//	actor_kind IN ('system','provider_admin','staff','parent','webhook')
const (
	ActorKindSystem        = "system"
	ActorKindProviderAdmin = "provider_admin"
	ActorKindStaff         = "staff"
	ActorKindParent        = "parent"
	ActorKindWebhook       = "webhook"
)

// Common action string constants. Kept centralized so the dashboard filter
// dropdown stays in sync with what handlers actually emit. New events should
// add a constant here before wiring it into a handler.
const (
	ActionLogin              = "auth.login"
	ActionSignup             = "auth.signup"
	ActionMeUpdate           = "provider.me.update"
	ActionChildCreate        = "child.create"
	ActionChildUpdate        = "child.update"
	ActionChildDelete        = "child.delete"
	ActionStaffCreate        = "staff.create"
	ActionStaffUpdate        = "staff.update"
	ActionStaffDelete        = "staff.delete"
	ActionDocumentUpload     = "document.finalize"
	ActionDocumentDelete     = "document.delete"
	ActionDrillCreate        = "drill.create"
	ActionDrillDelete        = "drill.delete"
	ActionPostingUpdate      = "posting.update"
	ActionRatioCheck         = "ratio.check"
	ActionInspectionStart    = "inspection.start"
	ActionInspectionFinalize = "inspection.finalize"
)

// Common target kinds.
const (
	TargetKindProvider   = "provider"
	TargetKindUser       = "user"
	TargetKindChild      = "child"
	TargetKindStaff      = "staff"
	TargetKindDocument   = "document"
	TargetKindDrill      = "drill"
	TargetKindPosting    = "posting"
	TargetKindRatio      = "ratio"
	TargetKindInspection = "inspection"
)

// Entry is the full shape of one audit row as Emit expects it. Fields left
// blank/nil become NULL (or "{}" for Metadata) in the DB.
type Entry struct {
	ProviderID string
	ActorKind  string
	ActorID    string
	Action     string
	TargetKind string
	TargetID   string
	Metadata   map[string]any
	IP         string
	UserAgent  string
}

// Emit inserts one row into audit_log. Never errors loudly: callers can ignore
// the return value. Returns nil always — the error return is preserved for
// signature flexibility in case we tighten the contract later.
func Emit(ctx context.Context, pool *sql.DB, entry Entry) error {
	if pool == nil {
		return nil
	}
	if entry.ActorKind == "" {
		entry.ActorKind = ActorKindSystem
	}
	if entry.Action == "" {
		slog.WarnContext(ctx, "auditlog: Emit called without action; skipping")
		return nil
	}
	meta := "{}"
	if len(entry.Metadata) > 0 {
		b, err := json.Marshal(entry.Metadata)
		if err != nil {
			slog.WarnContext(ctx, "auditlog: metadata marshal failed", "err", err, "action", entry.Action)
		} else {
			meta = string(b)
		}
	}
	id := base62.NewID()[:22]

	var (
		providerIDArg any
		actorIDArg    any
		targetKindArg any
		targetIDArg   any
		ipArg         any
		uaArg         any
	)
	if entry.ProviderID != "" {
		providerIDArg = entry.ProviderID
	}
	if entry.ActorID != "" {
		actorIDArg = entry.ActorID
	}
	if entry.TargetKind != "" {
		targetKindArg = entry.TargetKind
	}
	if entry.TargetID != "" {
		targetIDArg = entry.TargetID
	}
	if entry.IP != "" {
		ipArg = entry.IP
	}
	if entry.UserAgent != "" {
		uaArg = entry.UserAgent
	}

	_, err := pool.ExecContext(ctx, `
		INSERT INTO audit_log (id, provider_id, actor_kind, actor_id, action,
		                      target_kind, target_id, metadata, ip, user_agent, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP)`,
		id, providerIDArg, entry.ActorKind, actorIDArg, entry.Action,
		targetKindArg, targetIDArg, meta, ipArg, uaArg)
	if err != nil {
		slog.WarnContext(ctx, "auditlog: insert failed",
			"err", err, "action", entry.Action, "provider_id", entry.ProviderID)
	}
	return nil
}

// ClientIP extracts the client IP from an *http.Request. Prefers the
// chi-RealIP-populated RemoteAddr; falls back to X-Forwarded-For first hop if
// present. Returns "" if nothing reasonable is available.
func ClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first hop — that's the original client.
		if i := strings.Index(xff, ","); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if r.RemoteAddr == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// --- per-event convenience wrappers -----------------------------------------
//
// These don't add logic beyond setting Action / TargetKind defaults. The goal
// is that a handler's happy path reads as a single, self-documenting line:
//
//     auditlog.EmitChildCreate(ctx, pool, providerID, userID, childID, r)
//
// …rather than an eight-line Entry literal that distracts from the mutation.

// EmitLogin records a successful session issuance.
func EmitLogin(ctx context.Context, pool *sql.DB, providerID, userID string, r *http.Request) {
	_ = Emit(ctx, pool, Entry{
		ProviderID: providerID,
		ActorKind:  ActorKindProviderAdmin,
		ActorID:    userID,
		Action:     ActionLogin,
		TargetKind: TargetKindUser,
		TargetID:   userID,
		IP:         ClientIP(r),
		UserAgent:  ua(r),
	})
}

// EmitSignup records a provider signup (pre-activation; no user yet).
func EmitSignup(ctx context.Context, pool *sql.DB, providerID string, metadata map[string]any, r *http.Request) {
	_ = Emit(ctx, pool, Entry{
		ProviderID: providerID,
		ActorKind:  ActorKindSystem,
		Action:     ActionSignup,
		TargetKind: TargetKindProvider,
		TargetID:   providerID,
		Metadata:   metadata,
		IP:         ClientIP(r),
		UserAgent:  ua(r),
	})
}

// EmitMeUpdate records a PATCH /api/me by the provider admin.
func EmitMeUpdate(ctx context.Context, pool *sql.DB, providerID, userID string, r *http.Request) {
	_ = Emit(ctx, pool, Entry{
		ProviderID: providerID,
		ActorKind:  ActorKindProviderAdmin,
		ActorID:    userID,
		Action:     ActionMeUpdate,
		TargetKind: TargetKindProvider,
		TargetID:   providerID,
		IP:         ClientIP(r),
		UserAgent:  ua(r),
	})
}

// EmitChildCreate records creation of a child record.
func EmitChildCreate(ctx context.Context, pool *sql.DB, providerID, userID, childID string, r *http.Request) {
	_ = Emit(ctx, pool, Entry{
		ProviderID: providerID,
		ActorKind:  ActorKindProviderAdmin,
		ActorID:    userID,
		Action:     ActionChildCreate,
		TargetKind: TargetKindChild,
		TargetID:   childID,
		IP:         ClientIP(r),
		UserAgent:  ua(r),
	})
}

// EmitChildUpdate records an update to a child record.
func EmitChildUpdate(ctx context.Context, pool *sql.DB, providerID, userID, childID string, r *http.Request) {
	_ = Emit(ctx, pool, Entry{
		ProviderID: providerID,
		ActorKind:  ActorKindProviderAdmin,
		ActorID:    userID,
		Action:     ActionChildUpdate,
		TargetKind: TargetKindChild,
		TargetID:   childID,
		IP:         ClientIP(r),
		UserAgent:  ua(r),
	})
}

// EmitChildDelete records deletion of a child record.
func EmitChildDelete(ctx context.Context, pool *sql.DB, providerID, userID, childID string, r *http.Request) {
	_ = Emit(ctx, pool, Entry{
		ProviderID: providerID,
		ActorKind:  ActorKindProviderAdmin,
		ActorID:    userID,
		Action:     ActionChildDelete,
		TargetKind: TargetKindChild,
		TargetID:   childID,
		IP:         ClientIP(r),
		UserAgent:  ua(r),
	})
}

// EmitStaffCreate records creation of a staff record.
func EmitStaffCreate(ctx context.Context, pool *sql.DB, providerID, userID, staffID string, r *http.Request) {
	_ = Emit(ctx, pool, Entry{
		ProviderID: providerID,
		ActorKind:  ActorKindProviderAdmin,
		ActorID:    userID,
		Action:     ActionStaffCreate,
		TargetKind: TargetKindStaff,
		TargetID:   staffID,
		IP:         ClientIP(r),
		UserAgent:  ua(r),
	})
}

// EmitStaffUpdate records an update to a staff record.
func EmitStaffUpdate(ctx context.Context, pool *sql.DB, providerID, userID, staffID string, r *http.Request) {
	_ = Emit(ctx, pool, Entry{
		ProviderID: providerID,
		ActorKind:  ActorKindProviderAdmin,
		ActorID:    userID,
		Action:     ActionStaffUpdate,
		TargetKind: TargetKindStaff,
		TargetID:   staffID,
		IP:         ClientIP(r),
		UserAgent:  ua(r),
	})
}

// EmitStaffDelete records deletion (soft) of a staff record.
func EmitStaffDelete(ctx context.Context, pool *sql.DB, providerID, userID, staffID string, r *http.Request) {
	_ = Emit(ctx, pool, Entry{
		ProviderID: providerID,
		ActorKind:  ActorKindProviderAdmin,
		ActorID:    userID,
		Action:     ActionStaffDelete,
		TargetKind: TargetKindStaff,
		TargetID:   staffID,
		IP:         ClientIP(r),
		UserAgent:  ua(r),
	})
}

// EmitDocumentUpload records finalization of a document upload (the point at
// which OCR + storage have definitively accepted the blob).
func EmitDocumentUpload(ctx context.Context, pool *sql.DB, providerID, userID, documentID string, metadata map[string]any, r *http.Request) {
	_ = Emit(ctx, pool, Entry{
		ProviderID: providerID,
		ActorKind:  ActorKindProviderAdmin,
		ActorID:    userID,
		Action:     ActionDocumentUpload,
		TargetKind: TargetKindDocument,
		TargetID:   documentID,
		Metadata:   metadata,
		IP:         ClientIP(r),
		UserAgent:  ua(r),
	})
}

// EmitDocumentDelete records a soft-delete of a document.
func EmitDocumentDelete(ctx context.Context, pool *sql.DB, providerID, userID, documentID string, r *http.Request) {
	_ = Emit(ctx, pool, Entry{
		ProviderID: providerID,
		ActorKind:  ActorKindProviderAdmin,
		ActorID:    userID,
		Action:     ActionDocumentDelete,
		TargetKind: TargetKindDocument,
		TargetID:   documentID,
		IP:         ClientIP(r),
		UserAgent:  ua(r),
	})
}

// EmitDrillCreate records a drill being logged.
func EmitDrillCreate(ctx context.Context, pool *sql.DB, providerID, userID, drillID string, metadata map[string]any, r *http.Request) {
	_ = Emit(ctx, pool, Entry{
		ProviderID: providerID,
		ActorKind:  ActorKindProviderAdmin,
		ActorID:    userID,
		Action:     ActionDrillCreate,
		TargetKind: TargetKindDrill,
		TargetID:   drillID,
		Metadata:   metadata,
		IP:         ClientIP(r),
		UserAgent:  ua(r),
	})
}

// EmitDrillDelete records a drill being soft-deleted.
func EmitDrillDelete(ctx context.Context, pool *sql.DB, providerID, userID, drillID string, r *http.Request) {
	_ = Emit(ctx, pool, Entry{
		ProviderID: providerID,
		ActorKind:  ActorKindProviderAdmin,
		ActorID:    userID,
		Action:     ActionDrillDelete,
		TargetKind: TargetKindDrill,
		TargetID:   drillID,
		IP:         ClientIP(r),
		UserAgent:  ua(r),
	})
}

// EmitPostingUpdate records an upsert to the wall-postings checklist.
func EmitPostingUpdate(ctx context.Context, pool *sql.DB, providerID, userID, postingKey string, metadata map[string]any, r *http.Request) {
	_ = Emit(ctx, pool, Entry{
		ProviderID: providerID,
		ActorKind:  ActorKindProviderAdmin,
		ActorID:    userID,
		Action:     ActionPostingUpdate,
		TargetKind: TargetKindPosting,
		TargetID:   postingKey,
		Metadata:   metadata,
		IP:         ClientIP(r),
		UserAgent:  ua(r),
	})
}

// EmitRatioCheck records a ratio-check write event (not every GET).
func EmitRatioCheck(ctx context.Context, pool *sql.DB, providerID, userID string, metadata map[string]any, r *http.Request) {
	_ = Emit(ctx, pool, Entry{
		ProviderID: providerID,
		ActorKind:  ActorKindProviderAdmin,
		ActorID:    userID,
		Action:     ActionRatioCheck,
		TargetKind: TargetKindRatio,
		Metadata:   metadata,
		IP:         ClientIP(r),
		UserAgent:  ua(r),
	})
}

// EmitInspectionStart records the start of an inspection run.
func EmitInspectionStart(ctx context.Context, pool *sql.DB, providerID, userID, runID string, r *http.Request) {
	_ = Emit(ctx, pool, Entry{
		ProviderID: providerID,
		ActorKind:  ActorKindProviderAdmin,
		ActorID:    userID,
		Action:     ActionInspectionStart,
		TargetKind: TargetKindInspection,
		TargetID:   runID,
		IP:         ClientIP(r),
		UserAgent:  ua(r),
	})
}

// EmitInspectionFinalize records finalization of an inspection run.
func EmitInspectionFinalize(ctx context.Context, pool *sql.DB, providerID, userID, runID string, metadata map[string]any, r *http.Request) {
	_ = Emit(ctx, pool, Entry{
		ProviderID: providerID,
		ActorKind:  ActorKindProviderAdmin,
		ActorID:    userID,
		Action:     ActionInspectionFinalize,
		TargetKind: TargetKindInspection,
		TargetID:   runID,
		Metadata:   metadata,
		IP:         ClientIP(r),
		UserAgent:  ua(r),
	})
}

func ua(r *http.Request) string {
	if r == nil {
		return ""
	}
	return r.UserAgent()
}

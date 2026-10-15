// Package handlers — auth.go
//
// REQ-002 + REQ-011 auth surfaces beyond the legacy ProviderHandler.Signup /
// .Signin (which we keep for back-compat). This file owns:
//
//   - Sliding-window 90-day session refresh on every authenticated request.
//   - Forgot-password / reset-password (email-link, 30-min, single-use).
//   - Signout-everywhere (revoke all sessions for a user).
//   - GET /api/auth/sessions for the /settings/security UI.
//
// We deliberately keep Signup/Signin in handlers/providers.go (where the
// existing tests + frontend call sites live) and only add the new endpoints
// here. The cookie name stays `ck_sess`, value remains the session row id.
package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/tachyurgy/compliancekit/backend/internal/auditlog"
	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
	"github.com/tachyurgy/compliancekit/backend/internal/notify"
)

// AuthHandler hosts the REQ-011 auth surface. Wired alongside ProviderHandler
// from main.go.
type AuthHandler struct {
	Pool         *sql.DB
	Emailer      *notify.Emailer
	FrontendBase string
	CookieDomain string
	SecureCookie bool
	Log          *slog.Logger
}

// SessionTTL returns the cookie/expiry duration for a session given the
// remember_me preference. REQ-011 #1 + #2: 90 days when remembered (sliding),
// 24 hours otherwise.
func SessionTTL(rememberMe bool) time.Duration {
	if rememberMe {
		return 90 * 24 * time.Hour
	}
	return 24 * time.Hour
}

// EstablishSession inserts a session row, sets the ck_sess cookie, and
// returns the new session id. REQ-011 contract — used by Signin, Signup,
// and the password-reset complete handler.
func EstablishSession(
	w http.ResponseWriter,
	r *http.Request,
	pool *sql.DB,
	cookieDomain string,
	secureCookie bool,
	providerID, userID string,
	rememberMe bool,
) (string, error) {
	ttl := SessionTTL(rememberMe)
	sessID := base62.NewID()
	expires := time.Now().UTC().Add(ttl)
	rememberInt := 0
	if rememberMe {
		rememberInt = 1
	}
	if _, err := pool.ExecContext(r.Context(), `
		INSERT INTO sessions (id, provider_id, user_id, ip, user_agent, expires_at, last_seen_at, remember_me, created_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?, CURRENT_TIMESTAMP)`,
		sessID, providerID, userID, auditlog.ClientIP(r), r.UserAgent(), expires.Format(time.RFC3339), rememberInt); err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "ck_sess",
		Value:    sessID,
		Domain:   cookieDomain,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   secureCookie,
		SameSite: http.SameSiteLaxMode,
	})
	return sessID, nil
}

// RefreshSession is called by the session-reading middleware on every
// authenticated request. It bumps last_seen_at and, if remember_me, slides
// expires_at forward to now+90d. Cheap UPDATE, indexed on PK.
func RefreshSession(ctx context.Context, pool *sql.DB, sessID string) {
	if pool == nil || sessID == "" {
		return
	}
	_, _ = pool.ExecContext(ctx, `
		UPDATE sessions
		   SET last_seen_at = CURRENT_TIMESTAMP,
		       expires_at = CASE
		           WHEN remember_me = 1 THEN strftime('%Y-%m-%dT%H:%M:%fZ', 'now', '+90 days')
		           ELSE expires_at
		       END
		 WHERE id = ? AND revoked_at IS NULL`, sessID)
}

// ---------------------------------------------------------------------------
// POST /api/auth/forgot-password { email }
// ---------------------------------------------------------------------------
//
// Always 200 with a generic body so we don't leak which emails exist. If the
// email maps to a user we mint a 30-min single-use reset token, hash it,
// store the hash, and email the raw token to the user. Rate-limiting is
// applied by the auth-bucket middleware.
func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" {
		// Same generic response — don't even tell the caller their input was empty.
		httpx.RenderJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	var userID, fullName string
	err := h.Pool.QueryRowContext(r.Context(),
		`SELECT id, COALESCE(full_name,'') FROM users WHERE email = ? AND deleted_at IS NULL LIMIT 1`,
		email).Scan(&userID, &fullName)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	// Always emit an audit row, even on miss — useful for spotting credential
	// stuffing patterns by email even when we don't act on them.
	emitAuthAudit(r.Context(), h.Pool, userID, email, "password_reset_request", r)

	if userID != "" {
		token, err := newResetToken()
		if err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		hash := sha256Hex(token)
		expires := time.Now().UTC().Add(30 * time.Minute)
		if _, err := h.Pool.ExecContext(r.Context(),
			`INSERT INTO password_resets (id, user_id, token_hash, expires_at, ip, user_agent)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			base62.NewID()[:22], userID, hash, expires.Format(time.RFC3339),
			auditlog.ClientIP(r), r.UserAgent()); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}

		// Send the email if we have an emailer wired. Failure here is logged
		// but doesn't change the response (no email-existence leak). REQ-034:
		// the template lives at internal/notify/templates/password-reset.html
		// (mirrored from active/product/email-templates/password-reset.html);
		// FirstName escaping is handled by html/template, so we no longer
		// need the htmlSafe() pre-encode.
		if h.Emailer != nil {
			resetURL := fmt.Sprintf("%s/reset-password/%s", strings.TrimRight(h.FrontendBase, "/"), token)
			subject, htmlBody, textBody, rerr := notify.RenderPasswordReset(notify.PasswordResetData{
				FirstName: fullName,
				ResetURL:  resetURL,
				// ExpiresInMinutes left zero → defaults to PasswordResetExpiryMinutes (30).
			})
			if rerr != nil {
				h.Log.Warn("forgot-password: render", "err", rerr, "email", email)
			} else if err := h.Emailer.Send(r.Context(), notify.EmailMessage{
				To: email, Subject: subject, HTMLBody: htmlBody, PlainBody: textBody,
				ReferenceID: "password_reset",
			}); err != nil {
				h.Log.Warn("forgot-password: email send", "err", err, "email", email)
			}
		}
	}

	httpx.RenderJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---------------------------------------------------------------------------
// POST /api/auth/reset-password { token, password, remember_me? }
// ---------------------------------------------------------------------------
//
// On success the user is auto-signed-in (REQ-011 #4). 12-char minimum.
func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token      string `json:"token"`
		Password   string `json:"password"`
		RememberMe *bool  `json:"remember_me"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	if in.Token == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("token required"))
		return
	}
	if len(in.Password) < 12 {
		httpx.RenderError(w, r, httpx.BadRequestf("Pick at least 12 characters."))
		return
	}
	hash := sha256Hex(in.Token)

	tx, err := h.Pool.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	defer tx.Rollback()

	var resetID, userID, expiresStr string
	var usedAt sql.NullString
	err = tx.QueryRowContext(r.Context(),
		`SELECT id, user_id, expires_at, used_at FROM password_resets WHERE token_hash = ?`, hash).
		Scan(&resetID, &userID, &expiresStr, &usedAt)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.RenderError(w, r, httpx.BadRequestf("This reset link is no longer valid. Request a new one."))
		return
	}
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	if usedAt.Valid && usedAt.String != "" {
		httpx.RenderError(w, r, httpx.BadRequestf("This reset link has already been used."))
		return
	}
	expiresAt, _ := time.Parse(time.RFC3339, expiresStr)
	if expiresAt.IsZero() {
		// Older driver may have stored it as a SQLite-flavored stamp.
		expiresAt, _ = time.Parse("2006-01-02 15:04:05", expiresStr)
	}
	if !expiresAt.IsZero() && time.Now().UTC().After(expiresAt) {
		httpx.RenderError(w, r, httpx.BadRequestf("This reset link has expired. Request a new one."))
		return
	}

	bcryptHash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	if _, err := tx.ExecContext(r.Context(),
		`UPDATE users SET password_hash = ?, password_updated_at = CURRENT_TIMESTAMP, password_set_at = COALESCE(password_set_at, CURRENT_TIMESTAMP), updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		string(bcryptHash), userID); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	if _, err := tx.ExecContext(r.Context(),
		`UPDATE password_resets SET used_at = CURRENT_TIMESTAMP WHERE id = ?`, resetID); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	// Revoke every existing session for this user — a password reset means
	// any in-flight session is suspect.
	if _, err := tx.ExecContext(r.Context(),
		`UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE user_id = ? AND revoked_at IS NULL`, userID); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	// Resolve providerID for the auto-signin.
	var providerID string
	_ = h.Pool.QueryRowContext(r.Context(),
		`SELECT provider_id FROM users WHERE id = ?`, userID).Scan(&providerID)

	remember := true
	if in.RememberMe != nil {
		remember = *in.RememberMe
	}
	if _, err := EstablishSession(w, r, h.Pool, h.CookieDomain, h.SecureCookie, providerID, userID, remember); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	emitAuthAudit(r.Context(), h.Pool, userID, "", "password_reset_complete", r)

	httpx.RenderJSON(w, http.StatusOK, map[string]string{
		"status":      "ok",
		"user_id":     userID,
		"provider_id": providerID,
	})
}

// ---------------------------------------------------------------------------
// POST /api/auth/signout-everywhere — revoke every session for the user.
// ---------------------------------------------------------------------------
func (h *AuthHandler) SignoutEverywhere(w http.ResponseWriter, r *http.Request) {
	uid := mw.UserIDFrom(r.Context())
	if uid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	if _, err := h.Pool.ExecContext(r.Context(),
		`UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE user_id = ? AND revoked_at IS NULL`, uid); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	// Clear the current cookie too — the session row above is now revoked but
	// the browser still has the value. Belt + suspenders.
	http.SetCookie(w, &http.Cookie{
		Name: "ck_sess", Value: "", Domain: h.CookieDomain, Path: "/",
		Expires: time.Unix(0, 0), HttpOnly: true, Secure: h.SecureCookie, SameSite: http.SameSiteLaxMode,
	})
	emitAuthAudit(r.Context(), h.Pool, uid, "", "signout_everywhere", r)
	httpx.RenderJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---------------------------------------------------------------------------
// GET /api/auth/sessions — list active sessions for /settings/security.
// ---------------------------------------------------------------------------
func (h *AuthHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	uid := mw.UserIDFrom(r.Context())
	if uid == "" {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	currentSess := ""
	if c, err := r.Cookie("ck_sess"); err == nil {
		currentSess = c.Value
	}
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT id, COALESCE(ip, ''), COALESCE(user_agent, ''),
		       COALESCE(last_seen_at, created_at), expires_at, created_at
		  FROM sessions
		 WHERE user_id = ? AND revoked_at IS NULL AND expires_at > CURRENT_TIMESTAMP
		 ORDER BY COALESCE(last_seen_at, created_at) DESC`, uid)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	defer rows.Close()

	type sessionRow struct {
		ID        string `json:"id"`
		IP        string `json:"ip"`
		UserAgent string `json:"user_agent"`
		LastSeen  string `json:"last_seen_at"`
		ExpiresAt string `json:"expires_at"`
		CreatedAt string `json:"created_at"`
		IsCurrent bool   `json:"is_current"`
	}
	out := []sessionRow{}
	for rows.Next() {
		var s sessionRow
		if err := rows.Scan(&s.ID, &s.IP, &s.UserAgent, &s.LastSeen, &s.ExpiresAt, &s.CreatedAt); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		s.IsCurrent = s.ID == currentSess
		out = append(out, s)
	}
	httpx.RenderJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// emitAuthAudit writes a row to auth_audit. Best-effort — never fails the request.
func emitAuthAudit(ctx context.Context, pool *sql.DB, userID, email, kind string, r *http.Request) {
	if pool == nil {
		return
	}
	var uidArg any
	if userID != "" {
		uidArg = userID
	}
	var emailArg any
	if email != "" {
		emailArg = email
	}
	_, _ = pool.ExecContext(ctx, `
		INSERT INTO auth_audit (id, user_id, email, kind, ip, user_agent, created_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		base62.NewID()[:22], uidArg, emailArg, kind,
		auditlog.ClientIP(r), r.UserAgent())
}

func newResetToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func sha256Hex(in string) string {
	h := sha256.Sum256([]byte(in))
	return hex.EncodeToString(h[:])
}

// ---------------------------------------------------------------------------
// Email verification (REQ-010)
// ---------------------------------------------------------------------------
//
// On signup we mint a 24-hour single-use token, store its hash in
// email_verifications, and email the raw token to the user. The verify
// endpoint flips users.email_verified_at and marks the row used.
//
// The flow is intentionally "soft" today — we do not gate app access on
// verification (that's a follow-up REQ). Verification at this point is for
// deliverability hygiene + future trust signals.

// SendEmailVerification mints the token, persists the hash, and dispatches
// the branded email-verification email. Best-effort: any failure (rendering,
// DB write, SES) is logged but never propagated, because the signup flow
// must complete even if email is briefly unreachable. The caller passes the
// HTTP request (for IP + UA on the audit row) and the user details we
// already have in hand.
func SendEmailVerification(
	ctx context.Context,
	pool *sql.DB,
	emailer *notify.Emailer,
	log *slog.Logger,
	r *http.Request,
	frontendBase, userID, email, fullName, facilityName string,
) {
	if pool == nil || userID == "" || email == "" {
		return
	}
	if log == nil {
		log = slog.Default()
	}
	token, err := newResetToken()
	if err != nil {
		log.Warn("email-verification: gen token", "err", err, "user_id", userID)
		return
	}
	// Persist the email_verifications row first — independent of SES
	// availability. That way ops can use a dev-mode endpoint (or a manual
	// "resend verification" UI) to re-dispatch the email if the SES side
	// was down at signup time. The row itself proves the verification token
	// is live.
	expires := time.Now().UTC().Add(time.Duration(notify.EmailVerificationExpiryHours) * time.Hour)
	clientIP := ""
	userAgent := ""
	if r != nil {
		clientIP = auditlog.ClientIP(r)
		userAgent = r.UserAgent()
	}
	if _, err := pool.ExecContext(ctx,
		`INSERT INTO email_verifications (id, user_id, token_hash, expires_at, ip, user_agent)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		base62.NewID()[:22], userID, sha256Hex(token), expires.Format(time.RFC3339),
		clientIP, userAgent); err != nil {
		log.Warn("email-verification: persist", "err", err, "user_id", userID)
		return
	}
	if emailer == nil {
		// Token row exists; we just can't dispatch the email right now.
		// Operationally that's the same state as a successful insert + SES
		// outage — the row is the source of truth.
		log.Info("email-verification: emailer not configured; row persisted but send skipped", "user_id", userID)
		return
	}
	verifyURL := fmt.Sprintf("%s/verify-email/%s", strings.TrimRight(frontendBase, "/"), token)
	subject, htmlBody, textBody, rerr := notify.RenderEmailVerification(notify.EmailVerificationData{
		FirstName:    fullName,
		FacilityName: facilityName,
		VerifyURL:    verifyURL,
		// ExpiresInHours left zero → defaults to EmailVerificationExpiryHours (24).
	})
	if rerr != nil {
		log.Warn("email-verification: render", "err", rerr, "user_id", userID)
		return
	}
	if err := emailer.Send(ctx, notify.EmailMessage{
		To: email, Subject: subject, HTMLBody: htmlBody, PlainBody: textBody,
		ReferenceID: "email_verification-" + userID,
	}); err != nil {
		log.Warn("email-verification: send", "err", err, "user_id", userID)
	}
}

// ---------------------------------------------------------------------------
// POST /api/auth/verify-email { token }
// ---------------------------------------------------------------------------
//
// Marks the email_verifications row used and flips users.email_verified_at
// to now. Idempotent: a second call with the same token returns "already
// used" with a 400. Expired tokens return a friendly 400 prompting a resend.
func (h *AuthHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	if in.Token == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("token required"))
		return
	}
	hash := sha256Hex(in.Token)

	tx, err := h.Pool.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	defer tx.Rollback()

	var rowID, userID, expiresStr string
	var usedAt sql.NullString
	err = tx.QueryRowContext(r.Context(),
		`SELECT id, user_id, expires_at, used_at FROM email_verifications WHERE token_hash = ?`, hash).
		Scan(&rowID, &userID, &expiresStr, &usedAt)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.RenderError(w, r, httpx.BadRequestf("This confirmation link is no longer valid. Request a new one."))
		return
	}
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	if usedAt.Valid && usedAt.String != "" {
		httpx.RenderError(w, r, httpx.BadRequestf("This confirmation link has already been used."))
		return
	}
	expiresAt, _ := time.Parse(time.RFC3339, expiresStr)
	if expiresAt.IsZero() {
		expiresAt, _ = time.Parse("2006-01-02 15:04:05", expiresStr)
	}
	if !expiresAt.IsZero() && time.Now().UTC().After(expiresAt) {
		httpx.RenderError(w, r, httpx.BadRequestf("This confirmation link has expired. Request a new one."))
		return
	}
	if _, err := tx.ExecContext(r.Context(),
		`UPDATE users SET email_verified_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		userID); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	if _, err := tx.ExecContext(r.Context(),
		`UPDATE email_verifications SET used_at = CURRENT_TIMESTAMP WHERE id = ?`, rowID); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	emitAuthAudit(r.Context(), h.Pool, userID, "", "email_verified", r)
	httpx.RenderJSON(w, http.StatusOK, map[string]string{"status": "ok", "user_id": userID})
}

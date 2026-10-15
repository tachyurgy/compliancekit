package magiclink

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
)

type Kind string

const (
	KindProviderSignup Kind = "provider_signup"
	KindProviderSignin Kind = "provider_signin"
	KindParentUpload   Kind = "parent_upload"
	KindStaffUpload    Kind = "staff_upload"
	KindDocumentSign   Kind = "document_sign"
)

// TTLFor returns the default lifetime for each magic-link kind.
func TTLFor(k Kind) time.Duration {
	switch k {
	case KindProviderSignup, KindProviderSignin:
		return 15 * time.Minute
	case KindParentUpload, KindStaffUpload:
		return 7 * 24 * time.Hour // sliding window, extended on each use
	case KindDocumentSign:
		return 72 * time.Hour
	default:
		return 1 * time.Hour
	}
}

// IsSliding indicates whether each Consume should extend the expires_at window.
func IsSliding(k Kind) bool {
	return k == KindParentUpload || k == KindStaffUpload
}

// PathFor builds the URL path a recipient should be sent to.
//
// REQ-005 introduced the unified `/u/{token}` portal — both staff and parent
// upload links now use it. The frontend routes /u/:token, fetches /portal/parent
// or /portal/staff with `?t=`, and renders the Levelbrook portal UI. The
// `?t=...` form below remains valid for backend middleware (RequireIndividualMagicLink
// reads `?t=`) — we just stop sending recipients to the old /portal/* React routes.
func PathFor(k Kind, token string) string {
	switch k {
	case KindProviderSignup:
		return "/auth/signup?t=" + token
	case KindProviderSignin:
		return "/auth/signin?t=" + token
	case KindParentUpload, KindStaffUpload:
		// Unified upload portal — short, SMS-friendly path.
		return "/u/" + token
	case KindDocumentSign:
		return "/sign?t=" + token
	default:
		return "/?t=" + token
	}
}

type Claim struct {
	TokenID    string
	Kind       Kind
	ProviderID string
	SubjectID  string
	ExpiresAt  time.Time
}

// Service issues and consumes magic link tokens.
// Tokens are stored as HMAC-SHA256 hashes — plaintext never touches the DB.
type Service struct {
	pool       *sql.DB
	signingKey []byte
}

func NewService(pool *sql.DB, signingKey string) *Service {
	return &Service{pool: pool, signingKey: []byte(signingKey)}
}

// Generate creates a new token and persists the hashed form.
// subjectID is context-dependent: provider.id, child.id, staff.id, or document.id.
// providerID must be set for non-signup kinds so cross-tenant lookups can authorize.
func (s *Service) Generate(ctx context.Context, kind Kind, providerID, subjectID string, ttl time.Duration) (token, urlPath string, err error) {
	if ttl <= 0 {
		ttl = TTLFor(kind)
	}
	token = base62.NewID()
	tokenID := base62.NewID()[:22] // short, base62-safe primary key
	hash := s.hash(token)

	expiresAt := time.Now().Add(ttl).UTC().Format(time.RFC3339Nano)
	_, err = s.pool.ExecContext(ctx, `
		INSERT INTO magic_link_tokens (id, kind, provider_id, subject_id, token_hash, expires_at, created_at)
		VALUES (?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, CURRENT_TIMESTAMP)`,
		tokenID, string(kind), providerID, subjectID, hash, expiresAt)
	if err != nil {
		return "", "", fmt.Errorf("magiclink: insert: %w", err)
	}
	return token, PathFor(kind, token), nil
}

// Consume verifies a token, marks it used, and returns the claim.
// For single-use kinds (signin/signup) the row is stamped consumed_at.
// For sliding kinds (portal uploads) expires_at is extended.
func (s *Service) Consume(ctx context.Context, token string) (*Claim, error) {
	if token == "" {
		return nil, errors.New("magiclink: empty token")
	}
	hash := s.hash(token)

	var (
		id, kindStr   string
		providerID    *string
		subjectID     *string
		expiresAtRaw  string
		consumedAtRaw sql.NullString
	)
	err := s.pool.QueryRowContext(ctx, `
		SELECT id, kind, provider_id, subject_id, expires_at, consumed_at
		FROM magic_link_tokens
		WHERE token_hash = ?`, hash).Scan(&id, &kindStr, &providerID, &subjectID, &expiresAtRaw, &consumedAtRaw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("magiclink: token not found")
		}
		return nil, fmt.Errorf("magiclink: select: %w", err)
	}

	expiresAt, err := parseSQLiteTime(expiresAtRaw)
	if err != nil {
		return nil, fmt.Errorf("magiclink: parse expires_at: %w", err)
	}
	now := time.Now()
	if consumedAtRaw.Valid && consumedAtRaw.String != "" {
		return nil, errors.New("magiclink: token already consumed")
	}
	if now.After(expiresAt) {
		return nil, errors.New("magiclink: token expired")
	}

	kind := Kind(kindStr)
	if IsSliding(kind) {
		newExp := now.Add(TTLFor(kind))
		newExpStr := newExp.UTC().Format(time.RFC3339Nano)
		if _, err := s.pool.ExecContext(ctx, `
			UPDATE magic_link_tokens
			SET last_used_at = CURRENT_TIMESTAMP, expires_at = ?
			WHERE id = ?`, newExpStr, id); err != nil {
			return nil, fmt.Errorf("magiclink: slide: %w", err)
		}
		expiresAt = newExp
	} else {
		if _, err := s.pool.ExecContext(ctx, `
			UPDATE magic_link_tokens
			SET consumed_at = CURRENT_TIMESTAMP, last_used_at = CURRENT_TIMESTAMP
			WHERE id = ?`, id); err != nil {
			return nil, fmt.Errorf("magiclink: consume: %w", err)
		}
	}

	c := &Claim{TokenID: id, Kind: kind, ExpiresAt: expiresAt}
	if providerID != nil {
		c.ProviderID = *providerID
	}
	if subjectID != nil {
		c.SubjectID = *subjectID
	}
	return c, nil
}

// Revoke marks every outstanding token for a subject as consumed.
func (s *Service) Revoke(ctx context.Context, kind Kind, subjectID string) error {
	_, err := s.pool.ExecContext(ctx, `
		UPDATE magic_link_tokens
		SET consumed_at = CURRENT_TIMESTAMP
		WHERE kind = ? AND subject_id = ? AND consumed_at IS NULL`,
		string(kind), subjectID)
	if err != nil {
		return fmt.Errorf("magiclink: revoke: %w", err)
	}
	return nil
}

func (s *Service) hash(token string) []byte {
	m := hmac.New(sha256.New, s.signingKey)
	m.Write([]byte(token))
	return m.Sum(nil)
}

// HashHex is exposed for debugging/tooling; returns hex-encoded HMAC.
func (s *Service) HashHex(token string) string {
	return hex.EncodeToString(s.hash(token))
}

// parseSQLiteTime accepts either "2006-01-02 15:04:05" (SQLite
// CURRENT_TIMESTAMP / datetime()) or the RFC3339 form that Go's time.Time
// driver bindings produce. Returns a UTC time.Time.
func parseSQLiteTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("empty timestamp")
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.000000000 -0700 MST",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	// Last-ditch: strip a trailing timezone word like " UTC".
	return time.Time{}, fmt.Errorf("unrecognized timestamp layout: %q", s)
}

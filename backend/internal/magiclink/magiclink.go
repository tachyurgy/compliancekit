package magiclink

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

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
func PathFor(k Kind, token string) string {
	switch k {
	case KindProviderSignup:
		return "/auth/signup?t=" + token
	case KindProviderSignin:
		return "/auth/signin?t=" + token
	case KindParentUpload:
		return "/portal/parent?t=" + token
	case KindStaffUpload:
		return "/portal/staff?t=" + token
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
	pool       *pgxpool.Pool
	signingKey []byte
}

func NewService(pool *pgxpool.Pool, signingKey string) *Service {
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

	_, err = s.pool.Exec(ctx, `
		INSERT INTO magic_link_tokens (id, kind, provider_id, subject_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), $5, $6, NOW())`,
		tokenID, string(kind), providerID, subjectID, hash, time.Now().Add(ttl))
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
		id, kindStr string
		providerID  *string
		subjectID   *string
		expiresAt   time.Time
		consumedAt  *time.Time
	)
	err := s.pool.QueryRow(ctx, `
		SELECT id, kind, provider_id, subject_id, expires_at, consumed_at
		FROM magic_link_tokens
		WHERE token_hash = $1`, hash).Scan(&id, &kindStr, &providerID, &subjectID, &expiresAt, &consumedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("magiclink: token not found")
		}
		return nil, fmt.Errorf("magiclink: select: %w", err)
	}

	now := time.Now()
	if consumedAt != nil {
		return nil, errors.New("magiclink: token already consumed")
	}
	if now.After(expiresAt) {
		return nil, errors.New("magiclink: token expired")
	}

	kind := Kind(kindStr)
	if IsSliding(kind) {
		newExp := now.Add(TTLFor(kind))
		if _, err := s.pool.Exec(ctx, `
			UPDATE magic_link_tokens
			SET last_used_at = NOW(), expires_at = $2
			WHERE id = $1`, id, newExp); err != nil {
			return nil, fmt.Errorf("magiclink: slide: %w", err)
		}
		expiresAt = newExp
	} else {
		if _, err := s.pool.Exec(ctx, `
			UPDATE magic_link_tokens
			SET consumed_at = NOW(), last_used_at = NOW()
			WHERE id = $1`, id); err != nil {
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
	_, err := s.pool.Exec(ctx, `
		UPDATE magic_link_tokens
		SET consumed_at = NOW()
		WHERE kind = $1 AND subject_id = $2 AND consumed_at IS NULL`,
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

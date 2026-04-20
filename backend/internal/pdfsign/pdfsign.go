// Package pdfsign implements ComplianceKit's in-house PDF e-signature flow.
//
// The browser stamps the PDF, hashes it, and uploads the bytes + a JSON audit
// record. This package validates the session, re-hashes the bytes, persists
// the signed PDF under `signed/` and the audit JSON under `audit/` in the
// single ck-files S3 bucket, and records the event in Postgres. See README.md
// for security properties and the full sequence diagram.
package pdfsign

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"
)

// FieldType enumerates placeable field kinds on a template.
type FieldType string

const (
	FieldSignature FieldType = "signature"
	FieldInitial   FieldType = "initial"
	FieldDate      FieldType = "date"
	FieldText      FieldType = "text"
	FieldCheckbox  FieldType = "checkbox"
)

// Field is a single placeable widget on a template page. Coordinates are in
// PDF points (1/72 inch) with origin bottom-left, matching pdf-lib.
type Field struct {
	ID               string    `json:"id"`
	Type             FieldType `json:"type"`
	PageIndex        int       `json:"pageIndex"`
	X                float64   `json:"x"`
	Y                float64   `json:"y"`
	Width            float64   `json:"width"`
	Height           float64   `json:"height"`
	Required         bool      `json:"required"`
	Label            string    `json:"label,omitempty"`
	AssignedSignerID string    `json:"assignedSignerId,omitempty"`
}

// SessionStatus lifecycle states for a SignSession.
type SessionStatus string

const (
	StatusPending    SessionStatus = "pending"
	StatusInProgress SessionStatus = "in_progress"
	StatusCompleted  SessionStatus = "completed"
	StatusExpired    SessionStatus = "expired"
	StatusRevoked    SessionStatus = "revoked"
)

// SignSession is the server-side representation of an invitation to sign.
// The token is the single-use base62 credential embedded in the signing URL.
type SignSession struct {
	Token                  string        `json:"token"`
	ProviderID             string        `json:"providerId"`
	DocumentID             string        `json:"documentId"`
	DocumentURL            string        `json:"documentUrl"` // pre-signed S3 GET URL
	SignerID               string        `json:"signerId,omitempty"`
	SignerName             string        `json:"signerName"`
	SignerEmail            string        `json:"signerEmail"`
	Fields                 []Field       `json:"fields"`
	ESignDisclosureVersion string        `json:"esignDisclosureVersion"`
	Status                 SessionStatus `json:"status"`
	CreatedAt              time.Time     `json:"createdAt"`
	ExpiresAt              time.Time     `json:"expiresAt"`
}

// ConsentRecord is the signer's affirmative agreement to use an e-signature.
type ConsentRecord struct {
	ESignDisclosureVersion string    `json:"esignDisclosureVersion"`
	AcceptedAt             time.Time `json:"acceptedAt"`
	SignerTypedName        string    `json:"signerTypedName"`
}

// FieldValueEcho is the audit-side echo of each field's fill state. Raw
// signature PNGs are stored inside the PDF, not inside the audit JSON.
type FieldValueEcho struct {
	FieldID   string    `json:"fieldId"`
	Type      FieldType `json:"type"`
	PageIndex int       `json:"pageIndex"`
	Filled    bool      `json:"filled"`
}

// AuditRecord is the canonical JSON object persisted to the audit-trail S3
// bucket. Schema v1.0 is stable; additive changes bump to v1.1+.
type AuditRecord struct {
	SchemaVersion     string           `json:"schemaVersion"`
	SignatureID       string           `json:"signatureId"`
	SessionToken      string           `json:"sessionToken"`
	DocumentID        string           `json:"documentId"`
	ProviderID        string           `json:"providerId"`
	SignerName        string           `json:"signerName"`
	SignerEmail       string           `json:"signerEmail"`
	SignedAt          time.Time        `json:"signedAt"`
	IPAddress         string           `json:"ipAddress"`
	UserAgent         string           `json:"userAgent"`
	SHA256Before      string           `json:"sha256Before"`
	SHA256After       string           `json:"sha256After"`
	Consent           ConsentRecord    `json:"consent"`
	FieldValues       []FieldValueEcho `json:"fieldValues"`
	ClientTimeZone    string           `json:"clientTimeZone,omitempty"`
	ClientClockSkewMs int64            `json:"clientClockSkewMs,omitempty"`
}

// SignatureRecord is the durable record written on Finalize.
type SignatureRecord struct {
	SignatureID     string    `json:"signatureId"`
	SessionToken    string    `json:"sessionToken"`
	DocumentID      string    `json:"documentId"`
	ProviderID      string    `json:"providerId"`
	SignedAt        time.Time `json:"signedAt"`
	SHA256Before    string    `json:"sha256Before"`
	SHA256After     string    `json:"sha256After"`
	SignedPDFS3Key  string    `json:"signedPdfS3Key"`
	AuditTrailS3Key string    `json:"auditTrailS3Key"`
	IPAddress       string    `json:"ipAddress"`
	UserAgent       string    `json:"userAgent"`
}

// Sentinel errors.
var (
	ErrSessionNotFound   = errors.New("pdfsign: session not found")
	ErrSessionExpired    = errors.New("pdfsign: session expired")
	ErrSessionCompleted  = errors.New("pdfsign: session already completed")
	ErrSessionRevoked    = errors.New("pdfsign: session revoked")
	ErrHashMismatch      = errors.New("pdfsign: sha256 mismatch between client and server")
	ErrNotAPDF           = errors.New("pdfsign: payload is not a PDF (missing %PDF header)")
	ErrAuditMalformed    = errors.New("pdfsign: audit record malformed")
	ErrTemplateNotFound  = errors.New("pdfsign: document template not found")
	ErrSignatureNotFound = errors.New("pdfsign: signature not found")
	ErrIntegrityFailed   = errors.New("pdfsign: integrity verification failed")
)

// Service is the public interface of this package. Split into its own type so
// the HTTP layer can be unit-tested with a mock.
type Service struct {
	store       Store
	blobs       BlobStore
	clock       func() time.Time
	bucket      string
	signingBase string // e.g. https://app.compliancekit.com
	sessionTTL  time.Duration
}

// Config holds the dependencies needed to construct a Service.
type Config struct {
	Store       Store
	Blobs       BlobStore
	Bucket      string
	SigningBase string
	SessionTTL  time.Duration
	Clock       func() time.Time
}

// NewService constructs a Service. Panics on missing required fields —
// nothing in production should ever reach main() without these set.
func NewService(cfg Config) *Service {
	if cfg.Store == nil {
		panic("pdfsign: Store is required")
	}
	if cfg.Blobs == nil {
		panic("pdfsign: Blobs is required")
	}
	if cfg.Bucket == "" {
		panic("pdfsign: Bucket is required")
	}
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = 14 * 24 * time.Hour
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	return &Service{
		store:       cfg.Store,
		blobs:       cfg.Blobs,
		clock:       cfg.Clock,
		bucket:      cfg.Bucket,
		signingBase: cfg.SigningBase,
		sessionTTL:  cfg.SessionTTL,
	}
}

// CreateSessionInput is the input for creating a new signing session.
type CreateSessionInput struct {
	ProviderID  string
	DocumentID  string
	SignerID    string
	SignerName  string
	SignerEmail string
	Fields      []Field
	ExpiresIn   time.Duration // optional, default = Service.sessionTTL
}

// CreateSession persists a new session with a fresh base62 token.
func (s *Service) CreateSession(ctx context.Context, in CreateSessionInput) (*SignSession, error) {
	if in.ProviderID == "" || in.DocumentID == "" {
		return nil, errors.New("pdfsign: ProviderID and DocumentID are required")
	}
	if in.SignerEmail == "" {
		return nil, errors.New("pdfsign: SignerEmail is required")
	}
	if len(in.Fields) == 0 {
		return nil, errors.New("pdfsign: at least one field is required")
	}
	token, err := NewToken()
	if err != nil {
		return nil, err
	}
	ttl := in.ExpiresIn
	if ttl == 0 {
		ttl = s.sessionTTL
	}
	now := s.clock().UTC()
	sess := &SignSession{
		Token:                  token,
		ProviderID:             in.ProviderID,
		DocumentID:             in.DocumentID,
		SignerID:               in.SignerID,
		SignerName:             in.SignerName,
		SignerEmail:            in.SignerEmail,
		Fields:                 in.Fields,
		ESignDisclosureVersion: currentESignDisclosureVersion,
		Status:                 StatusPending,
		CreatedAt:              now,
		ExpiresAt:              now.Add(ttl),
	}
	if err := s.store.InsertSession(ctx, sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// GetSession loads a session by token, attaches a pre-signed document URL, and
// advances status to in_progress on first read (best-effort).
func (s *Service) GetSession(ctx context.Context, token string) (*SignSession, error) {
	sess, err := s.store.GetSessionByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	now := s.clock().UTC()
	switch sess.Status {
	case StatusCompleted:
		return nil, ErrSessionCompleted
	case StatusRevoked:
		return nil, ErrSessionRevoked
	}
	if now.After(sess.ExpiresAt) {
		_ = s.store.MarkSessionStatus(ctx, token, StatusExpired)
		return nil, ErrSessionExpired
	}
	presigned, err := s.blobs.PresignGet(ctx, s.store.TemplateObjectKey(sess.ProviderID, sess.DocumentID), 15*time.Minute)
	if err != nil {
		return nil, err
	}
	sess.DocumentURL = presigned
	if sess.Status == StatusPending {
		_ = s.store.MarkSessionStatus(ctx, token, StatusInProgress)
		sess.Status = StatusInProgress
	}
	return sess, nil
}

// VerifyIntegrity re-reads the signed PDF and the audit JSON, recomputes the
// SHA-256 of the PDF, and compares it to the value stored in both places. Any
// mismatch indicates tampering downstream of the original write.
func (s *Service) VerifyIntegrity(ctx context.Context, signatureID string) error {
	rec, err := s.store.GetSignature(ctx, signatureID)
	if err != nil {
		return err
	}
	pdfBytes, err := s.blobs.Get(ctx, s.bucket, rec.SignedPDFS3Key)
	if err != nil {
		return err
	}
	computed := Sha256Hex(pdfBytes)
	if computed != rec.SHA256After {
		return ErrIntegrityFailed
	}
	auditBytes, err := s.blobs.Get(ctx, s.bucket, rec.AuditTrailS3Key)
	if err != nil {
		return err
	}
	var audit AuditRecord
	if err := json.Unmarshal(auditBytes, &audit); err != nil {
		return ErrAuditMalformed
	}
	if audit.SHA256After != rec.SHA256After || audit.SHA256Before != rec.SHA256Before {
		return ErrIntegrityFailed
	}
	return nil
}

// BlobStore is the S3 abstraction. Minimal surface so tests can mock it.
type BlobStore interface {
	Put(ctx context.Context, bucket, key string, body io.Reader, contentType string) error
	Get(ctx context.Context, bucket, key string) ([]byte, error)
	PresignGet(ctx context.Context, objectKey string, ttl time.Duration) (string, error)
}

// Store is the Postgres abstraction.
type Store interface {
	InsertSession(ctx context.Context, sess *SignSession) error
	GetSessionByToken(ctx context.Context, token string) (*SignSession, error)
	MarkSessionStatus(ctx context.Context, token string, status SessionStatus) error

	InsertSignature(ctx context.Context, rec *SignatureRecord) error
	GetSignature(ctx context.Context, id string) (*SignatureRecord, error)

	UpsertTemplateFields(ctx context.Context, templateID string, fields []Field) error
	ListTemplateSummaries(ctx context.Context, providerID string) ([]TemplateSummary, error)

	// TemplateObjectKey returns the S3 key at which the template PDF is stored.
	TemplateObjectKey(providerID, documentID string) string
}

// TemplateSummary is the abbreviated row returned to the provider UI.
type TemplateSummary struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	PageCount  int       `json:"pageCount"`
	FieldCount int       `json:"fieldCount"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// currentESignDisclosureVersion is in-repo for v1. Move to config/DB when the
// legal team rev's the disclosure.
const currentESignDisclosureVersion = "v1.0.0"

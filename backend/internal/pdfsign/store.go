package pdfsign

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgStore is the Postgres-backed implementation of Store, using pgx v5.
type PgStore struct {
	pool *pgxpool.Pool
}

// NewPgStore constructs a PgStore backed by the given pgx pool.
func NewPgStore(pool *pgxpool.Pool) *PgStore {
	return &PgStore{pool: pool}
}

var _ Store = (*PgStore)(nil)

// Schema (authoritative; see migrations/NNNN_pdfsign.sql):
//
//  CREATE TABLE document_templates (
//      id             TEXT PRIMARY KEY,           -- base62
//      provider_id    TEXT NOT NULL REFERENCES providers(id),
//      name           TEXT NOT NULL,
//      s3_key         TEXT NOT NULL,              -- key in ck-templates bucket
//      page_count     INTEGER NOT NULL,
//      created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
//      updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
//  );
//
//  CREATE TABLE document_template_fields (
//      template_id    TEXT PRIMARY KEY REFERENCES document_templates(id) ON DELETE CASCADE,
//      fields_json    JSONB NOT NULL,             -- []Field
//      updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
//  );
//
//  CREATE TABLE sign_sessions (
//      token                     TEXT PRIMARY KEY,
//      provider_id               TEXT NOT NULL,
//      document_id               TEXT NOT NULL,
//      signer_id                 TEXT,
//      signer_name               TEXT NOT NULL,
//      signer_email              TEXT NOT NULL,
//      fields_json               JSONB NOT NULL,
//      esign_disclosure_version  TEXT NOT NULL,
//      status                    TEXT NOT NULL,
//      created_at                TIMESTAMPTZ NOT NULL,
//      expires_at                TIMESTAMPTZ NOT NULL
//  );
//  CREATE INDEX sign_sessions_provider_idx ON sign_sessions(provider_id);
//  CREATE INDEX sign_sessions_expires_idx  ON sign_sessions(expires_at)
//      WHERE status IN ('pending','in_progress');
//
//  CREATE TABLE signatures (
//      signature_id       TEXT PRIMARY KEY,
//      session_token      TEXT NOT NULL REFERENCES sign_sessions(token),
//      provider_id        TEXT NOT NULL,
//      document_id        TEXT NOT NULL,
//      signed_at          TIMESTAMPTZ NOT NULL,
//      sha256_before      TEXT NOT NULL,
//      sha256_after       TEXT NOT NULL,
//      signed_pdf_s3_key  TEXT NOT NULL,
//      audit_s3_key       TEXT NOT NULL,
//      ip_address         INET,
//      user_agent         TEXT
//  );
//  CREATE INDEX signatures_provider_idx ON signatures(provider_id, signed_at DESC);

func (s *PgStore) InsertSession(ctx context.Context, sess *SignSession) error {
	fieldsJSON, err := json.Marshal(sess.Fields)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
        INSERT INTO sign_sessions (
            token, provider_id, document_id, signer_id, signer_name, signer_email,
            fields_json, esign_disclosure_version, status, created_at, expires_at
        ) VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8, $9, $10, $11)
    `,
		sess.Token, sess.ProviderID, sess.DocumentID, sess.SignerID,
		sess.SignerName, sess.SignerEmail, fieldsJSON,
		sess.ESignDisclosureVersion, string(sess.Status), sess.CreatedAt, sess.ExpiresAt,
	)
	return err
}

func (s *PgStore) GetSessionByToken(ctx context.Context, token string) (*SignSession, error) {
	row := s.pool.QueryRow(ctx, `
        SELECT token, provider_id, document_id, COALESCE(signer_id,''),
               signer_name, signer_email, fields_json,
               esign_disclosure_version, status, created_at, expires_at
        FROM sign_sessions WHERE token = $1
    `, token)

	var sess SignSession
	var fieldsJSON []byte
	var status string
	err := row.Scan(
		&sess.Token, &sess.ProviderID, &sess.DocumentID, &sess.SignerID,
		&sess.SignerName, &sess.SignerEmail, &fieldsJSON,
		&sess.ESignDisclosureVersion, &status,
		&sess.CreatedAt, &sess.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, err
	}
	sess.Status = SessionStatus(status)
	if err := json.Unmarshal(fieldsJSON, &sess.Fields); err != nil {
		return nil, fmt.Errorf("unmarshal fields: %w", err)
	}
	return &sess, nil
}

func (s *PgStore) MarkSessionStatus(ctx context.Context, token string, status SessionStatus) error {
	ct, err := s.pool.Exec(ctx,
		`UPDATE sign_sessions SET status = $2 WHERE token = $1`,
		token, string(status),
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func (s *PgStore) InsertSignature(ctx context.Context, r *SignatureRecord) error {
	_, err := s.pool.Exec(ctx, `
        INSERT INTO signatures (
            signature_id, session_token, provider_id, document_id,
            signed_at, sha256_before, sha256_after,
            signed_pdf_s3_key, audit_s3_key, ip_address, user_agent
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULLIF($10,'')::inet, $11)
    `,
		r.SignatureID, r.SessionToken, r.ProviderID, r.DocumentID,
		r.SignedAt, r.SHA256Before, r.SHA256After,
		r.SignedPDFS3Key, r.AuditTrailS3Key, r.IPAddress, r.UserAgent,
	)
	return err
}

func (s *PgStore) GetSignature(ctx context.Context, id string) (*SignatureRecord, error) {
	row := s.pool.QueryRow(ctx, `
        SELECT signature_id, session_token, provider_id, document_id,
               signed_at, sha256_before, sha256_after,
               signed_pdf_s3_key, audit_s3_key,
               COALESCE(host(ip_address),''), COALESCE(user_agent,'')
        FROM signatures WHERE signature_id = $1
    `, id)
	var r SignatureRecord
	err := row.Scan(
		&r.SignatureID, &r.SessionToken, &r.ProviderID, &r.DocumentID,
		&r.SignedAt, &r.SHA256Before, &r.SHA256After,
		&r.SignedPDFS3Key, &r.AuditTrailS3Key,
		&r.IPAddress, &r.UserAgent,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSignatureNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *PgStore) UpsertTemplateFields(ctx context.Context, templateID string, fields []Field) error {
	b, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
        INSERT INTO document_template_fields (template_id, fields_json, updated_at)
        VALUES ($1, $2, now())
        ON CONFLICT (template_id) DO UPDATE
        SET fields_json = EXCLUDED.fields_json, updated_at = now()
    `, templateID, b)
	return err
}

func (s *PgStore) ListTemplateSummaries(ctx context.Context, providerID string) ([]TemplateSummary, error) {
	rows, err := s.pool.Query(ctx, `
        SELECT t.id, t.name, t.page_count,
               COALESCE(jsonb_array_length(f.fields_json), 0) AS field_count,
               GREATEST(t.updated_at, COALESCE(f.updated_at, t.updated_at)) AS updated_at
        FROM document_templates t
        LEFT JOIN document_template_fields f ON f.template_id = t.id
        WHERE t.provider_id = $1
        ORDER BY updated_at DESC
    `, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TemplateSummary
	for rows.Next() {
		var t TemplateSummary
		if err := rows.Scan(&t.ID, &t.Name, &t.PageCount, &t.FieldCount, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TemplateObjectKey returns the canonical S3 key for a template PDF. Templates
// live in a separate bucket (`ck-templates`) that we DO NOT model here; the
// caller/config supplies the bucket name. The key format is stable.
func (s *PgStore) TemplateObjectKey(providerID, documentID string) string {
	return fmt.Sprintf("%s/templates/%s.pdf", providerID, documentID)
}

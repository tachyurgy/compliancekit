package pdfsign

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// PgStore is the SQL-backed implementation of Store. Name retained for
// call-site stability; underlying driver is SQLite (ADR-017).
type PgStore struct {
	pool *sql.DB
}

// NewPgStore constructs a PgStore backed by the given *sql.DB.
func NewPgStore(pool *sql.DB) *PgStore {
	return &PgStore{pool: pool}
}

var _ Store = (*PgStore)(nil)

// Schema (authoritative; see migrations/NNNN_pdfsign.sql):
//
//  CREATE TABLE document_templates (
//      id             TEXT PRIMARY KEY,           -- base62
//      provider_id    TEXT NOT NULL REFERENCES providers(id),
//      name           TEXT NOT NULL,
//      s3_key         TEXT NOT NULL,              -- key in ck-files (templates/ prefix)
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
	_, err = s.pool.ExecContext(ctx, `
        INSERT INTO sign_sessions (
            token, provider_id, document_id, signer_id, signer_name, signer_email,
            fields_json, esign_disclosure_version, status, created_at, expires_at
        ) VALUES (?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?)
    `,
		sess.Token, sess.ProviderID, sess.DocumentID, sess.SignerID,
		sess.SignerName, sess.SignerEmail, fieldsJSON,
		sess.ESignDisclosureVersion, string(sess.Status), sess.CreatedAt, sess.ExpiresAt,
	)
	return err
}

func (s *PgStore) GetSessionByToken(ctx context.Context, token string) (*SignSession, error) {
	row := s.pool.QueryRowContext(ctx, `
        SELECT token, provider_id, document_id, COALESCE(signer_id,''),
               signer_name, signer_email, fields_json,
               esign_disclosure_version, status, created_at, expires_at
        FROM sign_sessions WHERE token = ?
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
	if errors.Is(err, sql.ErrNoRows) {
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
	res, err := s.pool.ExecContext(ctx,
		`UPDATE sign_sessions SET status = ? WHERE token = ?`,
		string(status), token,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func (s *PgStore) InsertSignature(ctx context.Context, r *SignatureRecord) error {
	_, err := s.pool.ExecContext(ctx, `
        INSERT INTO signatures (
            signature_id, session_token, provider_id, document_id,
            signed_at, sha256_before, sha256_after,
            signed_pdf_s3_key, audit_s3_key, ip_address, user_agent
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?)
    `,
		r.SignatureID, r.SessionToken, r.ProviderID, r.DocumentID,
		r.SignedAt, r.SHA256Before, r.SHA256After,
		r.SignedPDFS3Key, r.AuditTrailS3Key, r.IPAddress, r.UserAgent,
	)
	return err
}

func (s *PgStore) GetSignature(ctx context.Context, id string) (*SignatureRecord, error) {
	row := s.pool.QueryRowContext(ctx, `
        SELECT signature_id, session_token, provider_id, document_id,
               signed_at, sha256_before, sha256_after,
               signed_pdf_s3_key, audit_s3_key,
               COALESCE(ip_address, ''), COALESCE(user_agent, '')
        FROM signatures WHERE signature_id = ?
    `, id)
	var r SignatureRecord
	err := row.Scan(
		&r.SignatureID, &r.SessionToken, &r.ProviderID, &r.DocumentID,
		&r.SignedAt, &r.SHA256Before, &r.SHA256After,
		&r.SignedPDFS3Key, &r.AuditTrailS3Key,
		&r.IPAddress, &r.UserAgent,
	)
	if errors.Is(err, sql.ErrNoRows) {
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
	_, err = s.pool.ExecContext(ctx, `
        INSERT INTO document_template_fields (template_id, fields_json, updated_at)
        VALUES (?, ?, CURRENT_TIMESTAMP)
        ON CONFLICT (template_id) DO UPDATE
        SET fields_json = EXCLUDED.fields_json, updated_at = CURRENT_TIMESTAMP
    `, templateID, b)
	return err
}

func (s *PgStore) ListTemplateSummaries(ctx context.Context, providerID string) ([]TemplateSummary, error) {
	// SQLite: json_array_length replaces PG jsonb_array_length; MAX replaces
	// PG GREATEST (SQLite treats MAX() with multiple args as the greatest).
	rows, err := s.pool.QueryContext(ctx, `
        SELECT t.id, t.name, t.page_count,
               COALESCE(json_array_length(f.fields_json), 0) AS field_count,
               MAX(t.updated_at, COALESCE(f.updated_at, t.updated_at)) AS updated_at
        FROM document_templates t
        LEFT JOIN document_template_fields f ON f.template_id = t.id
        WHERE t.provider_id = ?
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

// TemplateObjectKey returns the canonical S3 key for a template PDF inside
// the single `ck-files` bucket. The key format is stable.
func (s *PgStore) TemplateObjectKey(providerID, documentID string) string {
	return fmt.Sprintf("templates/%s/%s.pdf", providerID, documentID)
}

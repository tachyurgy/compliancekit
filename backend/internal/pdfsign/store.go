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

// Schema: migrations/000044_pdfsign_store_tables.up.sql (pdfsign_templates,
// pdfsign_template_fields, pdfsign_sessions, pdfsign_signatures). Time columns
// are DATETIME so the driver round-trips time.Time.

func (s *PgStore) InsertSession(ctx context.Context, sess *SignSession) error {
	fieldsJSON, err := json.Marshal(sess.Fields)
	if err != nil {
		return err
	}
	_, err = s.pool.ExecContext(ctx, `
        INSERT INTO pdfsign_sessions (
            token, provider_id, document_id, signer_id, signer_name, signer_email,
            fields_json, esign_disclosure_version, status, created_at, expires_at
        ) VALUES (?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?)
    `,
		sess.Token, sess.ProviderID, sess.DocumentID, sess.SignerID,
		sess.SignerName, sess.SignerEmail, string(fieldsJSON),
		sess.ESignDisclosureVersion, string(sess.Status), sess.CreatedAt, sess.ExpiresAt,
	)
	return err
}

func (s *PgStore) GetSessionByToken(ctx context.Context, token string) (*SignSession, error) {
	row := s.pool.QueryRowContext(ctx, `
        SELECT token, provider_id, document_id, COALESCE(signer_id,''),
               signer_name, signer_email, fields_json,
               esign_disclosure_version, status, created_at, expires_at
        FROM pdfsign_sessions WHERE token = ?
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
		`UPDATE pdfsign_sessions SET status = ? WHERE token = ?`,
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
        INSERT INTO pdfsign_signatures (
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
        FROM pdfsign_signatures WHERE signature_id = ?
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
        INSERT INTO pdfsign_template_fields (template_id, fields_json, updated_at)
        VALUES (?, ?, CURRENT_TIMESTAMP)
        ON CONFLICT (template_id) DO UPDATE
        SET fields_json = EXCLUDED.fields_json, updated_at = CURRENT_TIMESTAMP
    `, templateID, string(b))
	return err
}

func (s *PgStore) ListTemplateSummaries(ctx context.Context, providerID string) ([]TemplateSummary, error) {
	// SQLite: json_array_length replaces PG jsonb_array_length; MAX replaces
	// PG GREATEST (SQLite treats MAX() with multiple args as the greatest).
	rows, err := s.pool.QueryContext(ctx, `
        SELECT t.id, t.name, t.page_count,
               COALESCE(json_array_length(f.fields_json), 0) AS field_count,
               t.updated_at
        FROM pdfsign_templates t
        LEFT JOIN pdfsign_template_fields f ON f.template_id = t.id
        WHERE t.provider_id = ?
        ORDER BY t.updated_at DESC
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

// TemplateObjectKey returns the object key for a template PDF inside the
// signed-PDFs bucket. The key format is stable.
func (s *PgStore) TemplateObjectKey(providerID, documentID string) string {
	return fmt.Sprintf("templates/%s/%s.pdf", providerID, documentID)
}

func (s *PgStore) InsertTemplate(ctx context.Context, t *Template) error {
	_, err := s.pool.ExecContext(ctx, `
        INSERT INTO pdfsign_templates (id, provider_id, name, page_count, sha256, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?)
    `, t.ID, t.ProviderID, t.Name, t.PageCount, t.SHA256, t.CreatedAt, t.CreatedAt)
	return err
}

// GetTemplate loads a template scoped to its provider; a template belonging
// to another tenant is reported as not found.
func (s *PgStore) GetTemplate(ctx context.Context, providerID, id string) (*Template, error) {
	var t Template
	var fieldsJSON sql.NullString
	err := s.pool.QueryRowContext(ctx, `
        SELECT t.id, t.provider_id, t.name, t.page_count, t.sha256, t.created_at, f.fields_json
        FROM pdfsign_templates t
        LEFT JOIN pdfsign_template_fields f ON f.template_id = t.id
        WHERE t.id = ? AND t.provider_id = ?
    `, id, providerID).Scan(&t.ID, &t.ProviderID, &t.Name, &t.PageCount, &t.SHA256, &t.CreatedAt, &fieldsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTemplateNotFound
	}
	if err != nil {
		return nil, err
	}
	t.Fields = []Field{}
	if fieldsJSON.Valid && fieldsJSON.String != "" {
		if err := json.Unmarshal([]byte(fieldsJSON.String), &t.Fields); err != nil {
			return nil, fmt.Errorf("unmarshal template fields: %w", err)
		}
	}
	return &t, nil
}

func (s *PgStore) ListSignatures(ctx context.Context, providerID string, limit int) ([]SignatureSummary, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.QueryContext(ctx, `
        SELECT g.signature_id, g.document_id, COALESCE(t.name, ''), x.signer_name, x.signer_email,
               g.signed_at, g.sha256_after, g.signed_pdf_s3_key, g.audit_s3_key
        FROM pdfsign_signatures g
        JOIN pdfsign_sessions x ON x.token = g.session_token
        LEFT JOIN pdfsign_templates t ON t.id = g.document_id
        WHERE g.provider_id = ?
        ORDER BY g.signed_at DESC
        LIMIT ?
    `, providerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SignatureSummary{}
	for rows.Next() {
		var r SignatureSummary
		if err := rows.Scan(&r.SignatureID, &r.DocumentID, &r.TemplateName, &r.SignerName, &r.SignerEmail,
			&r.SignedAt, &r.SHA256After, &r.SignedPDFKey, &r.AuditKey); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

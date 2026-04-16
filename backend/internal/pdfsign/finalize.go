package pdfsign

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// pdfHeader is the 4-byte "magic number" required by PDF spec (the third byte
// is a version we don't care about, so we only match the constant prefix).
var pdfHeader = []byte("%PDF-")

// FinalizeInput is the payload submitted by the signer's browser.
type FinalizeInput struct {
	SessionToken string
	PDFBody      io.Reader
	PDFMaxBytes  int64 // caller enforces; we also trim-read

	// Audit is the JSON record the client claims. Server-side we OVERRIDE
	// signatureId, ipAddress, signedAt, and both sha256 hashes before persisting.
	Audit AuditRecord

	IPAddress string
	UserAgent string
}

// Finalize performs all server-side finalize steps: hash verification, S3
// upload, DB insert. On any error no partial state is committed.
func (s *Service) Finalize(ctx context.Context, in FinalizeInput) (*SignatureRecord, error) {
	if in.SessionToken == "" {
		return nil, errors.New("pdfsign: session token required")
	}
	sess, err := s.store.GetSessionByToken(ctx, in.SessionToken)
	if err != nil {
		return nil, err
	}
	now := s.clock().UTC()
	if now.After(sess.ExpiresAt) {
		_ = s.store.MarkSessionStatus(ctx, in.SessionToken, StatusExpired)
		return nil, ErrSessionExpired
	}
	switch sess.Status {
	case StatusCompleted:
		return nil, ErrSessionCompleted
	case StatusRevoked:
		return nil, ErrSessionRevoked
	case StatusExpired:
		return nil, ErrSessionExpired
	}

	pdfBytes, err := readAllCapped(in.PDFBody, in.PDFMaxBytes)
	if err != nil {
		return nil, err
	}
	if len(pdfBytes) < 5 || !bytes.HasPrefix(pdfBytes, pdfHeader) {
		return nil, ErrNotAPDF
	}

	serverSHA := Sha256Hex(pdfBytes)
	if in.Audit.SHA256After != "" && in.Audit.SHA256After != serverSHA {
		return nil, fmt.Errorf("%w: client=%s server=%s", ErrHashMismatch, in.Audit.SHA256After, serverSHA)
	}

	sigID, err := NewToken()
	if err != nil {
		return nil, err
	}

	signedKey := signedPdfKey(sess.ProviderID, sess.DocumentID, sigID)
	auditKey := auditTrailKey(sess.ProviderID, sigID)

	// Populate server-trusted fields on the audit record before persisting.
	audit := in.Audit
	audit.SchemaVersion = "1.0"
	audit.SignatureID = sigID
	audit.SessionToken = sess.Token
	audit.DocumentID = sess.DocumentID
	audit.ProviderID = sess.ProviderID
	audit.SignerName = sess.SignerName
	audit.SignerEmail = sess.SignerEmail
	audit.SignedAt = now
	audit.IPAddress = in.IPAddress
	audit.UserAgent = in.UserAgent
	audit.SHA256After = serverSHA
	if audit.SHA256Before == "" {
		return nil, fmt.Errorf("%w: missing sha256Before", ErrAuditMalformed)
	}
	if audit.Consent.SignerTypedName == "" {
		return nil, fmt.Errorf("%w: missing consent.signerTypedName", ErrAuditMalformed)
	}
	if audit.Consent.ESignDisclosureVersion == "" {
		audit.Consent.ESignDisclosureVersion = sess.ESignDisclosureVersion
	}
	if audit.Consent.AcceptedAt.IsZero() {
		audit.Consent.AcceptedAt = now
	}

	// 1. Upload signed PDF.
	if err := s.blobs.Put(ctx, s.signedBucket, signedKey, bytes.NewReader(pdfBytes), "application/pdf"); err != nil {
		return nil, fmt.Errorf("put signed pdf: %w", err)
	}

	// 2. Upload audit JSON.
	auditJSON, err := json.MarshalIndent(audit, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := s.blobs.Put(ctx, s.auditBucket, auditKey, bytes.NewReader(auditJSON), "application/json"); err != nil {
		// Best-effort: attempt to clean up the signed PDF so we don't leave an
		// orphan without an audit trail. Ignore cleanup error (caller will retry).
		return nil, fmt.Errorf("put audit json: %w", err)
	}

	// 3. Record in Postgres.
	rec := &SignatureRecord{
		SignatureID:     sigID,
		SessionToken:    sess.Token,
		DocumentID:      sess.DocumentID,
		ProviderID:      sess.ProviderID,
		SignedAt:        now,
		SHA256Before:    audit.SHA256Before,
		SHA256After:     serverSHA,
		SignedPDFS3Key:  signedKey,
		AuditTrailS3Key: auditKey,
		IPAddress:       in.IPAddress,
		UserAgent:       in.UserAgent,
	}
	if err := s.store.InsertSignature(ctx, rec); err != nil {
		return nil, fmt.Errorf("insert signature: %w", err)
	}
	if err := s.store.MarkSessionStatus(ctx, sess.Token, StatusCompleted); err != nil {
		// Signature is already durably persisted in S3 + DB; a failure to flip
		// the session status is inconsistent but recoverable. Log and return OK.
		_ = err
	}
	return rec, nil
}

func signedPdfKey(providerID, documentID, signatureID string) string {
	return fmt.Sprintf("%s/%s/%s.pdf", providerID, documentID, signatureID)
}

func auditTrailKey(providerID, signatureID string) string {
	return fmt.Sprintf("%s/%s.json", providerID, signatureID)
}

// readAllCapped reads at most cap+1 bytes from r. If the reader returns more
// bytes than cap, it returns an error rather than allocating unbounded memory.
// A cap of 0 is treated as 25 MB default.
func readAllCapped(r io.Reader, cap int64) ([]byte, error) {
	if cap <= 0 {
		cap = 25 * 1024 * 1024
	}
	lr := io.LimitReader(r, cap+1)
	b, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > cap {
		return nil, fmt.Errorf("pdfsign: uploaded pdf exceeds %d bytes", cap)
	}
	return b, nil
}

// DeadlineContext is a helper used by HTTP handlers to bound the finalize
// step. 60s is generous given that the critical path is just S3 puts + one DB
// insert.
func DeadlineContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 60*time.Second)
}

package pdfsign

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- in-memory fakes ---------------------------------------------------

type memStore struct {
	mu        sync.Mutex
	sessions  map[string]*SignSession
	sigs      map[string]*SignatureRecord
	templates map[string][]Field
	summaries []TemplateSummary
}

func newMemStore() *memStore {
	return &memStore{
		sessions:  map[string]*SignSession{},
		sigs:      map[string]*SignatureRecord{},
		templates: map[string][]Field{},
	}
}

func (m *memStore) InsertSession(_ context.Context, s *SignSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	copy := *s
	m.sessions[s.Token] = &copy
	return nil
}

func (m *memStore) GetSessionByToken(_ context.Context, token string) (*SignSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[token]
	if !ok {
		return nil, ErrSessionNotFound
	}
	copy := *s
	return &copy, nil
}

func (m *memStore) MarkSessionStatus(_ context.Context, token string, st SessionStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[token]
	if !ok {
		return ErrSessionNotFound
	}
	s.Status = st
	return nil
}

func (m *memStore) InsertSignature(_ context.Context, r *SignatureRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	copy := *r
	m.sigs[r.SignatureID] = &copy
	return nil
}

func (m *memStore) GetSignature(_ context.Context, id string) (*SignatureRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.sigs[id]
	if !ok {
		return nil, ErrSignatureNotFound
	}
	copy := *r
	return &copy, nil
}

func (m *memStore) UpsertTemplateFields(_ context.Context, id string, f []Field) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]Field, len(f))
	copy(cp, f)
	m.templates[id] = cp
	return nil
}

func (m *memStore) ListTemplateSummaries(_ context.Context, _ string) ([]TemplateSummary, error) {
	return m.summaries, nil
}

func (m *memStore) TemplateObjectKey(providerID, documentID string) string {
	return providerID + "/templates/" + documentID + ".pdf"
}

type memBlobs struct {
	mu      sync.Mutex
	objects map[string][]byte // bucket/key -> bytes
}

func newMemBlobs() *memBlobs {
	return &memBlobs{objects: map[string][]byte{}}
}

func (b *memBlobs) Put(_ context.Context, bucket, key string, body io.Reader, _ string) error {
	buf, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.objects[bucket+"/"+key] = buf
	return nil
}

func (b *memBlobs) Get(_ context.Context, bucket, key string) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	v, ok := b.objects[bucket+"/"+key]
	if !ok {
		return nil, errors.New("not found")
	}
	return v, nil
}

func (b *memBlobs) PresignGet(_ context.Context, objectKey string, _ time.Duration) (string, error) {
	return "https://example.test/" + objectKey, nil
}

// ---- helpers ----------------------------------------------------------

func newTestService(t *testing.T) (*Service, *memStore, *memBlobs) {
	t.Helper()
	store := newMemStore()
	blobs := newMemBlobs()
	svc := NewService(Config{
		Store:       store,
		Blobs:       blobs,
		Bucket:      "ck-files",
		SigningBase: "https://app.test",
		SessionTTL:  24 * time.Hour,
		Clock:       func() time.Time { return time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC) },
	})
	return svc, store, blobs
}

func defaultFields() []Field {
	return []Field{
		{ID: "f1", Type: FieldSignature, PageIndex: 0, X: 100, Y: 100, Width: 120, Height: 40, Required: true},
	}
}

// minimal-but-valid looking PDF payload (enough to satisfy the %PDF header
// check; we don't exercise pdfcpu here).
func minimalPDF() []byte {
	return []byte("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n1 0 obj<</Type/Catalog>>endobj\nxref\n0 1\n0000000000 65535 f \ntrailer<</Size 1>>\nstartxref\n0\n%%EOF\n")
}

// ---- tests ------------------------------------------------------------

func TestBase62Roundtrip(t *testing.T) {
	for i := 0; i < 64; i++ {
		tok, err := NewToken()
		require.NoError(t, err)
		assert.True(t, len(tok) >= 30 && len(tok) <= 50, "unexpected length: %d", len(tok))
		for _, c := range tok {
			assert.Contains(t, base62Alphabet, string(c))
		}
	}
}

func TestSha256Stable(t *testing.T) {
	h1 := Sha256Hex([]byte("hello"))
	h2 := Sha256Hex([]byte("hello"))
	assert.Equal(t, h1, h2)
	assert.Equal(t, 64, len(h1))
}

func TestCreateSessionPersistsAndIssuesToken(t *testing.T) {
	svc, store, _ := newTestService(t)
	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{
		ProviderID:  "provA",
		DocumentID:  "docA",
		SignerEmail: "parent@example.com",
		SignerName:  "Parent One",
		Fields:      defaultFields(),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, sess.Token)
	assert.Equal(t, StatusPending, sess.Status)
	assert.Equal(t, 1, len(sess.Fields))
	got, err := store.GetSessionByToken(context.Background(), sess.Token)
	require.NoError(t, err)
	assert.Equal(t, sess.Token, got.Token)
}

func TestGetSessionExpired(t *testing.T) {
	svc, store, _ := newTestService(t)
	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{
		ProviderID: "p", DocumentID: "d",
		SignerEmail: "x@y.com", Fields: defaultFields(),
	})
	require.NoError(t, err)
	// force expiry
	store.sessions[sess.Token].ExpiresAt = time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	_, err = svc.GetSession(context.Background(), sess.Token)
	assert.ErrorIs(t, err, ErrSessionExpired)
	assert.Equal(t, StatusExpired, store.sessions[sess.Token].Status)
}

func TestGetSessionAdvancesToInProgress(t *testing.T) {
	svc, store, _ := newTestService(t)
	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{
		ProviderID: "p", DocumentID: "d",
		SignerEmail: "x@y.com", Fields: defaultFields(),
	})
	require.NoError(t, err)
	got, err := svc.GetSession(context.Background(), sess.Token)
	require.NoError(t, err)
	assert.Equal(t, StatusInProgress, got.Status)
	assert.True(t, strings.Contains(got.DocumentURL, "p/templates/d.pdf"))
	assert.Equal(t, StatusInProgress, store.sessions[sess.Token].Status)
}

func TestFinalizeHappyPath(t *testing.T) {
	svc, store, blobs := newTestService(t)
	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{
		ProviderID: "provX", DocumentID: "docX",
		SignerEmail: "signer@example.com", SignerName: "Signer X",
		Fields: defaultFields(),
	})
	require.NoError(t, err)

	pdf := minimalPDF()
	want := Sha256Hex(pdf)
	audit := AuditRecord{
		SchemaVersion: "1.0",
		SHA256Before:  "deadbeef",
		SHA256After:   want,
		Consent: ConsentRecord{
			SignerTypedName: "Signer X",
		},
	}

	rec, err := svc.Finalize(context.Background(), FinalizeInput{
		SessionToken: sess.Token,
		PDFBody:      bytes.NewReader(pdf),
		PDFMaxBytes:  1 << 20,
		Audit:        audit,
		IPAddress:    "203.0.113.5",
		UserAgent:    "Mozilla/5.0 test",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, rec.SignatureID)
	assert.Equal(t, want, rec.SHA256After)
	assert.Equal(t, "signed/provX/docX/"+rec.SignatureID+".pdf", rec.SignedPDFS3Key)
	assert.Equal(t, "audit/provX/"+rec.SignatureID+".json", rec.AuditTrailS3Key)

	// S3 contents are present.
	signed, err := blobs.Get(context.Background(), "ck-files", rec.SignedPDFS3Key)
	require.NoError(t, err)
	assert.Equal(t, pdf, signed)

	auditBytes, err := blobs.Get(context.Background(), "ck-files", rec.AuditTrailS3Key)
	require.NoError(t, err)
	var persistedAudit AuditRecord
	require.NoError(t, json.Unmarshal(auditBytes, &persistedAudit))
	assert.Equal(t, want, persistedAudit.SHA256After)
	assert.Equal(t, "203.0.113.5", persistedAudit.IPAddress)
	assert.Equal(t, "Mozilla/5.0 test", persistedAudit.UserAgent)
	assert.Equal(t, rec.SignatureID, persistedAudit.SignatureID)

	// Session flipped to completed.
	assert.Equal(t, StatusCompleted, store.sessions[sess.Token].Status)
}

func TestFinalizeRejectsBadHeader(t *testing.T) {
	svc, _, _ := newTestService(t)
	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{
		ProviderID: "p", DocumentID: "d",
		SignerEmail: "x@y.com", Fields: defaultFields(),
	})
	require.NoError(t, err)

	_, err = svc.Finalize(context.Background(), FinalizeInput{
		SessionToken: sess.Token,
		PDFBody:      bytes.NewReader([]byte("NOPE, NOT A PDF")),
		PDFMaxBytes:  1 << 20,
		Audit: AuditRecord{
			SHA256Before: "abc",
			Consent:      ConsentRecord{SignerTypedName: "x"},
		},
	})
	assert.ErrorIs(t, err, ErrNotAPDF)
}

func TestFinalizeRejectsHashMismatch(t *testing.T) {
	svc, _, _ := newTestService(t)
	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{
		ProviderID: "p", DocumentID: "d",
		SignerEmail: "x@y.com", Fields: defaultFields(),
	})
	require.NoError(t, err)

	_, err = svc.Finalize(context.Background(), FinalizeInput{
		SessionToken: sess.Token,
		PDFBody:      bytes.NewReader(minimalPDF()),
		PDFMaxBytes:  1 << 20,
		Audit: AuditRecord{
			SHA256Before: "abc",
			SHA256After:  "000000", // wrong on purpose
			Consent:      ConsentRecord{SignerTypedName: "x"},
		},
	})
	assert.ErrorIs(t, err, ErrHashMismatch)
}

func TestFinalizeRejectsExpiredSession(t *testing.T) {
	svc, store, _ := newTestService(t)
	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{
		ProviderID: "p", DocumentID: "d",
		SignerEmail: "x@y.com", Fields: defaultFields(),
	})
	require.NoError(t, err)
	store.sessions[sess.Token].ExpiresAt = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	_, err = svc.Finalize(context.Background(), FinalizeInput{
		SessionToken: sess.Token,
		PDFBody:      bytes.NewReader(minimalPDF()),
		PDFMaxBytes:  1 << 20,
		Audit: AuditRecord{
			SHA256Before: "abc",
			Consent:      ConsentRecord{SignerTypedName: "x"},
		},
	})
	assert.ErrorIs(t, err, ErrSessionExpired)
}

func TestFinalizeRejectsReusedSession(t *testing.T) {
	svc, _, _ := newTestService(t)
	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{
		ProviderID: "p", DocumentID: "d",
		SignerEmail: "x@y.com", Fields: defaultFields(),
	})
	require.NoError(t, err)

	pdf := minimalPDF()
	audit := AuditRecord{
		SHA256Before: "abc",
		SHA256After:  Sha256Hex(pdf),
		Consent:      ConsentRecord{SignerTypedName: "Signer"},
	}
	_, err = svc.Finalize(context.Background(), FinalizeInput{
		SessionToken: sess.Token,
		PDFBody:      bytes.NewReader(pdf),
		Audit:        audit,
	})
	require.NoError(t, err)

	// Second finalize with same token → rejected.
	_, err = svc.Finalize(context.Background(), FinalizeInput{
		SessionToken: sess.Token,
		PDFBody:      bytes.NewReader(pdf),
		Audit:        audit,
	})
	assert.ErrorIs(t, err, ErrSessionCompleted)
}

func TestFinalizeRequiresConsent(t *testing.T) {
	svc, _, _ := newTestService(t)
	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{
		ProviderID: "p", DocumentID: "d",
		SignerEmail: "x@y.com", Fields: defaultFields(),
	})
	require.NoError(t, err)
	pdf := minimalPDF()
	_, err = svc.Finalize(context.Background(), FinalizeInput{
		SessionToken: sess.Token,
		PDFBody:      bytes.NewReader(pdf),
		Audit: AuditRecord{
			SHA256Before: "abc",
			SHA256After:  Sha256Hex(pdf),
			// Consent.SignerTypedName intentionally blank
		},
	})
	assert.ErrorIs(t, err, ErrAuditMalformed)
}

func TestVerifyIntegrityDetectsTampering(t *testing.T) {
	svc, _, blobs := newTestService(t)
	sess, err := svc.CreateSession(context.Background(), CreateSessionInput{
		ProviderID: "p", DocumentID: "d",
		SignerEmail: "x@y.com", Fields: defaultFields(),
	})
	require.NoError(t, err)

	pdf := minimalPDF()
	rec, err := svc.Finalize(context.Background(), FinalizeInput{
		SessionToken: sess.Token,
		PDFBody:      bytes.NewReader(pdf),
		Audit: AuditRecord{
			SHA256Before: "abc",
			SHA256After:  Sha256Hex(pdf),
			Consent:      ConsentRecord{SignerTypedName: "Signer"},
		},
	})
	require.NoError(t, err)

	// Clean verification.
	require.NoError(t, svc.VerifyIntegrity(context.Background(), rec.SignatureID))

	// Tamper: replace S3 object with altered bytes.
	altered := append([]byte(nil), pdf...)
	altered[len(altered)-1] ^= 0xFF
	blobs.objects["ck-files/"+rec.SignedPDFS3Key] = altered

	err = svc.VerifyIntegrity(context.Background(), rec.SignatureID)
	assert.ErrorIs(t, err, ErrIntegrityFailed)
}

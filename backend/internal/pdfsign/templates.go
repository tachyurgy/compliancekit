package pdfsign

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// Template is a blank PDF a provider sends out for signature, plus the
// signature/date/text fields placed on it in the FieldDesigner.
type Template struct {
	ID         string    `json:"id"`
	ProviderID string    `json:"providerId"`
	Name       string    `json:"name"`
	PageCount  int       `json:"pageCount"`
	SHA256     string    `json:"sha256"`
	Fields     []Field   `json:"fields"`
	CreatedAt  time.Time `json:"createdAt"`
}

// SignatureSummary is one completed signature as listed to the provider.
type SignatureSummary struct {
	SignatureID  string    `json:"signatureId"`
	DocumentID   string    `json:"documentId"`
	TemplateName string    `json:"templateName"`
	SignerName   string    `json:"signerName"`
	SignerEmail  string    `json:"signerEmail"`
	SignedAt     time.Time `json:"signedAt"`
	SHA256After  string    `json:"sha256After"`
	SignedPDFKey string    `json:"-"`
	AuditKey     string    `json:"-"`
	SignedPDFURL string    `json:"signedPdfUrl,omitempty"`
	AuditURL     string    `json:"auditUrl,omitempty"`
}

// pageObject matches "/Type /Page" but not "/Type /Pages". Good enough to
// show a page count in the UI; the browser's pdf.js is the real renderer.
var pageObject = regexp.MustCompile(`/Type\s*/Page[^s]`)

// UploadTemplate stores a blank PDF for a provider and records it.
func (s *Service) UploadTemplate(ctx context.Context, providerID, name string, body io.Reader, maxBytes int64) (*Template, error) {
	name = strings.TrimSpace(name)
	if providerID == "" || name == "" {
		return nil, errors.New("pdfsign: provider and name are required")
	}
	b, err := readAllCapped(body, maxBytes)
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(b, pdfHeader) {
		return nil, ErrNotAPDF
	}
	id, err := NewToken()
	if err != nil {
		return nil, err
	}
	id = id[:22]
	pages := len(pageObject.FindAll(b, -1))
	if pages == 0 {
		pages = 1
	}
	t := &Template{
		ID: id, ProviderID: providerID, Name: name, PageCount: pages,
		SHA256: Sha256Hex(b), Fields: []Field{}, CreatedAt: s.clock().UTC(),
	}
	if err := s.blobs.Put(ctx, s.bucket, s.store.TemplateObjectKey(providerID, id), bytes.NewReader(b), "application/pdf"); err != nil {
		return nil, fmt.Errorf("put template: %w", err)
	}
	if err := s.store.InsertTemplate(ctx, t); err != nil {
		return nil, fmt.Errorf("insert template: %w", err)
	}
	return t, nil
}

// TemplatePDF returns the raw bytes of a provider's template.
func (s *Service) TemplatePDF(ctx context.Context, providerID, id string) ([]byte, error) {
	if _, err := s.store.GetTemplate(ctx, providerID, id); err != nil {
		return nil, err
	}
	return s.blobs.Get(ctx, s.bucket, s.store.TemplateObjectKey(providerID, id))
}

// ListSignatures returns recent signatures with short-lived download links
// for the signed PDF and its audit JSON.
func (s *Service) ListSignatures(ctx context.Context, providerID string) ([]SignatureSummary, error) {
	out, err := s.store.ListSignatures(ctx, providerID, 50)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if u, err := s.blobs.PresignGet(ctx, out[i].SignedPDFKey, 15*time.Minute); err == nil {
			out[i].SignedPDFURL = u
		}
		if u, err := s.blobs.PresignGet(ctx, out[i].AuditKey, 15*time.Minute); err == nil {
			out[i].AuditURL = u
		}
	}
	return out, nil
}

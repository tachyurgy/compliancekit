package pdfsign

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTemplateTenancyAndHashBinding(t *testing.T) {
	ctx := context.Background()
	store, blobs := newMemStore(), newMemBlobs()
	svc := NewService(Config{Store: store, Blobs: blobs, Bucket: "signed", SigningBase: "https://x"})

	pdf := "%PDF-1.4\n1 0 obj << /Type /Page >> endobj\n%%EOF"
	tpl, err := svc.UploadTemplate(ctx, "prov-a", "Permission slip", strings.NewReader(pdf), 0)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if tpl.PageCount != 1 || tpl.SHA256 != Sha256Hex([]byte(pdf)) {
		t.Fatalf("unexpected template %+v", tpl)
	}
	if _, err := svc.UploadTemplate(ctx, "prov-a", "x", strings.NewReader("not a pdf"), 0); !errors.Is(err, ErrNotAPDF) {
		t.Fatalf("want ErrNotAPDF, got %v", err)
	}

	fields := []Field{{ID: "f1", Type: FieldSignature, Width: 100, Height: 30, Required: true}}
	if _, err := svc.CreateSession(ctx, CreateSessionInput{
		ProviderID: "prov-b", DocumentID: tpl.ID, SignerEmail: "p@example.com", Fields: fields,
	}); !errors.Is(err, ErrTemplateNotFound) {
		t.Fatalf("cross-tenant session: want ErrTemplateNotFound, got %v", err)
	}
	if _, err := svc.TemplatePDF(ctx, "prov-b", tpl.ID); !errors.Is(err, ErrTemplateNotFound) {
		t.Fatalf("cross-tenant pdf: want ErrTemplateNotFound, got %v", err)
	}

	sess, err := svc.CreateSession(ctx, CreateSessionInput{
		ProviderID: "prov-a", DocumentID: tpl.ID, SignerName: "Pat", SignerEmail: "p@example.com", Fields: fields,
	})
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	signed := pdf + "\n% stamped"
	_, err = svc.Finalize(ctx, FinalizeInput{
		SessionToken: sess.Token,
		PDFBody:      strings.NewReader(signed),
		Audit: AuditRecord{
			SHA256Before: Sha256Hex([]byte("some other pdf")),
			Consent:      ConsentRecord{SignerTypedName: "Pat", AcceptedAt: time.Now()},
		},
	})
	if !errors.Is(err, ErrHashMismatch) {
		t.Fatalf("wrong sha256Before: want ErrHashMismatch, got %v", err)
	}
	rec, err := svc.Finalize(ctx, FinalizeInput{
		SessionToken: sess.Token,
		PDFBody:      strings.NewReader(signed),
		Audit: AuditRecord{
			SHA256Before: tpl.SHA256,
			Consent:      ConsentRecord{SignerTypedName: "Pat", AcceptedAt: time.Now()},
		},
	})
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if rec.SHA256After != Sha256Hex([]byte(signed)) {
		t.Fatal("server must record the hash of the bytes it received")
	}
}

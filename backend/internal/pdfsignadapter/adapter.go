// Package pdfsignadapter glues the external pdfsign package to the rest of
// the backend: it adapts our storage.Client to pdfsign.BlobStore, our session
// middleware context to pdfsign.AuthContext, and implements api.PDFSignRoutes
// so main.go can wire pdfsign.Handlers into the router via Deps.PDFSign.
//
// Without this adapter the pdfsign package compiles but no routes mount —
// a state we shipped at until this package landed.
package pdfsignadapter

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
	"github.com/tachyurgy/compliancekit/backend/internal/pdfsign"
	"github.com/tachyurgy/compliancekit/backend/internal/storage"
)

// Adapter implements api.PDFSignRoutes.
type Adapter struct {
	h *pdfsign.Handlers
}

// New constructs an Adapter given a pdfsign Service and storage client.
// signingBase is the absolute URL prefix used when building /sign/{token}
// links (e.g., https://compliancekit.com).
func New(svc *pdfsign.Service, store pdfsign.Store, s3 *storage.Client, bucket, signingBase string) *Adapter {
	h := &pdfsign.Handlers{
		Svc:        svc,
		Auth:       &sessionAuth{},
		MaxPDFSize: 25 << 20, // 25 MiB
	}
	return &Adapter{h: h}
}

// MountProviderRoutes attaches the operator routes (templates, create session)
// under /pdfsign on the caller's router, which is the /api subrouter already
// behind session auth and the paywall, giving /api/pdfsign/*.
func (a *Adapter) MountProviderRoutes(r chi.Router) {
	a.h.RegisterProvider(r)
}

// MountPublicRoutes attaches the signer endpoints (GET session, POST
// finalize) to the root router. The signer UI at /sign/:token calls them with
// no login; the session token is the credential.
func (a *Adapter) MountPublicRoutes(r chi.Router) {
	a.h.RegisterSigner(r)
}

// sessionAuth implements pdfsign.AuthContext by reading the provider_id that
// our RequireProviderSession middleware places on the request context.
type sessionAuth struct{}

func (sessionAuth) ProviderID(r *http.Request) (string, bool) {
	id := mw.ProviderIDFrom(r.Context())
	if id == "" {
		return "", false
	}
	return id, true
}

// --- BlobStore adapter ---

// NewBlobStore adapts storage.Client to the pdfsign.BlobStore interface.
// Templates, signed PDFs and audit JSON all live in the signed-PDFs bucket
// under templates/, signed/ and audit/ prefixes; PresignGet mints the
// short-lived links the signer and the operator download from.
func NewBlobStore(c *storage.Client) pdfsign.BlobStore {
	return &blobStoreAdapter{c: c}
}

type blobStoreAdapter struct {
	c *storage.Client
}

func (b *blobStoreAdapter) Put(ctx context.Context, bucket, key string, body io.Reader, contentType string) error {
	// Honor the bucket pdfsign asks for (the signed-PDFs bucket). An earlier
	// version collapsed every write into the documents bucket while
	// PresignGet read from signed-PDFs, so a template could never be fetched.
	return b.c.PutToBucket(ctx, bucket, key, contentType, body)
}

func (b *blobStoreAdapter) Get(ctx context.Context, bucket, key string) ([]byte, error) {
	rc, _, err := b.c.GetFromBucket(ctx, bucket, key)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func (b *blobStoreAdapter) PresignGet(ctx context.Context, objectKey string, ttl time.Duration) (string, error) {
	return b.c.PresignGetURL(ctx, b.c.Buckets().SignedPDFs, objectKey, ttl)
}

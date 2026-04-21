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

// MountProviderRoutes attaches the pdfsign provider-authenticated routes onto
// the given chi router. The external package's Register() mounts everything
// under /api/pdfsign (createSession, getSession, finalize, templates, etc.).
// Our caller has already nested this router behind session-auth middleware.
func (a *Adapter) MountProviderRoutes(r chi.Router) {
	a.h.Register(r)
}

// MountPublicRoutes is a no-op today — the pdfsign flow treats the token in
// /api/pdfsign/sessions/{token} as its own auth, and the signer UI lives on
// the frontend at /sign/:token calling the same /api/pdfsign/sessions/{token}
// endpoints. Left as a hook point for future magic-link-gated public routes.
func (a *Adapter) MountPublicRoutes(r chi.Router) {
	// intentionally empty
	_ = r
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
// pdfsign writes to the documents bucket and reads signed PDFs by key;
// PresignGet is used to hand the signer a short-lived download of their
// finalized copy.
func NewBlobStore(c *storage.Client) pdfsign.BlobStore {
	return &blobStoreAdapter{c: c}
}

type blobStoreAdapter struct {
	c *storage.Client
}

func (b *blobStoreAdapter) Put(ctx context.Context, bucket, key string, body io.Reader, contentType string) error {
	// storage.Client's PutDocument does not take a bucket — it uses the
	// configured Documents bucket. For MVP we collapse bucket→Documents;
	// signed/audit writes go to the same S3 bucket under different prefixes.
	return b.c.PutDocument(ctx, key, contentType, body)
}

func (b *blobStoreAdapter) Get(ctx context.Context, bucket, key string) ([]byte, error) {
	rc, _, err := b.c.GetDocument(ctx, key)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func (b *blobStoreAdapter) PresignGet(ctx context.Context, objectKey string, ttl time.Duration) (string, error) {
	return b.c.PresignGetURL(ctx, b.c.Buckets().SignedPDFs, objectKey, ttl)
}

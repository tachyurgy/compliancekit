package api

// Route map (v1):
//
//   PUBLIC
//   POST  /api/auth/signup              providers.Signup       (rate-limited)
//   POST  /api/auth/signin              providers.Signin       (rate-limited)
//   GET   /api/auth/callback            providers.Callback
//   POST  /api/auth/logout              providers.Logout
//   POST  /webhooks/stripe              billing.HandleWebhook  (raw body)
//
//   SESSION-PROTECTED (/api/**)
//   GET   /api/me                       providers.Me
//   PATCH /api/me                       providers.UpdateMe
//   GET   /api/dashboard                dashboard.Get
//   GET/POST/PATCH/DELETE /api/children  children.CRUD
//   GET   /api/children/{id}/documents  children.ListDocuments
//   GET/POST/PATCH/DELETE /api/staff     staff.CRUD
//   GET   /api/staff/{id}/documents     staff.ListDocuments
//   POST  /api/documents/presign        documents.Presign
//   POST  /api/documents/{id}/finalize  documents.Finalize
//   GET   /api/documents                documents.List
//   GET   /api/documents/{id}           documents.Get
//   DELETE /api/documents/{id}          documents.Delete
//   POST  /api/billing/checkout         billing.CreateCheckout
//   POST  /api/billing/portal           billing.Portal
//   POST  /api/sign/request             pdfsign.RequestSignature (external pkg)
//   GET   /api/sign/status/{id}         pdfsign.Status           (external pkg)
//
//   MAGIC-LINK PROTECTED
//   GET   /portal/parent                portal.ParentHome
//   GET   /portal/staff                 portal.StaffHome
//   POST  /portal/upload                portal.Upload
//   GET   /sign/{id}                    pdfsign.SignerPage       (external pkg)
//   POST  /sign/{id}/submit             pdfsign.SubmitSignature  (external pkg)

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/tachyurgy/compliancekit/backend/internal/handlers"
	"github.com/tachyurgy/compliancekit/backend/internal/magiclink"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
)

// PDFSignRoutes is implemented by the external pdfsign package — it registers
// its own routes onto a subrouter. We only invoke it here.
type PDFSignRoutes interface {
	MountProviderRoutes(r chi.Router)
	MountPublicRoutes(r chi.Router)
}

// Deps wires every feature together for NewRouter.
type Deps struct {
	Logger          *slog.Logger
	Providers       *handlers.ProviderHandler
	Children        *handlers.ChildHandler
	Staff           *handlers.StaffHandler
	Documents       *handlers.DocumentHandler
	Dashboard       *handlers.DashboardHandler
	Portal          *handlers.PortalHandler
	Billing         *handlers.BillingHandler
	StripeWebhook   *handlers.StripeWebhookHandler
	Magic           *magiclink.Service
	Session         mw.SessionReader
	BillingChecker  mw.BillingChecker
	RateLimit       *mw.TokenBucket
	FrontendOrigins []string
	PDFSign         PDFSignRoutes // provided by the pdfsign package; may be nil
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(slogRequestLogger(d.Logger))
	r.Use(chimw.Timeout(30 * time.Second))

	if len(d.FrontendOrigins) > 0 {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   d.FrontendOrigins,
			AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Authorization", "Content-Type"},
			AllowCredentials: true,
			MaxAge:           300,
		}))
	}

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	// Public auth routes (rate-limited)
	r.Group(func(r chi.Router) {
		if d.RateLimit != nil {
			r.Use(d.RateLimit.Limit("auth"))
		}
		r.Post("/api/auth/signup", d.Providers.Signup)
		r.Post("/api/auth/signin", d.Providers.Signin)
		r.Get("/api/auth/callback", d.Providers.Callback)
		r.Post("/api/auth/logout", d.Providers.Logout)
	})

	// Stripe webhook — raw body required by Stripe signing.
	r.Post("/webhooks/stripe", d.StripeWebhook.Handle)

	// Session-protected API.
	r.Route("/api", func(r chi.Router) {
		r.Use(mw.RequireProviderSession(d.Session))

		r.Get("/me", d.Providers.Me)
		r.Patch("/me", d.Providers.UpdateMe)
		r.Get("/dashboard", d.Dashboard.Get)

		r.Route("/children", func(r chi.Router) {
			r.Get("/", d.Children.List)
			r.Post("/", d.Children.Create)
			r.Get("/{id}", d.Children.Get)
			r.Patch("/{id}", d.Children.Update)
			r.Delete("/{id}", d.Children.Delete)
			r.Get("/{id}/documents", d.Children.ListDocuments)
		})

		r.Route("/staff", func(r chi.Router) {
			r.Get("/", d.Staff.List)
			r.Post("/", d.Staff.Create)
			r.Get("/{id}", d.Staff.Get)
			r.Patch("/{id}", d.Staff.Update)
			r.Delete("/{id}", d.Staff.Delete)
			r.Get("/{id}/documents", d.Staff.ListDocuments)
		})

		r.Route("/documents", func(r chi.Router) {
			r.Post("/presign", d.Documents.Presign)
			r.Get("/", d.Documents.List)
			r.Get("/{id}", d.Documents.Get)
			r.Post("/{id}/finalize", d.Documents.Finalize)
			r.Delete("/{id}", d.Documents.Delete)
		})

		r.Route("/billing", func(r chi.Router) {
			r.Post("/checkout", d.Billing.CreateCheckout)
			r.Post("/portal", d.Billing.Portal)
		})

		// Paywalled routes — require active subscription
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireStripeCustomer(d.BillingChecker))
			if d.PDFSign != nil {
				d.PDFSign.MountProviderRoutes(r)
			}
		})
	})

	// Magic-link protected portal routes.
	r.Route("/portal", func(r chi.Router) {
		if d.RateLimit != nil {
			r.Use(d.RateLimit.Limit("portal"))
		}
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireIndividualMagicLink(d.Magic, magiclink.KindParentUpload))
			r.Get("/parent", d.Portal.ParentHome)
		})
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireIndividualMagicLink(d.Magic, magiclink.KindStaffUpload))
			r.Get("/staff", d.Portal.StaffHome)
		})
		r.Group(func(r chi.Router) {
			r.Use(mw.RequireIndividualMagicLink(d.Magic, magiclink.KindParentUpload, magiclink.KindStaffUpload))
			r.Post("/upload", d.Portal.Upload)
		})
	})

	// pdfsign package mounts its own magic-link-gated sign endpoints.
	if d.PDFSign != nil {
		d.PDFSign.MountPublicRoutes(r)
	}

	return r
}

// slogRequestLogger is a chi-compatible logging middleware using slog.
func slogRequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			logger.LogAttrs(r.Context(), slog.LevelInfo, "http",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", ww.Status()),
				slog.Int("bytes", ww.BytesWritten()),
				slog.Duration("took", time.Since(start)),
				slog.String("request_id", chimw.GetReqID(r.Context())),
				slog.String("remote", r.RemoteAddr),
			)
		})
	}
}

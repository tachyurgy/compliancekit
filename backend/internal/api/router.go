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
//   POST  /api/provider/onboarding      providers.CompleteOnboarding (admin)
//   GET   /api/dashboard                dashboard.Get
//   GET/POST/PATCH/DELETE /api/children  children.CRUD
//   GET   /api/children/{id}/documents  children.ListDocuments
//   POST  /api/children/{id}/portal-link providers.MintParentPortalLink (admin)
//   GET/POST/PATCH/DELETE /api/staff     staff.CRUD
//   GET   /api/staff/{id}/documents     staff.ListDocuments
//   POST  /api/staff/{id}/portal-link   providers.MintStaffPortalLink (admin)
//   POST  /api/documents/presign        documents.Presign
//   POST  /api/documents/{id}/finalize  documents.Finalize
//   GET   /api/documents                documents.List
//   GET   /api/documents/{id}           documents.Get
//   DELETE /api/documents/{id}          documents.Delete
//   POST  /api/billing/checkout         billing.CreateCheckout
//   POST  /api/billing/portal           billing.Portal
//   GET   /api/drills                   drills.List
//   POST  /api/drills                   drills.Create
//   DELETE /api/drills/{id}             drills.Delete (soft)
//   GET   /api/facility/postings        postings.List
//   PATCH /api/facility/postings/{key}  postings.Upsert
//   POST  /api/facility/ratio-check     ratio.Check
//   POST  /api/inspections              inspections.Start
//   GET   /api/inspections              inspections.List
//   GET   /api/inspections/{id}         inspections.Get
//   PATCH /api/inspections/{id}/items/{item_id}  inspections.UpsertResponse
//   POST  /api/inspections/{id}/finalize inspections.Finalize
//   GET   /api/inspections/{id}/report.pdf inspections.Report
//   GET   /api/inspection/snapshot                              inspectionMode.Snapshot
//   POST  /api/inspection/sessions                              inspectionMode.StartSession
//   POST  /api/inspection/sessions/{id}/sample                  inspectionMode.Sample
//   POST  /api/inspection/sessions/{id}/finding                 inspectionMode.AddFinding
//   POST  /api/inspection/sessions/{id}/end                     inspectionMode.EndSession
//   GET   /api/inspection/sessions/{id}/plan-of-correction.pdf  inspectionMode.DownloadPlan
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
	BillingV2       *handlers.BillingV2Handler // REQ-002 shape (annual/monthly)
	Auth            *handlers.AuthHandler      // REQ-011 forgot/reset/sessions
	Legal           *handlers.LegalHandler     // REQ-002 ToS/DPA e-sign
	StripeWebhook   *handlers.StripeWebhookHandler
	Drills          *handlers.DrillHandler
	Postings        *handlers.PostingHandler
	Ratio           *handlers.RatioHandler
	Inspections     *handlers.InspectionHandler
	InspectionMode  *handlers.InspectionModeHandler
	AuditLog        *handlers.AuditLogHandler
	DataExport      *handlers.DataExportHandler
	Contact         *handlers.ContactHandler
	Nag             *handlers.NagHandler
	Facility        *handlers.FacilityHandler   // REQ-003: 4-step onboarding wizard
	UploadLink      *handlers.UploadLinkHandler // REQ-004: shareable staff upload link
	Score           *handlers.ScoreHandler      // REQ-013: synthesised dashboard score
	Kiosk           *handlers.KioskHandler      // REQ-016: facility-scoped attendance kiosk
	Attendance      *handlers.AttendanceHandler // REQ-016: today + history + drill roster PDF
	TestHelpers     *handlers.TestHelperHandler // only mounted when non-production
	Magic           *magiclink.Service
	Session         mw.SessionReader
	RoleLookup      mw.RoleLookuper
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

	// Test helper endpoints — only mounted when non-production. Callers pass
	// a non-nil TestHelpers handler; leaving Deps.TestHelpers nil skips mount.
	if d.TestHelpers != nil {
		r.Get("/api/test/latest-magic-link", d.TestHelpers.LatestMagicLink)
		r.Post("/api/test/session", d.TestHelpers.CreateSession)
		r.Post("/api/test/reset", d.TestHelpers.Reset)
	}

	// Public auth routes (rate-limited)
	r.Group(func(r chi.Router) {
		if d.RateLimit != nil {
			r.Use(d.RateLimit.Limit("auth"))
		}
		r.Post("/api/auth/signup", d.Providers.Signup)
		r.Post("/api/auth/signin", d.Providers.Signin)
		r.Get("/api/auth/callback", d.Providers.Callback)
		r.Post("/api/auth/logout", d.Providers.Logout)
		// REQ-011: alias /signout for the new frontend; /logout remains for back-compat.
		r.Post("/api/auth/signout", d.Providers.Logout)
		if d.Auth != nil {
			r.Post("/api/auth/forgot-password", d.Auth.ForgotPassword)
			r.Post("/api/auth/reset-password", d.Auth.ResetPassword)
			// REQ-010: consume the email-verification token sent at signup.
			r.Post("/api/auth/verify-email", d.Auth.VerifyEmail)
		}
	})

	// Stripe webhook — raw body required by Stripe signing.
	r.Post("/webhooks/stripe", d.StripeWebhook.Handle)

	// Public marketing-site contact form.
	if d.Contact != nil {
		r.Post("/webhooks/contact", d.Contact.Post)
	}

	// REQ-016a: kiosk endpoints. The session-issue endpoint is admin-only and
	// lives under /api below; the per-event endpoints validate the kiosk
	// session bearer themselves so a mounted iPad can sign children in
	// without a user logged in. The PIN is the second factor.
	if d.Kiosk != nil {
		r.Post("/kiosk/sign-in", d.Kiosk.SignIn)
		r.Post("/kiosk/sign-out", d.Kiosk.SignOut)
		r.Get("/kiosk/roster", d.Kiosk.Roster)
	}

	// adminOnly is a convenience wrapper that gates a subrouter to
	// provider_admin. Kept local so we don't sprinkle the role string at
	// every call site.
	adminOnly := func(inner func(r chi.Router)) func(r chi.Router) {
		return func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(mw.RequireRole(d.RoleLookup, mw.RoleProviderAdmin))
				inner(r)
			})
		}
	}

	// Session-protected API.
	r.Route("/api", func(r chi.Router) {
		r.Use(mw.RequireProviderSession(d.Session))
		// Apply a generous per-session rate limit to the whole /api surface so
		// a compromised or buggy client cannot hammer CRUD endpoints without
		// bound. The auth route group above already has its own stricter
		// limiter. See middleware/ratelimit.go for the token-bucket logic.
		if d.RateLimit != nil {
			r.Use(d.RateLimit.Limit("api"))
		}

		r.Get("/me", d.Providers.Me)
		r.Patch("/me", d.Providers.UpdateMe)
		r.Get("/dashboard", d.Dashboard.Get)

		// REQ-013: synthesised compliance score (and history for the future
		// sparkline). Lives off the engine + classification weights in
		// internal/compliance/score.go.
		if d.Score != nil {
			r.Get("/compliance/score", d.Score.Get)
			r.Get("/compliance/score/history", d.Score.History)
		}

		// REQ-016: kiosk session minting (admin issues a 4h facility-bound
		// token to a mounted iPad) + parent/staff PIN rotation.
		if d.Kiosk != nil {
			r.Group(adminOnly(func(r chi.Router) {
				r.Post("/kiosk/session", d.Kiosk.IssueSession)
				r.Post("/kiosk/pins/staff/{id}", d.Kiosk.RotateStaffPIN)
				r.Post("/kiosk/pins/child/{id}", d.Kiosk.RotateChildPIN)
				r.Post("/settings/kiosk/rotate-pins", d.Kiosk.BulkRotatePINs)
			}))
		}

		// REQ-016: attendance reads + drill roster PDF.
		if d.Attendance != nil {
			r.Get("/attendance/today", d.Attendance.Today)
			r.Get("/attendance/history", d.Attendance.History)
			r.Get("/drill-roster.pdf", d.Attendance.DrillRosterPDF)
		}

		// Onboarding wizard finalize — admin-only. Flipping onboarding_complete
		// and seeding staff/children is a tenant-wide write; gate it the same
		// way as other provider-level mutations.
		r.Group(adminOnly(func(r chi.Router) {
			r.Post("/provider/onboarding", d.Providers.CompleteOnboarding)
		}))

		// Children: reads are open to both roles; writes are admin-only.
		// REQ-008 adds withdraw, nag (parent-side), and a child upload-link.
		r.Route("/children", func(r chi.Router) {
			r.Get("/", d.Children.List)
			r.Get("/{id}", d.Children.Get)
			r.Get("/{id}/documents", d.Children.ListDocuments)
			r.Get("/{id}/upload-link", d.Children.UploadLink)
			r.Group(adminOnly(func(r chi.Router) {
				r.Post("/", d.Children.Create)
				r.Patch("/{id}", d.Children.Update)
				r.Delete("/{id}", d.Children.Delete)
				r.Post("/{id}/withdraw", d.Children.Withdraw)
				r.Post("/{id}/portal-link", d.Providers.MintParentPortalLink)
				if d.Nag != nil {
					r.Post("/{id}/nag", d.Nag.TriggerChild)
				}
			}))
		})

		// Staff: reads are open to both roles; writes are admin-only.
		// REQ-004 swaps Staff.List → ListWithRollup and Staff.Get → GetDetail
		// to surface the status-pill rollup and required-doc checklist.
		r.Route("/staff", func(r chi.Router) {
			r.Get("/", d.Staff.ListWithRollup)
			r.Get("/{id}", d.Staff.GetDetail)
			r.Get("/{id}/documents", d.Staff.ListDocuments)
			if d.Nag != nil {
				r.Get("/{id}/nag-history", d.Nag.History)
			}
			if d.UploadLink != nil {
				r.Get("/{id}/upload-link", d.UploadLink.Get)
			}
			r.Group(adminOnly(func(r chi.Router) {
				r.Post("/", d.Staff.Create)
				r.Patch("/{id}", d.Staff.Update)
				r.Delete("/{id}", d.Staff.Delete)
				r.Post("/{id}/archive", d.Staff.Archive)
				r.Post("/{id}/portal-link", d.Providers.MintStaffPortalLink)
				if d.Nag != nil {
					r.Post("/{id}/nag", d.Nag.Trigger)
				}
			}))
		})

		// REQ-003 (onboarding wizard): facility GET is open to both roles so
		// the dashboard can read; PATCH and complete-onboarding are admin-only.
		// Rooms CRUD lives at /api/rooms within the same `if d.Facility != nil`
		// guard.
		if d.Facility != nil {
			r.Get("/facility", d.Facility.Get)
			r.Group(adminOnly(func(r chi.Router) {
				r.Patch("/facility", d.Facility.Patch)
				r.Post("/facility/complete-onboarding", d.Facility.CompleteOnboarding)
			}))
			r.Route("/rooms", func(r chi.Router) {
				r.Get("/", d.Facility.ListRooms)
				r.Group(adminOnly(func(r chi.Router) {
					r.Post("/", d.Facility.CreateRoom)
					r.Patch("/{id}", d.Facility.PatchRoom)
					r.Delete("/{id}", d.Facility.DeleteRoom)
				}))
			})
		}

		// Documents: list/get are open; presign/finalize/delete are admin-only.
		// REQ-006 adds POST /api/documents/{id}/confirm — admin-only, sets
		// expiration_confirmed_at and ungates the chase engine for that doc.
		r.Route("/documents", func(r chi.Router) {
			r.Get("/", d.Documents.List)
			r.Get("/{id}", d.Documents.Get)
			r.Group(adminOnly(func(r chi.Router) {
				r.Post("/presign", d.Documents.Presign)
				r.Post("/{id}/finalize", d.Documents.Finalize)
				r.Post("/{id}/confirm", d.Documents.Confirm)
				r.Delete("/{id}", d.Documents.Delete)
			}))
		})

		r.Route("/billing", func(r chi.Router) {
			r.Post("/checkout", d.Billing.CreateCheckout)
			r.Post("/portal", d.Billing.Portal)
			// REQ-002 shape: explicit annual/monthly + trial-aware checkout.
			if d.BillingV2 != nil {
				r.Post("/checkout-session", d.BillingV2.CreateCheckoutSession)
				r.Post("/portal-session", d.BillingV2.CreatePortalSession)
				r.Get("/subscription", d.BillingV2.GetSubscription)
			}
		})

		// REQ-011 session-management UI: list/active sessions, sign out everywhere.
		if d.Auth != nil {
			r.Get("/auth/sessions", d.Auth.ListSessions)
			r.Post("/auth/signout-everywhere", d.Auth.SignoutEverywhere)
		}

		// REQ-002 ToS/DPA e-signature capture.
		if d.Legal != nil {
			r.Get("/legal/documents", d.Legal.Documents)
			r.Post("/legal/sign", d.Legal.Sign)
			// admin-only read of the audit trail
			r.Group(adminOnly(func(r chi.Router) {
				r.Get("/legal/signatures", d.Legal.ListSignatures)
			}))
		}

		// Facility & Operations: drills, wall postings, ratio checks.
		if d.Drills != nil {
			r.Route("/drills", func(r chi.Router) {
				r.Get("/", d.Drills.List)
				r.Group(adminOnly(func(r chi.Router) {
					r.Post("/", d.Drills.Create)
					r.Delete("/{id}", d.Drills.Delete)
				}))
			})
		}
		// /api/facility — onboarding wizard read/write (REQ-003) shares
		// the prefix with the existing postings + ratio handlers. Mount
		// them all in one Route block so chi's pattern matching doesn't
		// conflict.
		if d.Postings != nil || d.Ratio != nil || d.Facility != nil {
			r.Route("/facility", func(r chi.Router) {
				if d.Facility != nil {
					r.Get("/", d.Facility.Get)
					r.Group(adminOnly(func(r chi.Router) {
						r.Patch("/", d.Facility.Patch)
						r.Post("/complete-onboarding", d.Facility.CompleteOnboarding)
					}))
				}
				if d.Postings != nil {
					r.Get("/postings", d.Postings.List)
					r.Group(adminOnly(func(r chi.Router) {
						r.Patch("/postings/{key}", d.Postings.Upsert)
					}))
				}
				if d.Ratio != nil {
					r.Group(adminOnly(func(r chi.Router) {
						r.Post("/ratio-check", d.Ratio.Check)
					}))
				}
			})
		}
		if d.Inspections != nil {
			r.Route("/inspections", func(r chi.Router) {
				r.Get("/", d.Inspections.List)
				r.Get("/{id}", d.Inspections.Get)
				r.Get("/{id}/report.pdf", d.Inspections.Report)
				r.Group(adminOnly(func(r chi.Router) {
					r.Post("/", d.Inspections.Start)
					r.Patch("/{id}/items/{item_id}", d.Inspections.UpsertResponse)
					r.Post("/{id}/finalize", d.Inspections.Finalize)
				}))
			})
		}

		// Inspector Mode (REQ-009) — singular path so it doesn't collide with
		// the simulator's /inspections/* surface above. The snapshot endpoint
		// is the read-model that powers all 7 tabs in one round-trip; the
		// session endpoints capture the live event log + Plan of Correction.
		if d.InspectionMode != nil {
			r.Route("/inspection", func(r chi.Router) {
				r.Get("/snapshot", d.InspectionMode.Snapshot)
				r.Group(adminOnly(func(r chi.Router) {
					r.Post("/sessions", d.InspectionMode.StartSession)
					r.Post("/sessions/{id}/sample", d.InspectionMode.Sample)
					r.Post("/sessions/{id}/finding", d.InspectionMode.AddFinding)
					r.Post("/sessions/{id}/end", d.InspectionMode.EndSession)
					r.Get("/sessions/{id}/plan-of-correction.pdf", d.InspectionMode.DownloadPlan)
				}))
			})
		}

		// Audit log: admin-only read.
		if d.AuditLog != nil {
			r.Group(adminOnly(func(r chi.Router) {
				r.Get("/audit-log", d.AuditLog.List)
			}))
		}

		// Data & Retention: export and delete. All endpoints admin-only because
		// both actions expose or destroy the entire tenant dataset.
		if d.DataExport != nil {
			r.Group(adminOnly(func(r chi.Router) {
				r.Get("/exports", d.DataExport.List)
				r.Post("/exports", d.DataExport.Create)
				r.Get("/exports/{id}/download", d.DataExport.Download)
			}))
		}
		// DELETE /api/providers/me schedules a 90-day retention purge.
		r.Group(adminOnly(func(r chi.Router) {
			r.Delete("/providers/me", d.Providers.DeleteMe)
		}))

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
			// REQ-005 multi-shot stitching + EXIF strip happens in Finalize.
			r.Post("/upload/{id}/finalize", d.Portal.Finalize)
			// REQ-006 token-gated confirmation (used right after the upload).
			r.Post("/document/{id}/confirm", d.Portal.ConfirmDocument)
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

// securityHeaders applies baseline security response headers to every request.
// Content-Security-Policy allows Stripe's hosted Checkout + webhook embeds only.
// HSTS is deliberately omitted here and terminated at nginx so dev doesn't
// lock the browser into HTTPS-only for localhost.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), geolocation=(), microphone=()")
		// CSP: only apply to HTML responses (API returns JSON — browsers don't parse
		// CSP there but it's harmless; frontend docs served via Pages have their
		// own. Setting it here protects against an API response being rendered by
		// a misconfigured reverse proxy).
		h.Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' https://js.stripe.com; "+
				"frame-src https://js.stripe.com https://checkout.stripe.com; "+
				"img-src 'self' data: https:; "+
				"style-src 'self' 'unsafe-inline'; "+
				"connect-src 'self' https://api.stripe.com; "+
				"object-src 'none'; "+
				"base-uri 'self'; "+
				"form-action 'self' https://checkout.stripe.com")
		next.ServeHTTP(w, r)
	})
}

// maxBodySize wraps every request with http.MaxBytesReader so a rogue client
// cannot OOM the server with a giant JSON blob. Document uploads use the S3
// presign flow, so they never route through this body — 10 MiB is a generous
// ceiling for JSON.
func maxBodySize(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Stripe webhooks need the raw body byte-exact for signature
			// verification; skip the limiter for them and rely on Stripe's own
			// per-event size cap (<64 KB).
			if r.URL.Path == "/webhooks/stripe" {
				next.ServeHTTP(w, r)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, n)
			next.ServeHTTP(w, r)
		})
	}
}

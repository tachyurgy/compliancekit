package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/api"
	"github.com/tachyurgy/compliancekit/backend/internal/billing"
	"github.com/tachyurgy/compliancekit/backend/internal/config"
	"github.com/tachyurgy/compliancekit/backend/internal/db"
	"github.com/tachyurgy/compliancekit/backend/internal/handlers"
	"github.com/tachyurgy/compliancekit/backend/internal/magiclink"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
	"github.com/tachyurgy/compliancekit/backend/internal/notify"
	"github.com/tachyurgy/compliancekit/backend/internal/ocr"
	"github.com/tachyurgy/compliancekit/backend/internal/pdfsign"
	"github.com/tachyurgy/compliancekit/backend/internal/pdfsignadapter"
	"github.com/tachyurgy/compliancekit/backend/internal/retention"
	"github.com/tachyurgy/compliancekit/backend/internal/storage"
	"github.com/tachyurgy/compliancekit/backend/internal/workers"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log := config.NewLogger(cfg)
	slog.SetDefault(log)

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(rootCtx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer pool.Close()
	log.Info("db connected")

	// ---- AWS S3 + SES ----
	s3Client, err := storage.New(rootCtx, storage.Config{
		Region:          cfg.AWSRegion,
		AccessKeyID:     cfg.AWSAccessKeyID,
		SecretAccessKey: cfg.AWSSecretAccessKey,
		EndpointURL:     cfg.AWSEndpointURL,
		Buckets: storage.Buckets{
			Documents:  cfg.S3BucketDocuments,
			SignedPDFs: cfg.S3BucketSignedPDFs,
			AuditTrail: cfg.S3BucketAuditTrail,
			RawUploads: cfg.S3BucketRawUploads,
		},
	})
	if err != nil {
		return fmt.Errorf("storage: %w", err)
	}

	emailer, err := notify.NewEmailer(rootCtx, notify.EmailerConfig{
		Region:          cfg.AWSRegion,
		AccessKeyID:     cfg.AWSAccessKeyID,
		SecretAccessKey: cfg.AWSSecretAccessKey,
		From:            cfg.SESFromEmail,
	})
	if err != nil {
		log.Warn("emailer init failed (continuing without SES)", "err", err)
	}

	sms := notify.NewSMSSender(notify.SMSConfig{
		AccountSID:          cfg.TwilioAccountSID,
		AuthToken:           cfg.TwilioAuthToken,
		From:                cfg.TwilioFromNumber,
		MessagingServiceSID: cfg.TwilioMessagingServiceSID,
	})

	// ---- magic link + billing ----
	magic := magiclink.NewService(pool, cfg.MagicLinkSigningKey)
	// REQ-002: monthly + annual price IDs. Default monthly to the legacy
	// PricePro env var so existing deploy environments don't need updating.
	priceMonthly := cfg.StripePriceMonthly
	if priceMonthly == "" {
		priceMonthly = cfg.StripePricePro
	}
	priceAnnual := cfg.StripePriceAnnual
	if priceAnnual == "" {
		priceAnnual = cfg.StripePricePro
	}
	bill := billing.New(billing.Config{
		SecretKey:       cfg.StripeSecretKey,
		WebhookSecret:   cfg.StripeWebhookSecret,
		FrontendBaseURL: cfg.FrontendBaseURL,
		PriceIDMonthly:  priceMonthly,
		PriceIDAnnual:   priceAnnual,
		Pool:            pool,
		Logger:          log.With("component", "billing"),
	})

	// ---- OCR chain (Mistral primary, Gemini fallback) ----
	var ocrEngine ocr.OCR
	if cfg.MistralAPIKey != "" && cfg.GeminiAPIKey != "" {
		ocrEngine = ocr.Chain(ocr.NewMistral(cfg.MistralAPIKey), ocr.NewGemini(cfg.GeminiAPIKey), 0.6)
	} else if cfg.MistralAPIKey != "" {
		ocrEngine = ocr.NewMistral(cfg.MistralAPIKey)
	} else if cfg.GeminiAPIKey != "" {
		ocrEngine = ocr.NewGemini(cfg.GeminiAPIKey)
	}
	var expiryExtra *ocr.ExpirationExtractor
	if cfg.MistralAPIKey != "" {
		expiryExtra = ocr.NewExpirationExtractor(cfg.MistralAPIKey)
	}

	// ---- handlers ----
	providers := &handlers.ProviderHandler{
		Pool: pool, Magic: magic, Emailer: emailer,
		FrontendBase: cfg.FrontendBaseURL, AppBase: cfg.AppBaseURL,
		CookieDomain: cfg.SessionCookieDomain, SecureCookie: cfg.IsProduction(),
		Log: log.With("component", "providers"),
	}
	children := &handlers.ChildHandler{
		Pool: pool, Magic: magic, FrontendBase: cfg.FrontendBaseURL,
		Log: log.With("component", "children"),
	}
	staff := &handlers.StaffHandler{Pool: pool, Log: log.With("component", "staff")}
	docs := &handlers.DocumentHandler{
		Pool: pool, Storage: s3Client, OCR: ocrEngine, ExpiryExtra: expiryExtra,
		Log: log.With("component", "documents"),
	}
	dash := &handlers.DashboardHandler{Pool: pool, Log: log.With("component", "dashboard")}
	portal := &handlers.PortalHandler{
		Pool: pool, Storage: s3Client, Magic: magic, Documents: docs,
		Log: log.With("component", "portal"),
	}
	billH := &handlers.BillingHandler{Pool: pool, Billing: bill, StripePrice: cfg.StripePricePro, Log: log.With("component", "billing")}
	billV2 := &handlers.BillingV2Handler{
		Pool: pool, Billing: bill, Emailer: emailer,
		FrontendBase: cfg.FrontendBaseURL, AppBase: cfg.AppBaseURL,
		Log: log.With("component", "billing_v2"),
	}
	authH := &handlers.AuthHandler{
		Pool: pool, Emailer: emailer,
		FrontendBase: cfg.FrontendBaseURL,
		CookieDomain: cfg.SessionCookieDomain, SecureCookie: cfg.IsProduction(),
		Log: log.With("component", "auth"),
	}
	legalH := &handlers.LegalHandler{Pool: pool, Log: log.With("component", "legal")}
	demoH := &handlers.DemoHandler{Pool: pool, Log: log.With("component", "demo")}
	stripeWH := &handlers.StripeWebhookHandler{Billing: bill, Log: log.With("component", "stripe_wh")}
	// REQ-007: opt-out registry hooks. SES bounce JSON arrives via SNS;
	// Twilio STOP-keyword inbound arrives as form-encoded. Both insert into
	// the `unsubscribes` table the chase service honors per-channel.
	sesWH := &handlers.SESWebhookHandler{Pool: pool, Log: log.With("component", "ses_wh")}
	twilioWH := &handlers.TwilioWebhookHandler{Pool: pool, Log: log.With("component", "twilio_wh")}

	// REQ-002 wiring: send the welcome email after Stripe's
	// checkout.session.completed lands, and persist trial info on providers.
	bill.SetNotifier(&handlers.CheckoutNotifier{
		Pool: pool, Emailer: emailer, AppBase: cfg.AppBaseURL,
		Log: log.With("component", "checkout_notifier"),
	})
	// REQ-022 wiring: send the branded payment-failed email after Stripe's
	// invoice.payment_failed webhook lands.
	bill.SetPaymentFailedNotifier(&handlers.PaymentFailedNotifier{
		Pool: pool, Emailer: emailer, AppBase: cfg.AppBaseURL,
		Log: log.With("component", "payment_failed_notifier"),
	})
	// REQ-010 wiring: send the branded trial-converted email when a
	// subscription flips from trialing → active for the first time.
	bill.SetTrialConvertedNotifier(&handlers.TrialConvertedNotifier{
		Pool: pool, Emailer: emailer, AppBase: cfg.AppBaseURL,
		Log: log.With("component", "trial_converted_notifier"),
	})
	drills := &handlers.DrillHandler{Pool: pool, Log: log.With("component", "drills")}
	postings := &handlers.PostingHandler{Pool: pool, Log: log.With("component", "postings")}
	ratio := &handlers.RatioHandler{Pool: pool, Log: log.With("component", "ratio")}
	inspections := &handlers.InspectionHandler{Pool: pool, Log: log.With("component", "inspections")}
	inspectionMode := &handlers.InspectionModeHandler{Pool: pool, Log: log.With("component", "inspector_mode")}
	auditLog := &handlers.AuditLogHandler{Pool: pool, Log: log.With("component", "audit_log")}
	dataExport := &handlers.DataExportHandler{
		Pool: pool, Storage: s3Client, Emailer: emailer,
		DownloadTTL: 15 * time.Minute,
		Log:         log.With("component", "data_export"),
	}
	contact := &handlers.ContactHandler{Pool: pool, Log: log.With("component", "contact")}
	nag := &handlers.NagHandler{
		Pool: pool, Magic: magic, Emailer: emailer, SMS: sms,
		FrontendBase: cfg.FrontendBaseURL,
		Log:          log.With("component", "nag"),
	}
	facility := &handlers.FacilityHandler{Pool: pool, Log: log.With("component", "facility")}
	uploadLink := &handlers.UploadLinkHandler{
		Pool: pool, Magic: magic, FrontendBase: cfg.FrontendBaseURL,
	}

	// REQ-013: synthesised dashboard score handler.
	scoreH := &handlers.ScoreHandler{Pool: pool, Log: log.With("component", "score")}

	// REQ-016: kiosk + attendance + drill roster (no rooms; warnings come from
	// pure compliance/ratio.go computation, not a stateful coach).
	kioskH := &handlers.KioskHandler{Pool: pool, Log: log.With("component", "kiosk")}
	attendanceH := &handlers.AttendanceHandler{Pool: pool, Log: log.With("component", "attendance")}

	// Test helpers — only instantiate in non-production so they can never be
	// mounted against a real prod DB. See handlers/testhelpers.go.
	var testHelpers *handlers.TestHelperHandler
	if !cfg.IsProduction() {
		testHelpers = &handlers.TestHelperHandler{
			Pool:         pool,
			Magic:        magic,
			CookieDomain: cfg.SessionCookieDomain,
			SecureCookie: cfg.IsProduction(),
			Log:          log.With("component", "test_helpers"),
		}
	}

	// ---- chase service (background) ----
	chase := notify.NewChaseService(notify.ChaseDeps{
		Pool:            pool,
		Emailer:         emailer,
		SMS:             sms,
		Magic:           magic,
		FrontendBaseURL: cfg.FrontendBaseURL,
		Logger:          log.With("component", "chase"),
	})
	go chase.RunDaily(rootCtx)

	// ---- session GC + nightly compliance snapshot recompute (background) ----
	sessionGC := &workers.SessionGC{Pool: pool, Log: log.With("component", "session_gc")}
	go sessionGC.RunDaily(rootCtx)

	snapshotWorker := &workers.SnapshotWorker{Pool: pool, Log: log.With("component", "snapshot")}
	go snapshotWorker.RunDaily(rootCtx)

	// REQ-013: daily synth-score snapshot writer (powers the future sparkline).
	scoreSnap := &workers.ScoreSnapshotWorker{Pool: pool, Log: log.With("component", "score_snapshot")}
	go scoreSnap.RunDaily(rootCtx)

	// REQ-022 trial-ending-3-days reminder.
	trialReminder := &workers.TrialReminderWorker{
		Pool: pool, Emailer: emailer, AppBase: cfg.AppBaseURL,
		Log: log.With("component", "trial_reminder"),
	}
	go trialReminder.RunDaily(rootCtx)

	// 90-day data purge worker (GDPR / Terms of Service 90-day grace period).
	purgeWorker := retention.NewPurgeWorker(pool, s3Client, log.With("component", "retention"))
	go purgeWorker.RunDaily(rootCtx)

	// ---- pdfsign (in-house e-signature) ----
	// Adapter bridges the external pdfsign package to our storage + session
	// middleware so Deps.PDFSign is non-nil and /api/pdfsign/* routes mount.
	pdfsignStore := pdfsign.NewPgStore(pool)
	pdfsignSvc := pdfsign.NewService(pdfsign.Config{
		Store:       pdfsignStore,
		Blobs:       pdfsignadapter.NewBlobStore(s3Client),
		Bucket:      cfg.S3BucketSignedPDFs,
		SigningBase: cfg.FrontendBaseURL,
	})
	pdfsignRoutes := pdfsignadapter.New(pdfsignSvc, pdfsignStore, s3Client, cfg.S3BucketSignedPDFs, cfg.FrontendBaseURL)

	// ---- router ----
	handler := api.NewRouter(api.Deps{
		Logger:          log,
		Providers:       providers,
		Children:        children,
		Staff:           staff,
		Documents:       docs,
		Dashboard:       dash,
		Portal:          portal,
		Billing:         billH,
		BillingV2:       billV2,
		Auth:            authH,
		Legal:           legalH,
		StripeWebhook:   stripeWH,
		SESWebhook:      sesWH,
		TwilioWebhook:   twilioWH,
		Drills:          drills,
		Postings:        postings,
		Ratio:           ratio,
		Inspections:     inspections,
		InspectionMode:  inspectionMode,
		AuditLog:        auditLog,
		DataExport:      dataExport,
		Contact:         contact,
		Nag:             nag,
		Facility:        facility,
		UploadLink:      uploadLink,
		Score:           scoreH,
		Demo:            demoH,
		Kiosk:           kioskH,
		Attendance:      attendanceH,
		TestHelpers:     testHelpers,
		Magic:           magic,
		Session:         providers,
		RoleLookup:      mw.PoolRoleLookup{DB: pool},
		BillingChecker:  bill,
		RateLimit: func() *mw.TokenBucket {
			// Tests need a much more generous bucket — every loginAs() burns
			// two auth-bucket tokens (signup + callback) and individual specs
			// can easily fire 20 logins back-to-back.
			if strings.EqualFold(cfg.AppEnv, "test") {
				return mw.NewTokenBucket(10_000, 1_000)
			}
			return mw.NewTokenBucket(10, 0.5) // 10 burst, 0.5 tok/sec
		}(),
		FrontendOrigins: parseOrigins(cfg.FrontendBaseURL),
		PDFSign:         pdfsignRoutes,
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           handler,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("server listening", "addr", srv.Addr, "env", cfg.AppEnv)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-rootCtx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		return fmt.Errorf("listen: %w", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	log.Info("server shut down cleanly")
	return nil
}

func parseOrigins(raw string) []string {
	if raw == "" {
		return nil
	}
	out := []string{}
	for _, o := range strings.Split(raw, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			out = append(out, o)
		}
	}
	return out
}

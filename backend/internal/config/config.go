package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	AppEnv              string `env:"APP_ENV" envDefault:"development"`
	Port                int    `env:"PORT" envDefault:"8080"`
	AppBaseURL          string `env:"APP_BASE_URL,required"`
	FrontendBaseURL     string `env:"FRONTEND_BASE_URL,required"`
	LogLevel            string `env:"LOG_LEVEL" envDefault:"info"`
	SessionCookieDomain string `env:"SESSION_COOKIE_DOMAIN" envDefault:"localhost"`

	DatabaseURL string `env:"DATABASE_URL,required"`

	AWSRegion          string `env:"AWS_REGION" envDefault:"us-east-1"`
	AWSAccessKeyID     string `env:"AWS_ACCESS_KEY_ID"`
	AWSSecretAccessKey string `env:"AWS_SECRET_ACCESS_KEY"`
	AWSEndpointURL     string `env:"AWS_ENDPOINT_URL_S3"` // for local MinIO

	S3BucketDocuments  string `env:"S3_BUCKET_DOCUMENTS,required"`
	S3BucketSignedPDFs string `env:"S3_BUCKET_SIGNED_PDFS,required"`
	S3BucketAuditTrail string `env:"S3_BUCKET_AUDIT_TRAIL,required"`
	S3BucketRawUploads string `env:"S3_BUCKET_RAW_UPLOADS,required"`
	SESFromEmail       string `env:"SES_FROM_EMAIL,required"`

	StripeSecretKey     string `env:"STRIPE_SECRET_KEY,required"`
	StripeWebhookSecret string `env:"STRIPE_WEBHOOK_SECRET,required"`
	StripePricePro      string `env:"STRIPE_PRICE_PRO,required"`

	TwilioAccountSID string `env:"TWILIO_ACCOUNT_SID"`
	TwilioAuthToken  string `env:"TWILIO_AUTH_TOKEN"`
	TwilioFromNumber string `env:"TWILIO_FROM_NUMBER"`

	MistralAPIKey string `env:"MISTRAL_API_KEY"`
	GeminiAPIKey  string `env:"GEMINI_API_KEY"`

	MagicLinkSigningKey string `env:"MAGIC_LINK_SIGNING_KEY,required"`
}

func Load() (*Config, error) {
	var c Config
	if err := env.Parse(&c); err != nil {
		return nil, fmt.Errorf("config: parse env: %w", err)
	}
	if len(c.MagicLinkSigningKey) < 32 {
		return nil, fmt.Errorf("config: MAGIC_LINK_SIGNING_KEY must be >= 32 bytes")
	}
	if !strings.HasPrefix(c.AppBaseURL, "http") {
		return nil, fmt.Errorf("config: APP_BASE_URL must be a full URL")
	}
	return &c, nil
}

func (c *Config) SlogLevel() slog.Level {
	switch strings.ToLower(c.LogLevel) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func (c *Config) IsProduction() bool {
	return strings.EqualFold(c.AppEnv, "production")
}

func NewLogger(c *Config) *slog.Logger {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: c.SlogLevel()})
	return slog.New(h).With("app", "compliancekit-api", "env", c.AppEnv)
}

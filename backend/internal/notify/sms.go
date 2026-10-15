package notify

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/twilio/twilio-go"
	twilioapi "github.com/twilio/twilio-go/rest/api/v2010"
)

type SMSSender struct {
	client     *twilio.RestClient
	from       string // E.164 sender — used when not empty
	serviceSID string // Twilio Messaging Service SID — used when From is empty
	sink       *slog.Logger
}

// NewLogSMSSender returns a sender that logs instead of calling Twilio and
// reports success with a synthetic SID. Used by the public demo.
func NewLogSMSSender(log *slog.Logger) *SMSSender {
	if log == nil {
		log = slog.Default()
	}
	return &SMSSender{sink: log}
}

type SMSConfig struct {
	AccountSID        string
	AuthToken         string
	From              string // E.164. Optional if MessagingServiceSID is set.
	MessagingServiceSID string // MGxxxx... Optional alternative to From; required for trial accounts that don't own a phone number.
}

func NewSMSSender(cfg SMSConfig) *SMSSender {
	if cfg.AccountSID == "" || cfg.AuthToken == "" {
		return &SMSSender{from: cfg.From, serviceSID: cfg.MessagingServiceSID}
	}
	client := twilio.NewRestClientWithParams(twilio.ClientParams{
		Username: cfg.AccountSID,
		Password: cfg.AuthToken,
	})
	return &SMSSender{client: client, from: cfg.From, serviceSID: cfg.MessagingServiceSID}
}

// Send sends a single SMS. Returns the provider message SID on success.
func (s *SMSSender) Send(to, body string) (string, error) {
	if s != nil && s.sink != nil {
		s.sink.Info("sms suppressed (outbound disabled)", "to", to, "chars", len(body))
		return fmt.Sprintf("SMdemo%d", time.Now().UnixNano()), nil
	}
	if s == nil || s.client == nil {
		return "", fmt.Errorf("notify: sms client not initialized")
	}
	if to == "" || body == "" {
		return "", fmt.Errorf("notify: sms missing required fields")
	}
	if s.from == "" && s.serviceSID == "" {
		return "", fmt.Errorf("notify: sms requires From number or MessagingServiceSID")
	}
	params := &twilioapi.CreateMessageParams{}
	params.SetTo(to)
	if s.serviceSID != "" {
		params.SetMessagingServiceSid(s.serviceSID)
	} else {
		params.SetFrom(s.from)
	}
	params.SetBody(body)
	resp, err := s.client.Api.CreateMessage(params)
	if err != nil {
		return "", fmt.Errorf("notify: twilio send: %w", err)
	}
	if resp.Sid == nil {
		return "", nil
	}
	return *resp.Sid, nil
}

// ---- SMS templates (kept short — 160 chars ideal) ----

func SMSMagicLink(action, url string) string {
	return truncate(fmt.Sprintf("ComplianceKit %s: %s (expires soon)", action, url), 320)
}

func SMSChaseReminder(docTitle string, daysUntil int, url string) string {
	return truncate(fmt.Sprintf("ComplianceKit: %s expires in %d days. Upload: %s", docTitle, daysUntil, url), 320)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return strings.TrimSpace(s[:max-1]) + "\u2026"
}

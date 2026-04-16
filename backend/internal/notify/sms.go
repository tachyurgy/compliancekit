package notify

import (
	"fmt"
	"strings"

	"github.com/twilio/twilio-go"
	twilioapi "github.com/twilio/twilio-go/rest/api/v2010"
)

type SMSSender struct {
	client *twilio.RestClient
	from   string
}

type SMSConfig struct {
	AccountSID string
	AuthToken  string
	From       string
}

func NewSMSSender(cfg SMSConfig) *SMSSender {
	if cfg.AccountSID == "" || cfg.AuthToken == "" {
		return &SMSSender{from: cfg.From}
	}
	client := twilio.NewRestClientWithParams(twilio.ClientParams{
		Username: cfg.AccountSID,
		Password: cfg.AuthToken,
	})
	return &SMSSender{client: client, from: cfg.From}
}

// Send sends a single SMS. Returns the provider message SID on success.
func (s *SMSSender) Send(to, body string) (string, error) {
	if s == nil || s.client == nil {
		return "", fmt.Errorf("notify: sms client not initialized")
	}
	if to == "" || body == "" {
		return "", fmt.Errorf("notify: sms missing required fields")
	}
	params := &twilioapi.CreateMessageParams{}
	params.SetTo(to)
	params.SetFrom(s.from)
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

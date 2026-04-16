package notify

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	sestypes "github.com/aws/aws-sdk-go-v2/service/ses/types"
)

type Emailer struct {
	ses  *ses.Client
	from string
}

type EmailerConfig struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	From            string
}

func NewEmailer(ctx context.Context, cfg EmailerConfig) (*Emailer, error) {
	opts := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(cfg.Region)}
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("notify: ses config: %w", err)
	}
	return &Emailer{ses: ses.NewFromConfig(awsCfg), from: cfg.From}, nil
}

type EmailMessage struct {
	To          string
	Subject     string
	HTMLBody    string
	PlainBody   string
	ReplyTo     string
	ReferenceID string // used as SES MessageTag for audit
}

func (e *Emailer) Send(ctx context.Context, m EmailMessage) error {
	if e == nil || e.ses == nil {
		return fmt.Errorf("notify: emailer not initialized")
	}
	if m.To == "" || m.Subject == "" {
		return fmt.Errorf("notify: email missing required fields")
	}
	body := &sestypes.Body{}
	if m.HTMLBody != "" {
		body.Html = &sestypes.Content{Charset: aws.String("UTF-8"), Data: aws.String(m.HTMLBody)}
	}
	if m.PlainBody != "" {
		body.Text = &sestypes.Content{Charset: aws.String("UTF-8"), Data: aws.String(m.PlainBody)}
	}
	in := &ses.SendEmailInput{
		Source:      aws.String(e.from),
		Destination: &sestypes.Destination{ToAddresses: []string{m.To}},
		Message: &sestypes.Message{
			Subject: &sestypes.Content{Charset: aws.String("UTF-8"), Data: aws.String(m.Subject)},
			Body:    body,
		},
	}
	if m.ReplyTo != "" {
		in.ReplyToAddresses = []string{m.ReplyTo}
	}
	if m.ReferenceID != "" {
		in.Tags = []sestypes.MessageTag{{Name: aws.String("ref"), Value: aws.String(m.ReferenceID)}}
	}
	if _, err := e.ses.SendEmail(ctx, in); err != nil {
		return fmt.Errorf("notify: ses send: %w", err)
	}
	return nil
}

// ---- templates ----

type MagicLinkEmailData struct {
	RecipientName string
	ActionText    string // "Sign in to ComplianceKit"
	URL           string
	ExpiresIn     string // "15 minutes" or "7 days"
}

func RenderMagicLinkEmail(d MagicLinkEmailData) (subject, html, text string) {
	subject = "Your ComplianceKit link"
	html = strings.TrimSpace(fmt.Sprintf(`
<!doctype html>
<html><body style="font-family:system-ui,Segoe UI,Arial,sans-serif;color:#111">
<p>Hi %s,</p>
<p><a href="%s" style="background:#111;color:#fff;padding:12px 20px;border-radius:8px;text-decoration:none;">%s</a></p>
<p>This link expires in %s. If you didn't request it, ignore this email.</p>
<p style="color:#666;font-size:12px;">ComplianceKit &mdash; be inspection-ready every single day.</p>
</body></html>`, htmlEscape(d.RecipientName), d.URL, htmlEscape(d.ActionText), htmlEscape(d.ExpiresIn)))
	text = fmt.Sprintf("Hi %s,\n\n%s: %s\n\nThis link expires in %s.\n\nComplianceKit",
		d.RecipientName, d.ActionText, d.URL, d.ExpiresIn)
	return
}

type ChaseEmailData struct {
	RecipientName   string
	ChildOrStaff    string // "your child, Maya" or "you"
	DocTitle        string
	ExpiresOn       string
	DaysUntil       int
	UploadURL       string
	ProviderName    string
	ProviderContact string
}

// RenderChaseEmail produces the expiration reminder at a given threshold.
func RenderChaseEmail(d ChaseEmailData) (subject, html, text string) {
	var tone string
	switch {
	case d.DaysUntil <= 3:
		tone = "URGENT: "
	case d.DaysUntil <= 7:
		tone = "Reminder: "
	default:
		tone = ""
	}
	subject = fmt.Sprintf("%s%s expires in %d days", tone, d.DocTitle, d.DaysUntil)
	html = strings.TrimSpace(fmt.Sprintf(`
<!doctype html>
<html><body style="font-family:system-ui,Segoe UI,Arial,sans-serif;color:#111">
<p>Hi %s,</p>
<p>%s%s expires on <strong>%s</strong> (in %d days).</p>
<p>To stay enrolled/in compliance, please upload a current copy.</p>
<p><a href="%s" style="background:#111;color:#fff;padding:12px 20px;border-radius:8px;text-decoration:none;">Upload now</a></p>
<p>Thanks,<br>%s<br>%s</p>
</body></html>`, htmlEscape(d.RecipientName), htmlEscape(d.ChildOrStaff), htmlEscape(d.DocTitle),
		htmlEscape(d.ExpiresOn), d.DaysUntil, d.UploadURL, htmlEscape(d.ProviderName), htmlEscape(d.ProviderContact)))
	text = fmt.Sprintf("Hi %s,\n\n%s%s expires on %s (in %d days). Upload: %s\n\n%s",
		d.RecipientName, d.ChildOrStaff, d.DocTitle, d.ExpiresOn, d.DaysUntil, d.UploadURL, d.ProviderName)
	return
}

func RenderWelcomeEmail(providerName string) (subject, html, text string) {
	subject = "Welcome to ComplianceKit"
	html = fmt.Sprintf(`<p>Welcome %s! Your account is ready. Be inspection-ready every single day.</p>`, htmlEscape(providerName))
	text = fmt.Sprintf("Welcome %s! Your account is ready.", providerName)
	return
}

func RenderReceiptEmail(providerName string, amountCents int64, periodEnd string) (subject, html, text string) {
	subject = "Your ComplianceKit receipt"
	dollars := float64(amountCents) / 100.0
	html = fmt.Sprintf(`<p>Hi %s,</p><p>Thanks — $%.2f paid. Next invoice: %s.</p>`, htmlEscape(providerName), dollars, htmlEscape(periodEnd))
	text = fmt.Sprintf("Hi %s, thanks — $%.2f paid. Next invoice: %s.", providerName, dollars, periodEnd)
	return
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")
	return r.Replace(s)
}

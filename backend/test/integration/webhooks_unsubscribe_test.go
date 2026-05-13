package integration

// REQ-007: integration coverage for the SES bounce + Twilio STOP webhooks.
//
// Each test posts a representative payload at the real chi router and
// asserts the `unsubscribes` row landed (or didn't, for cases that should
// not opt-out — transient bounces, non-keyword inbound SMS). UNIQUE
// (channel, recipient) handles idempotency, so we also re-POST and assert
// no duplicate row.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// snsBody builds a {"Type":"Notification","Message":"<inner JSON as string>"}
// envelope. SNS double-encodes — Message is a string containing the SES JSON.
func snsBody(t *testing.T, inner any) []byte {
	t.Helper()
	innerJSON, err := json.Marshal(inner)
	if err != nil {
		t.Fatalf("encode inner: %v", err)
	}
	env := map[string]any{
		"Type":      "Notification",
		"MessageId": "test-msg-1",
		"TopicArn":  "arn:aws:sns:us-east-1:123:test",
		"Message":   string(innerJSON),
		"Timestamp": "2026-05-12T00:00:00Z",
	}
	out, _ := json.Marshal(env)
	return out
}

func TestSESWebhook_PermanentBounce_InsertsUnsubscribe(t *testing.T) {
	h := NewHarness(t)
	body := snsBody(t, map[string]any{
		"notificationType": "Bounce",
		"bounce": map[string]any{
			"bounceType": "Permanent",
			"bouncedRecipients": []map[string]any{
				{"emailAddress": "hardbounce@example.com"},
			},
		},
	})
	resp, err := http.Post(h.URL("/webhooks/ses/bounce"), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Row present?
	var n int
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM unsubscribes
		WHERE channel = 'email' AND recipient = 'hardbounce@example.com'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 unsubscribes row, got %d", n)
	}

	// Idempotency — second POST does not double up.
	resp2, _ := http.Post(h.URL("/webhooks/ses/bounce"), "application/json", bytes.NewReader(body))
	resp2.Body.Close()
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM unsubscribes
		WHERE channel = 'email' AND recipient = 'hardbounce@example.com'`).Scan(&n); err != nil {
		t.Fatalf("count2: %v", err)
	}
	if n != 1 {
		t.Fatalf("duplicate bounce must not double-insert; got %d", n)
	}
}

func TestSESWebhook_TransientBounce_NoUnsubscribe(t *testing.T) {
	h := NewHarness(t)
	body := snsBody(t, map[string]any{
		"notificationType": "Bounce",
		"bounce": map[string]any{
			"bounceType": "Transient",
			"bouncedRecipients": []map[string]any{
				{"emailAddress": "tempfail@example.com"},
			},
		},
	})
	resp, _ := http.Post(h.URL("/webhooks/ses/bounce"), "application/json", bytes.NewReader(body))
	resp.Body.Close()

	var n int
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM unsubscribes WHERE recipient = 'tempfail@example.com'`).Scan(&n)
	if n != 0 {
		t.Fatalf("transient bounce must NOT opt out; got %d rows", n)
	}
}

func TestSESWebhook_Complaint_OptsOut(t *testing.T) {
	h := NewHarness(t)
	body := snsBody(t, map[string]any{
		"notificationType": "Complaint",
		"complaint": map[string]any{
			"complainedRecipients": []map[string]any{
				{"emailAddress": "spamflag@example.com"},
			},
		},
	})
	resp, _ := http.Post(h.URL("/webhooks/ses/bounce"), "application/json", bytes.NewReader(body))
	resp.Body.Close()

	var reason string
	if err := h.DB.QueryRow(`SELECT reason FROM unsubscribes WHERE recipient = 'spamflag@example.com'`).Scan(&reason); err != nil {
		t.Fatalf("expected complaint to insert row: %v", err)
	}
	if reason != "ses_complaint" {
		t.Fatalf("reason = %q, want ses_complaint", reason)
	}
}

// TestSESWebhook_SubscriptionConfirmation_403: the legit SNS subscription was
// confirmed manually on 2026-05-12 and the handler now refuses any
// SubscriptionConfirmation that arrives through the bounce endpoint — either
// it's AWS replaying a confirmed message or somebody trying to trick us into
// subscribing to a hostile topic. Test pinned to 403 so the security choice
// can't silently regress to 200.
func TestSESWebhook_SubscriptionConfirmation_403(t *testing.T) {
	h := NewHarness(t)
	body := []byte(`{"Type":"SubscriptionConfirmation","TopicArn":"arn:x","SubscribeURL":"https://sns.example/confirm"}`)
	resp, _ := http.Post(h.URL("/webhooks/ses/bounce"), "application/json", bytes.NewReader(body))
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

// --- Twilio STOP ---

func twilioForm(from, body string) []byte {
	v := url.Values{}
	v.Set("From", from)
	v.Set("Body", body)
	v.Set("To", "+13055559999")
	v.Set("MessageSid", "SMtest")
	return []byte(v.Encode())
}

func postTwilio(t *testing.T, h *Harness, from, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, h.URL("/webhooks/twilio/sms"),
		bytes.NewReader(twilioForm(from, body)))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	return resp
}

func TestTwilioWebhook_STOP_InsertsUnsubscribe(t *testing.T) {
	h := NewHarness(t)
	resp := postTwilio(t, h, "+13055551212", "STOP")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	// TwiML body present so Twilio relays a confirmation reply.
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	if !strings.Contains(buf.String(), "<Response>") || !strings.Contains(buf.String(), "<Message>") {
		t.Fatalf("expected TwiML response, got %q", buf.String())
	}

	var reason string
	if err := h.DB.QueryRow(`SELECT reason FROM unsubscribes
		WHERE channel = 'sms' AND recipient = '+13055551212'`).Scan(&reason); err != nil {
		t.Fatalf("STOP did not insert unsubscribe: %v", err)
	}
	if reason != "twilio_stop" {
		t.Fatalf("reason = %q, want twilio_stop", reason)
	}
}

func TestTwilioWebhook_KeywordVariants(t *testing.T) {
	h := NewHarness(t)
	cases := []struct {
		from string
		body string
	}{
		{"+15551110001", "UNSUBSCRIBE"},
		{"+15551110002", "End"}, // case-insensitive
		{"+15551110003", "quit"},
		{"+15551110004", "CANCEL"},
	}
	for _, c := range cases {
		resp := postTwilio(t, h, c.from, c.body)
		resp.Body.Close()
		var n int
		if err := h.DB.QueryRow(`SELECT COUNT(*) FROM unsubscribes WHERE recipient = ?`, c.from).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", c.from, err)
		}
		if n != 1 {
			t.Errorf("body=%q from=%s: expected unsubscribe row, got %d", c.body, c.from, n)
		}
	}
}

func TestTwilioWebhook_START_ClearsUnsubscribe(t *testing.T) {
	h := NewHarness(t)
	from := "+15551112222"

	// Seed an opt-out, then post START to clear it.
	if _, err := h.DB.Exec(`INSERT INTO unsubscribes (id, channel, recipient, reason, created_at)
		VALUES ('preexisting', 'sms', ?, 'twilio_stop', CURRENT_TIMESTAMP)`, from); err != nil {
		t.Fatalf("seed: %v", err)
	}
	resp := postTwilio(t, h, from, "START")
	resp.Body.Close()

	var n int
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM unsubscribes WHERE recipient = ?`, from).Scan(&n)
	if n != 0 {
		t.Fatalf("START should clear opt-out; %d rows remain", n)
	}
}

func TestTwilioWebhook_NonKeyword_NoOptOut(t *testing.T) {
	h := NewHarness(t)
	resp := postTwilio(t, h, "+15558889999", "hello can you help me?")
	resp.Body.Close()
	var n int
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM unsubscribes WHERE recipient = '+15558889999'`).Scan(&n)
	if n != 0 {
		t.Fatalf("non-keyword inbound must not opt out; got %d rows", n)
	}
}

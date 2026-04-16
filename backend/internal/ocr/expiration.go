package ocr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ExpirationResult struct {
	ExpirationDate string  `json:"expiration_date"` // YYYY-MM-DD or ""
	Confidence     float64 `json:"confidence"`      // 0..1
	Reasoning      string  `json:"reasoning"`
}

// ExpirationExtractor asks Gemini Flash to find an expiration/valid-through date in OCR text
// and return a JSON object matching the response schema.
type ExpirationExtractor struct {
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

func NewExpirationExtractor(apiKey string) *ExpirationExtractor {
	return &ExpirationExtractor{
		APIKey:     apiKey,
		Model:      "gemini-2.0-flash",
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// ExtractExpiration is a free function wrapper for callers who don't want to hold the struct.
func ExtractExpiration(ctx context.Context, apiKey, ocrText, docType string) (*ExpirationResult, error) {
	return NewExpirationExtractor(apiKey).Extract(ctx, ocrText, docType)
}

func (e *ExpirationExtractor) Extract(ctx context.Context, ocrText, docType string) (*ExpirationResult, error) {
	if strings.TrimSpace(ocrText) == "" {
		return &ExpirationResult{Confidence: 0}, nil
	}
	prompt := fmt.Sprintf(`You are extracting the expiration date from a child-care compliance document.

DOCUMENT TYPE: %s

RULES:
- Return ONLY JSON matching the schema. No prose.
- If the document has a clear "expires", "expiration", "valid through", "valid until", or "next due" date, use that.
- For immunization records with "next due" dates per vaccine, choose the EARLIEST next-due date.
- For CPR/First Aid certifications, use the certification expiration date.
- If the document has only an "issued" date and the document type implies a standard duration (CPR=2 years, TB=1 year, background check=1 year in most states, physical exam=1 year for children), compute expiration = issued + duration. Note this in reasoning.
- If no expiration can be determined, return empty string and confidence 0.
- Confidence: 1.0 = explicit printed expiration; 0.7 = computed from issued+duration; 0.3 = best guess; 0.0 = unknown.

DOCUMENT TEXT:
---
%s
---`, docType, truncate(ocrText, 8000))

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"expiration_date": map[string]any{"type": "string"},
			"confidence":      map[string]any{"type": "number"},
			"reasoning":       map[string]any{"type": "string"},
		},
		"required": []string{"expiration_date", "confidence", "reasoning"},
	}

	reqBody := map[string]any{
		"contents": []map[string]any{{
			"parts": []map[string]any{{"text": prompt}},
		}},
		"generationConfig": map[string]any{
			"temperature":        0.0,
			"response_mime_type": "application/json",
			"response_schema":    schema,
		},
	}
	body, _ := json.Marshal(reqBody)
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", e.Model, e.APIKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ocr/expiration: new req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ocr/expiration: http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ocr/expiration: status %d: %s", resp.StatusCode, string(b))
	}
	var out struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("ocr/expiration: decode wrapper: %w", err)
	}
	if len(out.Candidates) == 0 || len(out.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("ocr/expiration: no candidates")
	}
	raw := out.Candidates[0].Content.Parts[0].Text
	// response_mime_type guarantees JSON, but be defensive.
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	var er ExpirationResult
	if err := json.Unmarshal([]byte(raw), &er); err != nil {
		return nil, fmt.Errorf("ocr/expiration: decode inner: %w (raw=%q)", err, raw)
	}
	// Sanity-check the date
	if er.ExpirationDate != "" {
		if _, err := time.Parse("2006-01-02", er.ExpirationDate); err != nil {
			er.ExpirationDate = ""
			er.Confidence = 0
		}
	}
	return &er, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...[truncated]"
}

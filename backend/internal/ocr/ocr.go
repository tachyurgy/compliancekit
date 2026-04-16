package ocr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Result is the normalized output of any OCR backend.
type Result struct {
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"` // 0..1
	Source     string  `json:"source"`     // "mistral" | "gemini" | "chain"
	Pages      int     `json:"pages,omitempty"`
}

// OCR abstracts any document OCR engine.
type OCR interface {
	Extract(ctx context.Context, r io.Reader, mimeType string) (Result, error)
}

// Chain tries primary; falls back to fallback on error OR if confidence < threshold.
// threshold of 0 disables the confidence check (only errors fall through).
func Chain(primary, fallback OCR, threshold float64) OCR {
	return &chained{primary: primary, fallback: fallback, threshold: threshold}
}

type chained struct {
	primary   OCR
	fallback  OCR
	threshold float64
}

func (c *chained) Extract(ctx context.Context, r io.Reader, mimeType string) (Result, error) {
	// buffer so we can read twice
	buf, err := io.ReadAll(r)
	if err != nil {
		return Result{}, fmt.Errorf("ocr: buffer body: %w", err)
	}
	res, err := c.primary.Extract(ctx, bytes.NewReader(buf), mimeType)
	if err == nil && res.Confidence >= c.threshold {
		res.Source = "chain:" + res.Source
		return res, nil
	}
	res2, err2 := c.fallback.Extract(ctx, bytes.NewReader(buf), mimeType)
	if err2 != nil {
		if err != nil {
			return Result{}, fmt.Errorf("ocr: primary=%v, fallback=%w", err, err2)
		}
		return res, nil // primary returned low-confidence but fallback failed
	}
	res2.Source = "chain:" + res2.Source
	return res2, nil
}

// ---- Mistral implementation ----

type Mistral struct {
	APIKey     string
	HTTPClient *http.Client
}

func NewMistral(apiKey string) *Mistral {
	return &Mistral{APIKey: apiKey, HTTPClient: &http.Client{Timeout: 60 * time.Second}}
}

func (m *Mistral) Extract(ctx context.Context, r io.Reader, mimeType string) (Result, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Result{}, fmt.Errorf("ocr/mistral: read body: %w", err)
	}
	b64 := base64.StdEncoding.EncodeToString(data)

	// Mistral OCR API: https://api.mistral.ai/v1/ocr
	reqBody := map[string]any{
		"model": "mistral-ocr-latest",
		"document": map[string]any{
			"type":          "document_base64",
			"document_name": "upload." + extFromMIME(mimeType),
			"document_base64": "data:" + mimeType + ";base64," + b64,
		},
		"include_image_base64": false,
	}
	body, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.mistral.ai/v1/ocr", bytes.NewReader(body))
	if err != nil {
		return Result{}, fmt.Errorf("ocr/mistral: new req: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.HTTPClient.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("ocr/mistral: http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return Result{}, fmt.Errorf("ocr/mistral: status %d: %s", resp.StatusCode, string(b))
	}
	var out struct {
		Pages []struct {
			Markdown string `json:"markdown"`
			Text     string `json:"text"`
		} `json:"pages"`
		UsageInfo struct {
			PagesProcessed int `json:"pages_processed"`
		} `json:"usage_info"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Result{}, fmt.Errorf("ocr/mistral: decode: %w", err)
	}
	var text bytes.Buffer
	for _, p := range out.Pages {
		if p.Markdown != "" {
			text.WriteString(p.Markdown)
		} else {
			text.WriteString(p.Text)
		}
		text.WriteByte('\n')
	}
	conf := 0.9
	if text.Len() == 0 {
		conf = 0.0
	}
	return Result{Text: text.String(), Confidence: conf, Source: "mistral", Pages: out.UsageInfo.PagesProcessed}, nil
}

// ---- Gemini implementation ----

type Gemini struct {
	APIKey     string
	Model      string // default gemini-2.0-flash
	HTTPClient *http.Client
}

func NewGemini(apiKey string) *Gemini {
	return &Gemini{APIKey: apiKey, Model: "gemini-2.0-flash", HTTPClient: &http.Client{Timeout: 60 * time.Second}}
}

func (g *Gemini) Extract(ctx context.Context, r io.Reader, mimeType string) (Result, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Result{}, fmt.Errorf("ocr/gemini: read body: %w", err)
	}
	b64 := base64.StdEncoding.EncodeToString(data)

	reqBody := map[string]any{
		"contents": []map[string]any{{
			"parts": []map[string]any{
				{"text": "Extract all readable text from this document. Output plain text only, preserving line breaks."},
				{"inline_data": map[string]any{"mime_type": mimeType, "data": b64}},
			},
		}},
		"generationConfig": map[string]any{
			"temperature":     0.0,
			"maxOutputTokens": 8192,
		},
	}
	body, _ := json.Marshal(reqBody)
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", g.Model, g.APIKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Result{}, fmt.Errorf("ocr/gemini: new req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.HTTPClient.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("ocr/gemini: http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return Result{}, fmt.Errorf("ocr/gemini: status %d: %s", resp.StatusCode, string(b))
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
		return Result{}, fmt.Errorf("ocr/gemini: decode: %w", err)
	}
	if len(out.Candidates) == 0 || len(out.Candidates[0].Content.Parts) == 0 {
		return Result{}, errors.New("ocr/gemini: no candidates")
	}
	var text bytes.Buffer
	for _, p := range out.Candidates[0].Content.Parts {
		text.WriteString(p.Text)
	}
	conf := 0.85
	if text.Len() == 0 {
		conf = 0.0
	}
	return Result{Text: text.String(), Confidence: conf, Source: "gemini"}, nil
}

func extFromMIME(m string) string {
	switch m {
	case "application/pdf":
		return "pdf"
	case "image/png":
		return "png"
	case "image/jpeg", "image/jpg":
		return "jpg"
	case "image/heic":
		return "heic"
	default:
		return "bin"
	}
}

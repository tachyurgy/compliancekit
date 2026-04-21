package ocr

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// fakeOCR is a minimal OCR implementation for chain testing.
type fakeOCR struct {
	res    Result
	err    error
	calls  int
	source string
}

func (f *fakeOCR) Extract(ctx context.Context, r io.Reader, mime string) (Result, error) {
	f.calls++
	// Drain the reader so we exercise buffer handling.
	_, _ = io.ReadAll(r)
	if f.err != nil {
		return Result{}, f.err
	}
	res := f.res
	if res.Source == "" {
		res.Source = f.source
	}
	return res, nil
}

func TestChain_PrimaryHighConfidence_FallbackNotCalled(t *testing.T) {
	t.Parallel()
	primary := &fakeOCR{res: Result{Text: "hi", Confidence: 0.95, Source: "mistral"}}
	fallback := &fakeOCR{res: Result{Text: "bye", Confidence: 0.90, Source: "gemini"}}
	chain := Chain(primary, fallback, 0.80)

	res, err := chain.Extract(context.Background(), bytes.NewReader([]byte("data")), "application/pdf")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if res.Text != "hi" {
		t.Fatalf("got text %q, want hi", res.Text)
	}
	if fallback.calls != 0 {
		t.Fatalf("fallback called %d times; should be 0", fallback.calls)
	}
	if !strings.HasPrefix(res.Source, "chain:") {
		t.Fatalf("source = %q; want chain:* prefix", res.Source)
	}
}

func TestChain_PrimaryLowConfidence_FallbackCalled(t *testing.T) {
	t.Parallel()
	primary := &fakeOCR{res: Result{Text: "bad", Confidence: 0.3, Source: "mistral"}}
	fallback := &fakeOCR{res: Result{Text: "good", Confidence: 0.9, Source: "gemini"}}
	chain := Chain(primary, fallback, 0.80)

	res, err := chain.Extract(context.Background(), bytes.NewReader([]byte("data")), "application/pdf")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if res.Text != "good" {
		t.Fatalf("got text %q, want good", res.Text)
	}
	if fallback.calls != 1 {
		t.Fatalf("fallback calls = %d, want 1", fallback.calls)
	}
}

func TestChain_PrimaryError_FallbackCalled(t *testing.T) {
	t.Parallel()
	primary := &fakeOCR{err: errors.New("mistral 500")}
	fallback := &fakeOCR{res: Result{Text: "rescued", Confidence: 0.9, Source: "gemini"}}
	chain := Chain(primary, fallback, 0.80)

	res, err := chain.Extract(context.Background(), bytes.NewReader([]byte("data")), "application/pdf")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if res.Text != "rescued" {
		t.Fatalf("got text %q, want rescued", res.Text)
	}
	if fallback.calls != 1 {
		t.Fatalf("fallback calls = %d, want 1", fallback.calls)
	}
}

func TestChain_BothFail_ReturnsError(t *testing.T) {
	t.Parallel()
	primary := &fakeOCR{err: errors.New("mistral down")}
	fallback := &fakeOCR{err: errors.New("gemini down")}
	chain := Chain(primary, fallback, 0.80)

	_, err := chain.Extract(context.Background(), bytes.NewReader([]byte("data")), "application/pdf")
	if err == nil {
		t.Fatal("expected error when both fail")
	}
	// The error string should surface both failures.
	if !strings.Contains(err.Error(), "mistral") || !strings.Contains(err.Error(), "gemini") {
		t.Fatalf("error should mention both failures: %v", err)
	}
}

func TestChain_PrimaryLowConf_FallbackFails_ReturnsPrimary(t *testing.T) {
	t.Parallel()
	primary := &fakeOCR{res: Result{Text: "mediocre", Confidence: 0.3, Source: "mistral"}}
	fallback := &fakeOCR{err: errors.New("gemini down")}
	chain := Chain(primary, fallback, 0.80)

	res, err := chain.Extract(context.Background(), bytes.NewReader([]byte("data")), "application/pdf")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if res.Text != "mediocre" {
		t.Fatalf("expected primary result as fallback for when fallback errors: got %q", res.Text)
	}
}

func TestChain_ThresholdBoundary_EqualToThresholdUsesPrimary(t *testing.T) {
	t.Parallel()
	// confidence == threshold → primary is accepted (>= comparison in Chain).
	primary := &fakeOCR{res: Result{Text: "boundary", Confidence: 0.80, Source: "mistral"}}
	fallback := &fakeOCR{res: Result{Text: "different", Confidence: 0.9, Source: "gemini"}}
	chain := Chain(primary, fallback, 0.80)

	res, err := chain.Extract(context.Background(), bytes.NewReader([]byte("data")), "application/pdf")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if res.Text != "boundary" {
		t.Fatalf("threshold boundary: got %q, want boundary", res.Text)
	}
	if fallback.calls != 0 {
		t.Fatalf("fallback should not be called at threshold equality")
	}
}

func TestChain_ZeroThreshold_OnlyErrorsFallThrough(t *testing.T) {
	t.Parallel()
	primary := &fakeOCR{res: Result{Text: "any", Confidence: 0.0, Source: "mistral"}}
	fallback := &fakeOCR{res: Result{Text: "other", Confidence: 0.9, Source: "gemini"}}
	chain := Chain(primary, fallback, 0)

	res, err := chain.Extract(context.Background(), bytes.NewReader([]byte("data")), "application/pdf")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	// Threshold 0 → any primary result (err == nil, confidence >= 0) is accepted.
	if res.Text != "any" {
		t.Fatalf("got %q, want any", res.Text)
	}
	if fallback.calls != 0 {
		t.Fatalf("fallback should be skipped at threshold=0 when primary returns no error")
	}
}

func TestExtFromMIME(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"application/pdf": "pdf",
		"image/png":       "png",
		"image/jpeg":      "jpg",
		"image/jpg":       "jpg",
		"image/heic":      "heic",
		"application/zip": "bin",
		"":                "bin",
	}
	for mime, want := range cases {
		if got := extFromMIME(mime); got != want {
			t.Errorf("extFromMIME(%q) = %q, want %q", mime, got, want)
		}
	}
}

// ---- ExpirationExtractor sanity checks (no network) ----

func TestExpirationExtractor_EmptyInput_ReturnsZeroResult(t *testing.T) {
	t.Parallel()
	e := NewExpirationExtractor("fake-key")
	r, err := e.Extract(context.Background(), "", "immunization_record")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if r == nil {
		t.Fatal("nil result")
	}
	if r.Confidence != 0 {
		t.Fatalf("confidence = %v, want 0", r.Confidence)
	}
	if r.ExpirationDate != "" {
		t.Fatalf("expected empty date, got %q", r.ExpirationDate)
	}
}

func TestExpirationExtractor_WhitespaceInput_ReturnsZeroResult(t *testing.T) {
	t.Parallel()
	e := NewExpirationExtractor("fake-key")
	r, err := e.Extract(context.Background(), "   \t\n  ", "immunization_record")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if r.Confidence != 0 {
		t.Fatalf("confidence = %v, want 0", r.Confidence)
	}
}

func TestTruncate_ShortInputUnchanged(t *testing.T) {
	t.Parallel()
	got := truncate("hello", 100)
	if got != "hello" {
		t.Fatalf("got %q, want hello", got)
	}
}

func TestTruncate_LongInputAppendsMarker(t *testing.T) {
	t.Parallel()
	in := strings.Repeat("x", 200)
	got := truncate(in, 50)
	if !strings.HasSuffix(got, "...[truncated]") {
		t.Fatalf("expected truncation marker suffix, got %q", got[len(got)-20:])
	}
}

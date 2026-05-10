package handlers

import (
	"testing"
	"time"
)

// Table-driven tests for classifyDoc — the riskiest piece of REQ-004's
// rollup logic. Each row pins down one of the five buckets defined in
// brand-voice.md.
func TestClassifyDoc(t *testing.T) {
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)

	mkTime := func(y int, m time.Month, d int) *time.Time {
		t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		return &t
	}

	cases := []struct {
		name string
		in   statusInputs
		want DocStatus
	}{
		{
			name: "no doc, no nag → missing",
			in:   statusInputs{now: now, hasDoc: false, expiryWindowD: 90},
			want: StatusMissing,
		},
		{
			name: "no doc, recent nag → awaiting_upload",
			in: statusInputs{
				now: now, hasDoc: false,
				pendingNagAt:  mkTime(2026, 5, 1), // 8 days ago
				expiryWindowD: 90,
			},
			want: StatusAwaitingUpload,
		},
		{
			name: "no doc, stale nag → missing",
			in: statusInputs{
				now: now, hasDoc: false,
				pendingNagAt:  mkTime(2026, 1, 1), // ~4 months ago
				expiryWindowD: 90,
			},
			want: StatusMissing,
		},
		{
			name: "doc on file, no expiry → on_file",
			in: statusInputs{
				now: now, hasDoc: true,
				doc:           docRow{},
				expiryWindowD: 90,
			},
			want: StatusOnFile,
		},
		{
			name: "doc on file, expires next year → on_file",
			in: statusInputs{
				now: now, hasDoc: true,
				doc:           docRow{ExpiresAt: mkTime(2027, 12, 1)},
				expiryWindowD: 90,
			},
			want: StatusOnFile,
		},
		{
			name: "doc on file, expires in 30 days → expiring_soon",
			in: statusInputs{
				now: now, hasDoc: true,
				doc:           docRow{ExpiresAt: mkTime(2026, 6, 8)},
				expiryWindowD: 90,
			},
			want: StatusExpiringSoon,
		},
		{
			// Boundary: doc expires at 00:00 today, now is later in day →
			// "after expiry" by clock-on-the-wall, so expired. We document
			// this so the rule isn't surprising when an operator opens the
			// app at 9am on the expiry date.
			name: "doc expires today at 00:00, now midday → expired",
			in: statusInputs{
				now: now, hasDoc: true,
				doc:           docRow{ExpiresAt: mkTime(2026, 5, 9)},
				expiryWindowD: 90,
			},
			want: StatusExpired,
		},
		{
			name: "doc expires tomorrow → expiring_soon",
			in: statusInputs{
				now: now, hasDoc: true,
				doc:           docRow{ExpiresAt: mkTime(2026, 5, 10)},
				expiryWindowD: 90,
			},
			want: StatusExpiringSoon,
		},
		{
			name: "doc on file, expired yesterday → expired",
			in: statusInputs{
				now: now, hasDoc: true,
				doc:           docRow{ExpiresAt: mkTime(2026, 5, 8)},
				expiryWindowD: 90,
			},
			want: StatusExpired,
		},
		{
			name: "expiry exactly 90 days out → expiring_soon",
			in: statusInputs{
				now: now, hasDoc: true,
				doc:           docRow{ExpiresAt: mkTime(2026, 8, 7)}, // 90 days from May 9
				expiryWindowD: 90,
			},
			want: StatusExpiringSoon,
		},
		{
			name: "expiry 91 days out → on_file",
			in: statusInputs{
				now: now, hasDoc: true,
				doc:           docRow{ExpiresAt: mkTime(2026, 8, 9)},
				expiryWindowD: 90,
			},
			want: StatusOnFile,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyDoc(tc.in)
			if got != tc.want {
				t.Fatalf("classifyDoc(%+v): got %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Worst-case rollup picks the most-attention-grabbing pill. Order is
// expired > missing > expiring_soon > awaiting_upload > on_file.
func TestRollupCounts_worst(t *testing.T) {
	cases := []struct {
		name string
		rc   rollupCounts
		want DocStatus
	}{
		{name: "all on file", rc: rollupCounts{OnFile: 5}, want: StatusOnFile},
		{name: "one expired wins", rc: rollupCounts{OnFile: 4, Expired: 1}, want: StatusExpired},
		{name: "expired beats missing", rc: rollupCounts{Expired: 1, Missing: 3}, want: StatusExpired},
		{name: "missing beats expiring", rc: rollupCounts{Missing: 1, ExpiringSoon: 1}, want: StatusMissing},
		{name: "expiring beats awaiting", rc: rollupCounts{ExpiringSoon: 1, AwaitingUpload: 2}, want: StatusExpiringSoon},
		{name: "awaiting beats on_file", rc: rollupCounts{AwaitingUpload: 1, OnFile: 1}, want: StatusAwaitingUpload},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.rc.worst()
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// sourceLabel exercises the brand-voice mapping ("Scanned with
// Levelbrook AI" vs "Uploaded by you").
func TestSourceLabel(t *testing.T) {
	cases := []struct {
		in   docRow
		want string
	}{
		{in: docRow{OCRSource: "mistral"}, want: "Scanned with Levelbrook AI"},
		{in: docRow{OCRSource: "Gemini"}, want: "Scanned with Levelbrook AI"},
		{in: docRow{UploadedBy: "user_abc"}, want: "Uploaded by you"},
		{in: docRow{}, want: ""},
	}
	for i, tc := range cases {
		got := sourceLabel(tc.in)
		if got != tc.want {
			t.Fatalf("case %d: got %q, want %q", i, got, tc.want)
		}
	}
}

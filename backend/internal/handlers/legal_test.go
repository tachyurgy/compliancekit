package handlers

import (
	"strings"
	"testing"
)

// TestLegalDocs_Embedded confirms the //go:embed pulled the Markdown in. The
// docs are the source of truth for the e-signature audit trail; if they are
// empty the audit hash would silently be a constant, which would be very bad.
func TestLegalDocs_Embedded(t *testing.T) {
	t.Parallel()
	if !strings.Contains(tosMD, "Levelbrook Terms of Service") {
		t.Fatalf("tos.md was not embedded or has unexpected first heading; got %q...", first200(tosMD))
	}
	if !strings.Contains(dpaMD, "Data Processing Addendum") {
		t.Fatalf("dpa.md was not embedded or has unexpected first heading; got %q...", first200(dpaMD))
	}
	if len(tosMD) < 1500 {
		t.Fatalf("tos.md unexpectedly short: %d bytes", len(tosMD))
	}
	if len(dpaMD) < 1500 {
		t.Fatalf("dpa.md unexpectedly short: %d bytes", len(dpaMD))
	}
}

// TestLegalDocs_Hash_Stable confirms the SHA-256 of the embedded Markdown is
// stable across calls and distinct between docs. This is the core contract:
// every legal_signatures row records the hash of the doc the operator saw
// at sign time, so any drift between sign-time and audit-time is detectable.
func TestLegalDocs_Hash_Stable(t *testing.T) {
	t.Parallel()
	if sha256Hex(tosMD) != sha256Hex(tosMD) {
		t.Fatal("tos hash unstable across calls")
	}
	if sha256Hex(tosMD) == sha256Hex(dpaMD) {
		t.Fatal("tos and dpa hash collided")
	}
}

// TestLegalVersionLabels are static markers we bump when we publish a new
// document. The audit-trail table records both the SHA (truth) and the
// version (human label); the version must be present and non-empty.
func TestLegalVersionLabels(t *testing.T) {
	t.Parallel()
	if TosVersion == "" || DpaVersion == "" {
		t.Fatalf("legal version labels must be non-empty: tos=%q dpa=%q", TosVersion, DpaVersion)
	}
}

func first200(s string) string {
	if len(s) > 200 {
		return s[:200]
	}
	return s
}

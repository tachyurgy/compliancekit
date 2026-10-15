package storage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newLocalClient(t *testing.T, base string) *Client {
	t.Helper()
	c, err := New(context.Background(), Config{
		LocalDir:      t.TempDir(),
		PublicBaseURL: base,
		URLSigningKey: []byte(strings.Repeat("k", 32)),
		Buckets:       Buckets{Documents: "docs", SignedPDFs: "signed", AuditTrail: "audit", RawUploads: "raw"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLocalPresignedRoundTrip(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := newLocalClient(t, srv.URL)
	mux.Handle("/blob/", c.BlobHandler())
	ctx := context.Background()

	put, err := c.PresignPutURL(ctx, "raw", "providers/p1/doc one.pdf", "application/pdf", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPut, put, strings.NewReader("%PDF-1.4 hello"))
	req.Header.Set("Content-Type", "application/pdf")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("PUT: %v %v", err, resp)
	}

	get, _ := c.PresignGetURL(ctx, "raw", "providers/p1/doc one.pdf", time.Minute)
	resp, err = http.Get(get)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("GET: %v %v", err, resp)
	}
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "%PDF-1.4 hello" || resp.Header.Get("Content-Type") != "application/pdf" {
		t.Fatalf("got %q %q", b, resp.Header.Get("Content-Type"))
	}

	// A GET signature must not authorize a PUT, and tampering fails.
	req, _ = http.NewRequest(http.MethodPut, get, strings.NewReader("x"))
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("PUT with GET signature: %d", resp.StatusCode)
	}
	resp, _ = http.Get(strings.Replace(get, "doc%20one", "other", 1))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("tampered key: %d", resp.StatusCode)
	}

	keys, err := c.ListPrefix(ctx, "raw", "providers/p1/")
	if err != nil || len(keys) != 1 {
		t.Fatalf("list: %v %v", keys, err)
	}
	if err := c.DeleteAllForProvider(ctx, "p1"); err != nil {
		t.Fatal(err)
	}
	keys, _ = c.ListPrefix(ctx, "raw", "providers/p1/")
	if len(keys) != 0 {
		t.Fatalf("after delete: %v", keys)
	}
}

func TestLocalRejectsTraversal(t *testing.T) {
	c := newLocalClient(t, "http://x")
	if err := c.PutDocument(context.Background(), "../../etc/passwd", "text/plain", strings.NewReader("x")); err == nil {
		t.Fatal("expected traversal to be rejected")
	}
}

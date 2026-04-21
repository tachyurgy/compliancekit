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

// newTestClient constructs a storage.Client pointing at the given test
// HTTP endpoint, with predictable bucket names so we can assert on them.
func newTestClient(t *testing.T, endpoint string) *Client {
	t.Helper()
	c, err := New(context.Background(), Config{
		Region:          "us-east-1",
		AccessKeyID:     "test",
		SecretAccessKey: "test",
		EndpointURL:     endpoint,
		Buckets: Buckets{
			Documents:  "docs",
			SignedPDFs: "signed",
			AuditTrail: "audit",
			RawUploads: "raw",
		},
	})
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	return c
}

func TestNew_ExposesBuckets(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, "http://127.0.0.1:1") // invalid but client construction succeeds
	got := c.Buckets()
	if got.Documents != "docs" || got.SignedPDFs != "signed" || got.AuditTrail != "audit" || got.RawUploads != "raw" {
		t.Fatalf("Buckets mismatch: %+v", got)
	}
}

func TestNew_NoOverrideEndpoint(t *testing.T) {
	t.Parallel()
	c, err := New(context.Background(), Config{
		Region:          "us-west-2",
		AccessKeyID:     "a",
		SecretAccessKey: "b",
		Buckets:         Buckets{Documents: "d"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.Buckets().Documents != "d" {
		t.Fatal("Documents bucket mismatch")
	}
}

func TestPresignGetURL_ContainsKeyAndBucket(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, "http://s3.example.local:9000")
	url, err := c.PresignGetURL(context.Background(), "docs", "providers/p1/x.pdf", 5*time.Minute)
	if err != nil {
		t.Fatalf("PresignGetURL: %v", err)
	}
	if !strings.Contains(url, "providers/p1/x.pdf") {
		t.Fatalf("URL missing key: %s", url)
	}
	if !strings.Contains(url, "docs") {
		t.Fatalf("URL missing bucket: %s", url)
	}
	if !strings.Contains(url, "X-Amz-Signature") {
		t.Fatalf("URL missing signature: %s", url)
	}
}

func TestPresignGetURL_DefaultsToDocumentsBucket(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, "http://s3.example.local:9000")
	url, err := c.PresignGetURL(context.Background(), "", "some/key.txt", 1*time.Minute)
	if err != nil {
		t.Fatalf("PresignGetURL: %v", err)
	}
	if !strings.Contains(url, "docs") {
		t.Fatalf("expected docs bucket in URL: %s", url)
	}
}

func TestPresignPutURL_ContainsKey(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, "http://s3.example.local:9000")
	url, err := c.PresignPutURL(context.Background(), "raw", "providers/p1/upload.bin", "image/png", 5*time.Minute)
	if err != nil {
		t.Fatalf("PresignPutURL: %v", err)
	}
	if !strings.Contains(url, "providers/p1/upload.bin") {
		t.Fatalf("URL missing key: %s", url)
	}
}

func TestPresignPutURL_DefaultsToRawUploadsBucket(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, "http://s3.example.local:9000")
	url, err := c.PresignPutURL(context.Background(), "", "k", "text/plain", 1*time.Minute)
	if err != nil {
		t.Fatalf("PresignPutURL: %v", err)
	}
	if !strings.Contains(url, "raw") {
		t.Fatalf("expected raw bucket in URL: %s", url)
	}
}

// ---- Fake S3 HTTP server to exercise put/get/list/delete flow ----

// fakeS3 is the smallest imaginable in-memory S3 implementation: a map of
// (bucket, key) → bytes, plus XML wire formats for the four operations the
// production code actually uses.
type fakeS3 struct {
	objects map[string][]byte // bucket/key → bytes
}

func newFakeS3() *fakeS3 {
	return &fakeS3{objects: map[string][]byte{}}
}

func (f *fakeS3) mountInto(mux *http.ServeMux) {
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// path is /{bucket}/{key...} because of UsePathStyle=true
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
		if len(parts) == 0 {
			http.Error(w, "bad path", 400)
			return
		}
		bucket := parts[0]
		key := ""
		if len(parts) == 2 {
			key = parts[1]
		}
		switch r.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			f.objects[bucket+"/"+key] = body
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			// list or get
			if r.URL.Query().Get("list-type") == "2" {
				prefix := r.URL.Query().Get("prefix")
				var keys []string
				for k := range f.objects {
					if strings.HasPrefix(k, bucket+"/") {
						inner := strings.TrimPrefix(k, bucket+"/")
						if prefix == "" || strings.HasPrefix(inner, prefix) {
							keys = append(keys, inner)
						}
					}
				}
				w.Header().Set("Content-Type", "application/xml")
				w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
<IsTruncated>false</IsTruncated>`))
				for _, k := range keys {
					w.Write([]byte("<Contents><Key>" + k + "</Key></Contents>"))
				}
				w.Write([]byte(`</ListBucketResult>`))
				return
			}
			b, ok := f.objects[bucket+"/"+key]
			if !ok {
				http.Error(w, "not found", 404)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write(b)
		case http.MethodPost:
			// DeleteObjects uses POST with ?delete and XML body listing keys.
			if _, ok := r.URL.Query()["delete"]; ok {
				body, _ := io.ReadAll(r.Body)
				// Pull <Key>…</Key> substrings with a tiny handwritten scan.
				// Good enough for our tests.
				s := string(body)
				for {
					i := strings.Index(s, "<Key>")
					if i < 0 {
						break
					}
					j := strings.Index(s[i:], "</Key>")
					if j < 0 {
						break
					}
					key := s[i+len("<Key>") : i+j]
					delete(f.objects, bucket+"/"+key)
					s = s[i+j+len("</Key>"):]
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><DeleteResult/>`))
				return
			}
			http.Error(w, "unsupported", 400)
		default:
			http.Error(w, "method not allowed", 405)
		}
	})
}

func TestPutAndGetObject_Roundtrip(t *testing.T) {
	t.Parallel()
	fake := newFakeS3()
	mux := http.NewServeMux()
	fake.mountInto(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	ctx := context.Background()

	if err := c.PutDocument(ctx, "providers/p1/x.pdf", "application/pdf", strings.NewReader("hello")); err != nil {
		t.Fatalf("PutDocument: %v", err)
	}
	body, _, err := c.GetDocument(ctx, "providers/p1/x.pdf")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	defer body.Close()
	got, _ := io.ReadAll(body)
	if string(got) != "hello" {
		t.Fatalf("got %q, want hello", string(got))
	}
}

func TestPutAuditJSON_SerializesAndUploads(t *testing.T) {
	t.Parallel()
	fake := newFakeS3()
	mux := http.NewServeMux()
	fake.mountInto(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	payload := map[string]any{"provider_id": "p1", "note": "purge"}
	if err := c.PutAuditJSON(context.Background(), "audit/p1/del.json", payload); err != nil {
		t.Fatalf("PutAuditJSON: %v", err)
	}
	got := fake.objects["audit/audit/p1/del.json"]
	if !strings.Contains(string(got), `"provider_id":"p1"`) {
		t.Fatalf("unexpected upload: %s", string(got))
	}
}

func TestListPrefix_FiltersByPrefix(t *testing.T) {
	t.Parallel()
	fake := newFakeS3()
	fake.objects["docs/providers/p1/a.pdf"] = []byte("a")
	fake.objects["docs/providers/p1/b.pdf"] = []byte("b")
	fake.objects["docs/providers/p2/c.pdf"] = []byte("c")
	mux := http.NewServeMux()
	fake.mountInto(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	keys, err := c.ListPrefix(context.Background(), "docs", "providers/p1/")
	if err != nil {
		t.Fatalf("ListPrefix: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("keys = %v, want 2", keys)
	}
	for _, k := range keys {
		if !strings.HasPrefix(k, "providers/p1/") {
			t.Fatalf("unexpected key outside prefix: %s", k)
		}
	}
}

func TestDeleteAllForProvider_RemovesAcrossBuckets(t *testing.T) {
	t.Parallel()
	fake := newFakeS3()
	fake.objects["docs/providers/p1/a.pdf"] = []byte("a")
	fake.objects["signed/providers/p1/b.pdf"] = []byte("b")
	fake.objects["docs/providers/p2/keep.pdf"] = []byte("c")

	mux := http.NewServeMux()
	fake.mountInto(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	if err := c.DeleteAllForProvider(context.Background(), "p1"); err != nil {
		t.Fatalf("DeleteAllForProvider: %v", err)
	}
	// p1 objects should be gone; p2 preserved.
	if _, ok := fake.objects["docs/providers/p1/a.pdf"]; ok {
		t.Fatal("p1 object not deleted")
	}
	if _, ok := fake.objects["signed/providers/p1/b.pdf"]; ok {
		t.Fatal("signed p1 object not deleted")
	}
	if _, ok := fake.objects["docs/providers/p2/keep.pdf"]; !ok {
		t.Fatal("p2 object was wrongfully deleted")
	}
}

func TestPutDocument_UsesDocumentsBucket(t *testing.T) {
	t.Parallel()
	fake := newFakeS3()
	mux := http.NewServeMux()
	fake.mountInto(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	if err := c.PutDocument(context.Background(), "k", "text/plain", strings.NewReader("x")); err != nil {
		t.Fatalf("PutDocument: %v", err)
	}
	// key "k" landed under the docs bucket.
	if _, ok := fake.objects["docs/k"]; !ok {
		t.Fatalf("expected docs/k, got keys: %v", keysOf(fake.objects))
	}
}

func TestPutObject_TargetsGivenBucket(t *testing.T) {
	t.Parallel()
	fake := newFakeS3()
	mux := http.NewServeMux()
	fake.mountInto(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	if err := c.PutObject(context.Background(), "audit", "exports/p1/ts.zip", "application/zip", strings.NewReader("zip")); err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	if _, ok := fake.objects["audit/exports/p1/ts.zip"]; !ok {
		t.Fatalf("expected audit/exports/p1/ts.zip, keys: %v", keysOf(fake.objects))
	}
}

func keysOf(m map[string][]byte) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

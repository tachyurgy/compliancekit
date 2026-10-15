package storage

// local.go is a filesystem-backed implementation of the same surface the S3
// client exposes. It exists so a single small server (the public demo, a dev
// laptop) can run the whole product, including browser direct uploads and
// signed-PDF downloads, without an object store.
//
// The browser-facing contract is kept identical to S3: handlers still mint a
// "presigned" URL and the browser PUTs/GETs it directly. Here the URL points
// back at this server (/blob/<bucket>/<key>) and carries an HMAC signature and
// an expiry, so the BlobHandler can authorize the request without a session,
// exactly like an S3 presigned URL.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// maxBlobBytes caps a single browser upload through the blob handler.
const maxBlobBytes = 25 << 20

// ctypeSuffix marks the sidecar file that remembers an object's Content-Type.
const ctypeSuffix = ".__ctype"

type localFS struct {
	root       string
	publicBase string // e.g. https://compliancekit.levelbrook.com
	key        []byte
	buckets    map[string]bool
}

func newLocalFS(root, publicBase string, signingKey []byte, b Buckets) (*localFS, error) {
	if len(signingKey) < 32 {
		return nil, errors.New("storage: local mode needs a >=32 byte URL signing key")
	}
	l := &localFS{
		root:       root,
		publicBase: strings.TrimRight(publicBase, "/"),
		key:        signingKey,
		buckets:    map[string]bool{},
	}
	for _, name := range []string{b.Documents, b.SignedPDFs, b.AuditTrail, b.RawUploads} {
		if name == "" {
			continue
		}
		l.buckets[name] = true
		if err := os.MkdirAll(filepath.Join(root, name), 0o750); err != nil {
			return nil, fmt.Errorf("storage: mkdir %s: %w", name, err)
		}
	}
	return l, nil
}

// resolve maps bucket/key to a path under root, rejecting traversal.
func (l *localFS) resolve(bucket, key string) (string, error) {
	if !l.buckets[bucket] {
		return "", fmt.Errorf("storage: unknown bucket %q", bucket)
	}
	clean := path.Clean("/" + key)
	if clean == "/" || strings.Contains(key, "..") || strings.HasSuffix(clean, ctypeSuffix) {
		return "", fmt.Errorf("storage: invalid key %q", key)
	}
	return filepath.Join(l.root, bucket, filepath.FromSlash(strings.TrimPrefix(clean, "/"))), nil
}

func (l *localFS) put(bucket, key, mimeType string, body io.Reader) error {
	p, err := l.resolve(bucket, key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return fmt.Errorf("storage: mkdir: %w", err)
	}
	// Write to a temp file and rename so a reader never sees a half object.
	tmp, err := os.CreateTemp(filepath.Dir(p), ".upload-*")
	if err != nil {
		return fmt.Errorf("storage: temp: %w", err)
	}
	if _, err := io.Copy(tmp, body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return fmt.Errorf("storage: write %s/%s: %w", bucket, key, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if mimeType != "" {
		_ = os.WriteFile(p+ctypeSuffix, []byte(mimeType), 0o640)
	}
	return nil
}

func (l *localFS) get(bucket, key string) (io.ReadCloser, string, error) {
	p, err := l.resolve(bucket, key)
	if err != nil {
		return nil, "", err
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, "", fmt.Errorf("storage: get %s/%s: %w", bucket, key, err)
	}
	ct := ""
	if b, err := os.ReadFile(p + ctypeSuffix); err == nil {
		ct = string(b)
	} else {
		ct = mime.TypeByExtension(filepath.Ext(p))
	}
	return f, ct, nil
}

func (l *localFS) list(bucket, prefix string) ([]string, error) {
	if !l.buckets[bucket] {
		return nil, fmt.Errorf("storage: unknown bucket %q", bucket)
	}
	base := filepath.Join(l.root, bucket)
	var keys []string
	err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() || strings.HasSuffix(p, ctypeSuffix) || strings.HasPrefix(d.Name(), ".upload-") {
			return nil
		}
		rel, _ := filepath.Rel(base, p)
		k := filepath.ToSlash(rel)
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
		return nil
	})
	return keys, err
}

func (l *localFS) deletePrefix(bucket, prefix string) error {
	keys, err := l.list(bucket, prefix)
	if err != nil {
		return err
	}
	for _, k := range keys {
		p, err := l.resolve(bucket, k)
		if err != nil {
			continue
		}
		_ = os.Remove(p)
		_ = os.Remove(p + ctypeSuffix)
	}
	return nil
}

func (l *localFS) sign(method, bucket, key string, exp int64) string {
	m := hmac.New(sha256.New, l.key)
	fmt.Fprintf(m, "%s\n%s\n%s\n%d", method, bucket, key, exp)
	return hex.EncodeToString(m.Sum(nil))
}

func (l *localFS) presign(method, bucket, key string, ttl time.Duration) (string, error) {
	if _, err := l.resolve(bucket, key); err != nil {
		return "", err
	}
	exp := time.Now().Add(ttl).Unix()
	q := url.Values{}
	q.Set("exp", strconv.FormatInt(exp, 10))
	q.Set("sig", l.sign(method, bucket, key, exp))
	return fmt.Sprintf("%s/blob/%s/%s?%s", l.publicBase, url.PathEscape(bucket),
		escapeKey(key), q.Encode()), nil
}

func escapeKey(key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

// BlobHandler serves the presigned URLs minted in local mode. Mount it at
// /blob/. It returns 404 for every request when the client is in S3 mode.
func (c *Client) BlobHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c == nil || c.local == nil {
			http.NotFound(w, r)
			return
		}
		l := c.local
		rest := strings.TrimPrefix(r.URL.Path, "/blob/")
		bucket, key, ok := strings.Cut(rest, "/")
		if !ok || bucket == "" || key == "" {
			http.NotFound(w, r)
			return
		}
		exp, err := strconv.ParseInt(r.URL.Query().Get("exp"), 10, 64)
		if err != nil || time.Now().Unix() > exp {
			http.Error(w, "link expired", http.StatusForbidden)
			return
		}
		method := r.Method
		if method == http.MethodHead {
			method = http.MethodGet
		}
		want := l.sign(method, bucket, key, exp)
		if !hmac.Equal([]byte(want), []byte(r.URL.Query().Get("sig"))) {
			http.Error(w, "bad signature", http.StatusForbidden)
			return
		}
		switch r.Method {
		case http.MethodPut:
			body := http.MaxBytesReader(w, r.Body, maxBlobBytes)
			if err := l.put(bucket, key, r.Header.Get("Content-Type"), body); err != nil {
				http.Error(w, "upload failed", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
		case http.MethodGet, http.MethodHead:
			rc, ct, err := l.get(bucket, key)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			defer rc.Close()
			if ct != "" {
				w.Header().Set("Content-Type", ct)
			}
			w.Header().Set("Cache-Control", "private, max-age=60")
			if f, ok := rc.(*os.File); ok {
				if st, err := f.Stat(); err == nil {
					http.ServeContent(w, r, path.Base(key), st.ModTime(), f)
					return
				}
			}
			_, _ = io.Copy(w, rc)
		default:
			w.Header().Set("Allow", "GET, HEAD, PUT")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

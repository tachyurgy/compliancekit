package pdfsign

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// AuthContext pulls the authenticated provider out of the request context.
// Replace this with the project's real auth middleware integration.
type AuthContext interface {
	ProviderID(r *http.Request) (string, bool)
}

// Handlers bundles the HTTP routes for pdfsign. Mount with Register.
type Handlers struct {
	Svc        *Service
	Auth       AuthContext
	MaxPDFSize int64 // bytes; 0 = 25MB default
}

// Register attaches all routes to the given chi router under /api/pdfsign.
func (h *Handlers) Register(r chi.Router) {
	r.Route("/api/pdfsign", func(r chi.Router) {
		r.Post("/sessions", h.createSession)
		r.Get("/sessions/{token}", h.getSession)
		r.Post("/sessions/{token}/finalize", h.finalize)
		r.Get("/templates", h.listTemplates)
		r.Put("/templates/{id}/fields", h.saveTemplateFields)
	})
}

type createSessionRequest struct {
	DocumentTemplateID string  `json:"documentTemplateId"`
	SignerName         string  `json:"signerName"`
	SignerEmail        string  `json:"signerEmail"`
	SignerID           string  `json:"signerId,omitempty"`
	Fields             []Field `json:"fields"`
	ExpiresInHours     int     `json:"expiresInHours,omitempty"`
}

type createSessionResponse struct {
	Token      string `json:"token"`
	SigningURL string `json:"signingUrl"`
	ExpiresAt  string `json:"expiresAt"`
}

func (h *Handlers) createSession(w http.ResponseWriter, r *http.Request) {
	providerID, ok := h.Auth.ProviderID(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var req createSessionRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.DocumentTemplateID == "" || req.SignerEmail == "" || len(req.Fields) == 0 {
		writeErr(w, http.StatusBadRequest, "documentTemplateId, signerEmail, fields are required")
		return
	}
	in := CreateSessionInput{
		ProviderID:  providerID,
		DocumentID:  req.DocumentTemplateID,
		SignerID:    req.SignerID,
		SignerName:  req.SignerName,
		SignerEmail: req.SignerEmail,
		Fields:      assignFieldIDs(req.Fields),
	}
	if req.ExpiresInHours > 0 {
		in.ExpiresIn = time.Duration(req.ExpiresInHours) * time.Hour
	}
	sess, err := h.Svc.CreateSession(r.Context(), in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, createSessionResponse{
		Token:      sess.Token,
		SigningURL: fmt.Sprintf("%s/sign/%s", strings.TrimRight(h.Svc.signingBase, "/"), sess.Token),
		ExpiresAt:  sess.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (h *Handlers) getSession(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	sess, err := h.Svc.GetSession(r.Context(), token)
	if err != nil {
		writeSessionErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

func (h *Handlers) finalize(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")

	// The browser posts multipart/form-data with "pdf" and "audit" parts.
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "parse multipart: "+err.Error())
		return
	}
	pdfFile, _, err := r.FormFile("pdf")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "missing 'pdf' part: "+err.Error())
		return
	}
	defer pdfFile.Close()
	auditFile, _, err := r.FormFile("audit")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "missing 'audit' part: "+err.Error())
		return
	}
	defer auditFile.Close()
	var audit AuditRecord
	if err := json.NewDecoder(io.LimitReader(auditFile, 1<<20)).Decode(&audit); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid audit JSON: "+err.Error())
		return
	}

	ctx, cancel := DeadlineContext(r.Context())
	defer cancel()

	max := h.MaxPDFSize
	if max == 0 {
		max = 25 * 1024 * 1024
	}

	rec, err := h.Svc.Finalize(ctx, FinalizeInput{
		SessionToken: token,
		PDFBody:      pdfFile,
		PDFMaxBytes:  max,
		Audit:        audit,
		IPAddress:    clientIP(r),
		UserAgent:    r.UserAgent(),
	})
	if err != nil {
		writeSessionErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"signatureId":     rec.SignatureID,
		"signedAt":        rec.SignedAt,
		"sha256After":     rec.SHA256After,
		"signedPdfS3Key":  rec.SignedPDFS3Key,
		"auditTrailS3Key": rec.AuditTrailS3Key,
	})
}

func (h *Handlers) listTemplates(w http.ResponseWriter, r *http.Request) {
	providerID, ok := h.Auth.ProviderID(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	ts, err := h.Svc.store.ListTemplateSummaries(r.Context(), providerID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ts == nil {
		ts = []TemplateSummary{}
	}
	writeJSON(w, http.StatusOK, ts)
}

type saveTemplateFieldsRequest struct {
	Fields []Field `json:"fields"`
}

func (h *Handlers) saveTemplateFields(w http.ResponseWriter, r *http.Request) {
	_, ok := h.Auth.ProviderID(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	templateID := chi.URLParam(r, "id")
	var req saveTemplateFieldsRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	fields := assignFieldIDs(req.Fields)
	if err := h.Svc.store.UpsertTemplateFields(r.Context(), templateID, fields); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Helpers

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeSessionErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrSessionNotFound):
		writeErr(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrSessionExpired),
		errors.Is(err, ErrSessionCompleted),
		errors.Is(err, ErrSessionRevoked):
		writeErr(w, http.StatusGone, err.Error())
	case errors.Is(err, ErrHashMismatch),
		errors.Is(err, ErrNotAPDF),
		errors.Is(err, ErrAuditMalformed):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrIntegrityFailed):
		writeErr(w, http.StatusConflict, err.Error())
	default:
		writeErr(w, http.StatusInternalServerError, err.Error())
	}
}

// clientIP extracts the signer's IP honoring X-Forwarded-For when set by the
// upstream CDN/ALB. This is trusted only because the L7 proxy always rewrites
// the header; never trust arbitrary clients.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Leftmost entry is the original client.
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// assignFieldIDs fills in any missing field IDs with freshly generated base62
// tokens. We still let the caller supply its own ID (useful for idempotent
// template edits).
func assignFieldIDs(in []Field) []Field {
	out := make([]Field, len(in))
	for i, f := range in {
		if f.ID == "" {
			// Ignore error: crypto/rand only fails if entropy is exhausted.
			tok, _ := NewToken()
			f.ID = tok[:16]
		}
		out[i] = f
	}
	return out
}

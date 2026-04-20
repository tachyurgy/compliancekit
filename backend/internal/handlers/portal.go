package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"database/sql"

	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	"github.com/tachyurgy/compliancekit/backend/internal/magiclink"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
	"github.com/tachyurgy/compliancekit/backend/internal/storage"
)

type PortalHandler struct {
	Pool    *sql.DB
	Storage *storage.Client
	Magic   *magiclink.Service
	Log     *slog.Logger
}

// GET /portal/parent  (magic-link gated)
// Returns {child, required_documents[], existing_documents[]}
func (h *PortalHandler) ParentHome(w http.ResponseWriter, r *http.Request) {
	claim := mw.MagicClaimFrom(r.Context())
	if claim == nil || claim.Kind != magiclink.KindParentUpload {
		httpx.RenderError(w, r, httpx.ErrForbidden)
		return
	}
	// claim.SubjectID is the child's ID
	var (
		firstName, lastName, classroom string
		providerName                   string
	)
	err := h.Pool.QueryRowContext(r.Context(), `
		SELECT c.first_name, c.last_name, COALESCE(c.classroom,''), p.name
		FROM children c JOIN providers p ON p.id = c.provider_id
		WHERE c.id = ? AND c.provider_id = ?`,
		claim.SubjectID, claim.ProviderID).Scan(&firstName, &lastName, &classroom, &providerName)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrNotFound, err))
		return
	}

	docs, err := h.listDocsForSubject(r, claim.ProviderID, "child", claim.SubjectID)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"child": map[string]string{
			"id": claim.SubjectID, "first_name": firstName, "last_name": lastName, "classroom": classroom,
		},
		"provider":           map[string]string{"id": claim.ProviderID, "name": providerName},
		"existing_documents": docs,
		"required_documents": []string{"immunization_record", "emergency_card", "physical_exam", "enrollment_form"},
		"token_expires_at":   claim.ExpiresAt,
	})
}

// POST /portal/upload (magic-link gated for parent/staff)
// body: { "kind":"immunization_record", "mime_type":"application/pdf", "title":"..." }
// returns presigned URL
func (h *PortalHandler) Upload(w http.ResponseWriter, r *http.Request) {
	claim := mw.MagicClaimFrom(r.Context())
	if claim == nil {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	var subjectKind string
	switch claim.Kind {
	case magiclink.KindParentUpload:
		subjectKind = "child"
	case magiclink.KindStaffUpload:
		subjectKind = "staff"
	default:
		httpx.RenderError(w, r, httpx.ErrForbidden)
		return
	}

	var in struct {
		Kind     string `json:"kind"`
		MIMEType string `json:"mime_type"`
		Title    string `json:"title"`
	}
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	if in.Kind == "" || in.MIMEType == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("kind and mime_type required"))
		return
	}

	docID := base62.NewID()[:22]
	bucket := h.Storage.Buckets().RawUploads
	key := fmt.Sprintf("providers/%s/%s/%s/%s/%s",
		claim.ProviderID, subjectKind, safeID(claim.SubjectID), in.Kind, docID+"."+extFor(in.MIMEType))

	url, err := h.Storage.PresignPutURL(r.Context(), bucket, key, in.MIMEType, 15*time.Minute)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	title := in.Title
	if title == "" {
		title = in.Kind
	}
	uploadedVia := "parent_portal"
	if subjectKind == "staff" {
		uploadedVia = "staff_portal"
	}
	if _, err := h.Pool.ExecContext(r.Context(), `
		INSERT INTO documents (id, provider_id, subject_kind, subject_id, kind, title,
		                      storage_bucket, storage_key, mime_type, size_bytes,
		                      uploaded_by, uploaded_via, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,0,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		docID, claim.ProviderID, subjectKind, claim.SubjectID, in.Kind, title,
		bucket, key, in.MIMEType, claim.SubjectID, uploadedVia); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	httpx.RenderJSON(w, http.StatusOK, map[string]string{
		"document_id": docID, "upload_url": url, "storage_key": key,
	})
}

// GET /portal/staff  (magic-link gated)
func (h *PortalHandler) StaffHome(w http.ResponseWriter, r *http.Request) {
	claim := mw.MagicClaimFrom(r.Context())
	if claim == nil || claim.Kind != magiclink.KindStaffUpload {
		httpx.RenderError(w, r, httpx.ErrForbidden)
		return
	}
	var firstName, lastName, role, providerName string
	err := h.Pool.QueryRowContext(r.Context(), `
		SELECT s.first_name, s.last_name, s.role, p.name
		FROM staff s JOIN providers p ON p.id = s.provider_id
		WHERE s.id = ? AND s.provider_id = ?`,
		claim.SubjectID, claim.ProviderID).Scan(&firstName, &lastName, &role, &providerName)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrNotFound, err))
		return
	}
	docs, err := h.listDocsForSubject(r, claim.ProviderID, "staff", claim.SubjectID)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"staff": map[string]string{
			"id": claim.SubjectID, "first_name": firstName, "last_name": lastName, "role": role,
		},
		"provider":           map[string]string{"id": claim.ProviderID, "name": providerName},
		"existing_documents": docs,
		"required_documents": []string{"tb_test", "cpr_cert", "first_aid_cert", "background_check"},
		"token_expires_at":   claim.ExpiresAt,
	})
}

type portalDoc struct {
	ID        string     `json:"id"`
	Kind      string     `json:"kind"`
	Title     string     `json:"title"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func (h *PortalHandler) listDocsForSubject(r *http.Request, pid, subjectKind, subjectID string) ([]portalDoc, error) {
	rows, err := h.Pool.QueryContext(r.Context(), `
		SELECT id, kind, title, expires_at, created_at
		FROM documents
		WHERE provider_id = ? AND subject_kind = ? AND subject_id = ? AND deleted_at IS NULL
		ORDER BY created_at DESC`, pid, subjectKind, subjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]portalDoc, 0)
	for rows.Next() {
		var d portalDoc
		if err := rows.Scan(&d.ID, &d.Kind, &d.Title, &d.ExpiresAt, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

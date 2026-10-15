package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/tachyurgy/compliancekit/backend/internal/auditlog"
	"github.com/tachyurgy/compliancekit/backend/internal/base62"
	"github.com/tachyurgy/compliancekit/backend/internal/dbtime"
	"github.com/tachyurgy/compliancekit/backend/internal/httpx"
	"github.com/tachyurgy/compliancekit/backend/internal/magiclink"
	mw "github.com/tachyurgy/compliancekit/backend/internal/middleware"
	"github.com/tachyurgy/compliancekit/backend/internal/storage"
)

type PortalHandler struct {
	Pool      *sql.DB
	Storage   *storage.Client
	Magic     *magiclink.Service
	Documents *DocumentHandler
	Log       *slog.Logger
}

// portalRequiredDoc is the rollup shape sent to the portal welcome screen.
// Same shape for parent and staff variants — the only difference upstream is
// which entity_type we filter document_requirements on.
type portalRequiredDoc struct {
	ID         string     `json:"id"`       // requirement row id
	DocKind    string     `json:"doc_kind"` // matches documents.kind
	Label      string     `json:"label"`
	Required   bool       `json:"required"`
	Status     string     `json:"status"` // on_file | expiring_soon | expired | missing | awaiting_upload
	DocumentID string     `json:"document_id,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

// GET /portal/parent  (magic-link gated)
// Returns {child, provider, required_documents[], existing_documents[]}.
func (h *PortalHandler) ParentHome(w http.ResponseWriter, r *http.Request) {
	claim := mw.MagicClaimFrom(r.Context())
	if claim == nil || claim.Kind != magiclink.KindParentUpload {
		httpx.RenderError(w, r, httpx.ErrForbidden)
		return
	}
	var (
		firstName, lastName, classroom string
		parentName                     sql.NullString
		providerName, providerState    string
	)
	err := h.Pool.QueryRowContext(r.Context(), `
		SELECT c.first_name, c.last_name, COALESCE(c.classroom,''), c.parent_name,
		       p.name, COALESCE(p.state_code,'FL')
		FROM children c JOIN providers p ON p.id = c.provider_id
		WHERE c.id = ? AND c.provider_id = ? AND c.withdrawn_at IS NULL`,
		claim.SubjectID, claim.ProviderID).Scan(&firstName, &lastName, &classroom, &parentName, &providerName, &providerState)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.RenderError(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	required, existing, err := h.rollupForSubject(r.Context(), claim.ProviderID, "child", claim.SubjectID, providerState)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"kind": "parent_upload",
		"child": map[string]string{
			"id": claim.SubjectID, "first_name": firstName, "last_name": lastName, "classroom": classroom,
		},
		"parent_name":        parentName.String,
		"provider":           map[string]string{"id": claim.ProviderID, "name": providerName, "state": providerState},
		"required_documents": required,
		"existing_documents": existing,
		"status_counts":      countStatuses(required, existing),
		"token_expires_at":   claim.ExpiresAt,
	})
}

// GET /portal/staff  (magic-link gated)
func (h *PortalHandler) StaffHome(w http.ResponseWriter, r *http.Request) {
	claim := mw.MagicClaimFrom(r.Context())
	if claim == nil || claim.Kind != magiclink.KindStaffUpload {
		httpx.RenderError(w, r, httpx.ErrForbidden)
		return
	}
	var firstName, lastName, role, providerName, providerState string
	err := h.Pool.QueryRowContext(r.Context(), `
		SELECT s.first_name, s.last_name, s.role, p.name, COALESCE(p.state_code,'FL')
		FROM staff s JOIN providers p ON p.id = s.provider_id
		WHERE s.id = ? AND s.provider_id = ?`,
		claim.SubjectID, claim.ProviderID).Scan(&firstName, &lastName, &role, &providerName, &providerState)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.RenderError(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	required, existing, err := h.rollupForSubject(r.Context(), claim.ProviderID, "staff", claim.SubjectID, providerState)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"kind": "staff_upload",
		"staff": map[string]string{
			"id": claim.SubjectID, "first_name": firstName, "last_name": lastName, "role": role,
		},
		"provider":           map[string]string{"id": claim.ProviderID, "name": providerName, "state": providerState},
		"required_documents": required,
		"existing_documents": existing,
		"status_counts":      countStatuses(required, existing),
		"token_expires_at":   claim.ExpiresAt,
	})
}

// rollupForSubject builds the {required[], existing[]} pair for a single
// subject (one child or one staff). Cross-references document_requirements
// (REQ-008/REQ-004 seeded set) against the live `documents` table.
func (h *PortalHandler) rollupForSubject(ctx context.Context, providerID, entity, subjectID, stateCode string) ([]portalRequiredDoc, []portalDoc, error) {
	// 1) Pull the required-doc set for this entity_type + state.
	reqRows, err := h.Pool.QueryContext(ctx, `
		SELECT id, doc_kind, label, required
		FROM document_requirements
		WHERE state_code = ? AND entity_type = ?
		ORDER BY sort_order, label`, stateCode, entity)
	if err != nil {
		return nil, nil, fmt.Errorf("portal: required docs: %w", err)
	}
	defer reqRows.Close()
	required := []portalRequiredDoc{}
	for reqRows.Next() {
		var rd portalRequiredDoc
		var reqInt int
		if err := reqRows.Scan(&rd.ID, &rd.DocKind, &rd.Label, &reqInt); err != nil {
			return nil, nil, err
		}
		rd.Required = reqInt != 0
		rd.Status = "missing"
		required = append(required, rd)
	}

	// 2) Pull existing documents for this subject.
	docRows, err := h.Pool.QueryContext(ctx, `
		SELECT id, COALESCE(kind, doc_type, '') AS kind, COALESCE(title,''),
		       expires_at, expiration_confirmed_at, COALESCE(does_not_expire,0),
		       created_at
		FROM documents
		WHERE provider_id = ? AND subject_kind = ? AND subject_id = ? AND deleted_at IS NULL
		ORDER BY created_at DESC`, providerID, entity, subjectID)
	if err != nil {
		return nil, nil, fmt.Errorf("portal: existing docs: %w", err)
	}
	defer docRows.Close()
	existing := []portalDoc{}
	now := time.Now()
	for docRows.Next() {
		var d portalDoc
		var dne int
		var confirmedAt sql.NullString
		if err := docRows.Scan(&d.ID, &d.Kind, &d.Title, dbtime.IntoPtr(&d.ExpiresAt), &confirmedAt, &dne, dbtime.Into(&d.CreatedAt)); err != nil {
			return nil, nil, err
		}
		d.DoesNotExpire = dne != 0
		d.Confirmed = confirmedAt.Valid && confirmedAt.String != ""
		existing = append(existing, d)

		// Annotate the matching requirement row.
		for i := range required {
			if required[i].DocKind != d.Kind {
				continue
			}
			required[i].DocumentID = d.ID
			required[i].ExpiresAt = d.ExpiresAt
			required[i].Status = computeStatus(d, now)
			break
		}
	}

	return required, existing, nil
}

// portalDoc is the shape returned in `existing_documents`.
type portalDoc struct {
	ID            string     `json:"id"`
	Kind          string     `json:"kind"`
	Title         string     `json:"title"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	DoesNotExpire bool       `json:"does_not_expire,omitempty"`
	Confirmed     bool       `json:"confirmed"`
	CreatedAt     time.Time  `json:"created_at"`
}

// computeStatus turns a doc + now into a brand-voice status label.
//
//	on_file       — confirmed and not within 90 days of expiry (or no expiry).
//	expiring_soon — within 90 days but not past.
//	expired       — past expires_at.
//	awaiting_upload — uploaded but pending OCR confirmation by the user.
func computeStatus(d portalDoc, now time.Time) string {
	if !d.Confirmed && !d.DoesNotExpire {
		return "awaiting_upload"
	}
	if d.DoesNotExpire || d.ExpiresAt == nil {
		return "on_file"
	}
	delta := d.ExpiresAt.Sub(now)
	switch {
	case delta < 0:
		return "expired"
	case delta < 90*24*time.Hour:
		return "expiring_soon"
	default:
		return "on_file"
	}
}

func countStatuses(required []portalRequiredDoc, _ []portalDoc) map[string]int {
	out := map[string]int{"on_file": 0, "expiring_soon": 0, "expired": 0, "missing": 0, "awaiting_upload": 0}
	for _, r := range required {
		out[r.Status]++
	}
	return out
}

// presignInput is the JSON body of /portal/upload (presign-style).
type presignInput struct {
	Kind     string `json:"kind"`      // doc kind hint (or "" → "unknown_will_classify")
	MIMEType string `json:"mime_type"` // image/jpeg | image/png | application/pdf | image/heic | image/webp
	Title    string `json:"title"`
	// MultiPage is the count for camera multi-shot; ≥1. Set to N to receive N
	// presigned URLs (one per photo). The browser PUTs each photo, then calls
	// /portal/upload/{id}/finalize once with the full set, server stitches
	// into a single PDF before OCR.
	MultiPage int `json:"multi_page,omitempty"`
}

// POST /portal/upload (magic-link gated for parent/staff)
//
// Two response shapes:
//   - single-file upload  → { document_id, upload_url, storage_key }
//   - multi-shot upload   → { document_id, parts: [{upload_url, storage_key}], multi_page: N }
//
// In the multi-shot case the browser uploads each part to its own presigned
// URL, then POSTs to /portal/upload/{id}/finalize, which downloads each part,
// EXIF-strips, auto-rotates, stitches into a single PDF, and runs OCR.
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

	var in presignInput
	if err := httpx.DecodeJSON(r, &in); err != nil {
		httpx.RenderError(w, r, err)
		return
	}
	if in.MIMEType == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("mime_type required"))
		return
	}
	kind := in.Kind
	if kind == "" {
		// REQ-005: if the user picked "I'm not sure", we run OCR-as-classifier.
		// The placeholder kind is what the chase engine sees if confirmation
		// never happens — kept narrow so the dashboard surfaces it.
		kind = "unknown_will_classify"
	}
	if !h.Storage.IsConfigured() {
		httpx.RenderError(w, r, &httpx.APIError{
			Status: http.StatusServiceUnavailable, Code: "storage_not_configured",
			Message: "document upload isn't configured yet — ask your director",
		})
		return
	}

	docID := base62.NewID()[:22]
	bucket := h.Storage.Buckets().RawUploads
	uploadedVia := "parent_portal"
	if subjectKind == "staff" {
		uploadedVia = "staff_portal"
	}
	title := in.Title
	if title == "" {
		title = strings.ReplaceAll(kind, "_", " ")
	}

	// Insert the parent documents row first; the parts are tracked through
	// the storage key prefix `<key_root>/part-<n>.<ext>`.
	keyRoot := fmt.Sprintf("providers/%s/%s/%s/%s/%s",
		claim.ProviderID, subjectKind, safeID(claim.SubjectID), kind, docID)
	finalKey := keyRoot + "." + extFor(in.MIMEType)
	if _, err := h.Pool.ExecContext(r.Context(), `
		INSERT INTO documents (id, provider_id, owner_kind, owner_id, subject_kind, subject_id, kind, title, doc_type,
		                      storage_bucket, storage_key, s3_key, mime_type, size_bytes,
		                      uploaded_by, uploaded_via, ocr_status,
		                      created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,0,?,?, 'pending', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		docID, claim.ProviderID, subjectKind, claim.SubjectID, subjectKind, claim.SubjectID, kind, title, kind,
		bucket, finalKey, finalKey, in.MIMEType, claim.SubjectID, uploadedVia); err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}

	if in.MultiPage > 1 {
		// Multi-shot: presign one URL per page; browser PUTs each independently.
		parts := make([]map[string]string, 0, in.MultiPage)
		for i := 1; i <= in.MultiPage; i++ {
			partKey := fmt.Sprintf("%s/part-%02d.%s", keyRoot, i, extFor(in.MIMEType))
			url, err := h.Storage.PresignPutURL(r.Context(), bucket, partKey, in.MIMEType, 15*time.Minute)
			if err != nil {
				httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
				return
			}
			parts = append(parts, map[string]string{
				"upload_url":  url,
				"storage_key": partKey,
			})
		}
		httpx.RenderJSON(w, http.StatusOK, map[string]any{
			"document_id": docID,
			"parts":       parts,
			"multi_page":  in.MultiPage,
			"final_key":   finalKey,
		})
		return
	}

	// Single-file path.
	url, err := h.Storage.PresignPutURL(r.Context(), bucket, finalKey, in.MIMEType, 15*time.Minute)
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	httpx.RenderJSON(w, http.StatusOK, map[string]any{
		"document_id": docID,
		"upload_url":  url,
		"storage_key": finalKey,
	})
}

// POST /portal/upload/{id}/finalize — token-gated finalize. Stitches multi-shot
// parts (if any), runs EXIF-strip + auto-rotate, then delegates to the shared
// OCR+expire pipeline. Writes an `upload_audit` row.
func (h *PortalHandler) Finalize(w http.ResponseWriter, r *http.Request) {
	claim := mw.MagicClaimFrom(r.Context())
	if claim == nil {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		httpx.RenderError(w, r, httpx.BadRequestf("id required"))
		return
	}

	// Verify the document belongs to this subject.
	var (
		bucket, key, mime, kind, subjectKind, subjectID string
	)
	err := h.Pool.QueryRowContext(r.Context(), `
		SELECT storage_bucket, storage_key, mime_type, kind, subject_kind, COALESCE(subject_id,'')
		FROM documents
		WHERE id = ? AND provider_id = ? AND deleted_at IS NULL`,
		id, claim.ProviderID).Scan(&bucket, &key, &mime, &kind, &subjectKind, &subjectID)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.RenderError(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	if subjectID != claim.SubjectID {
		httpx.RenderError(w, r, httpx.ErrForbidden)
		return
	}

	var in struct {
		Parts []string `json:"parts"` // ordered storage keys to stitch (optional)
	}
	if r.Body != nil {
		_ = httpx.DecodeJSON(r, &in)
	}

	// Stitch + EXIF-strip if we have multi-shot parts.
	if len(in.Parts) > 1 {
		if err := h.stitchAndStore(r.Context(), bucket, key, in.Parts); err != nil {
			httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
			return
		}
		// After stitching, the final blob is a PDF.
		if _, err := h.Pool.ExecContext(r.Context(),
			`UPDATE documents SET mime_type = 'application/pdf', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id); err != nil {
			h.Log.Warn("portal finalize: update mime", "err", err)
		}
		mime = "application/pdf"
	} else if isImage(mime) {
		// Single image: re-encode in place to strip EXIF + apply rotation.
		if err := h.stripEXIFInPlace(r.Context(), bucket, key, mime); err != nil {
			h.Log.Warn("portal finalize: exif strip", "err", err, "doc", id)
			// non-fatal — keep going.
		}
	}

	// Audit row.
	auditID := base62.NewID()[:22]
	var fileBytes int64
	if body, _, err := h.Storage.GetFromBucket(r.Context(), bucket, key); err == nil {
		if buf, err := io.ReadAll(body); err == nil {
			fileBytes = int64(len(buf))
		}
		_ = body.Close()
	}
	if _, err := h.Pool.ExecContext(r.Context(), `
		INSERT INTO upload_audit (id, magic_link_id, document_id, provider_id, ip, user_agent, file_bytes, file_mime, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		auditID, claim.TokenID, id, claim.ProviderID,
		auditlog.ClientIP(r), r.UserAgent(), fileBytes, mime); err != nil {
		h.Log.Warn("portal finalize: audit write", "err", err)
	}

	// Delegate to the shared OCR+expire pipeline.
	if h.Documents != nil {
		h.Documents.runFinalize(w, r, claim.ProviderID, id, "")
		return
	}
	httpx.RenderJSON(w, http.StatusOK, map[string]any{"document_id": id})
}

// POST /portal/document/{id}/confirm — token-gated confirmation. Same body as
// /api/documents/{id}/confirm; verifies subject ownership.
func (h *PortalHandler) ConfirmDocument(w http.ResponseWriter, r *http.Request) {
	claim := mw.MagicClaimFrom(r.Context())
	if claim == nil {
		httpx.RenderError(w, r, httpx.ErrUnauthorized)
		return
	}
	id := chi.URLParam(r, "id")
	var subjectID string
	err := h.Pool.QueryRowContext(r.Context(), `
		SELECT COALESCE(subject_id,'') FROM documents
		WHERE id = ? AND provider_id = ? AND deleted_at IS NULL`,
		id, claim.ProviderID).Scan(&subjectID)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.RenderError(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.RenderError(w, r, httpx.Wrap(httpx.ErrInternal, err))
		return
	}
	if subjectID != claim.SubjectID {
		httpx.RenderError(w, r, httpx.ErrForbidden)
		return
	}
	if h.Documents == nil {
		httpx.RenderError(w, r, httpx.ErrInternal)
		return
	}
	// userID empty for portal confirms — confirmed_by_user_id stays NULL,
	// the magic_link token id on the upload_audit row carries provenance.
	h.Documents.runConfirm(w, r, claim.ProviderID, id, "")
}

// stitchAndStore downloads each part, EXIF-strips images, and writes a single
// PDF blob to finalKey.
//
// PDF construction: each image becomes a page of `pdf-lib`-style PDF using the
// stdlib `pdf` package isn't available, so we use github.com/jung-kurt/gofpdf
// when present. We avoid pulling a new dep for this slice — instead we
// concatenate the JPEG bytes inside a minimal PDF using the existing
// `pdfsign` package's helpers if exposed; falling back to the simplest path
// of writing parts unchanged when no PDF builder is available.
//
// For v1, we cheat: we encode each image as a JPEG and write it as a
// minimal one-page-per-image PDF. The exact PDF rendering helper is
// pdfFromImages below, which uses only stdlib + the imaging dep we already
// pull.
func (h *PortalHandler) stitchAndStore(ctx context.Context, bucket, finalKey string, partKeys []string) error {
	imgs := make([]image.Image, 0, len(partKeys))
	for _, k := range partKeys {
		body, _, err := h.Storage.GetFromBucket(ctx, bucket, k)
		if err != nil {
			return fmt.Errorf("stitch: get %s: %w", k, err)
		}
		buf, err := io.ReadAll(body)
		_ = body.Close()
		if err != nil {
			return fmt.Errorf("stitch: read %s: %w", k, err)
		}
		// Decode then re-encode → pixel data only. The Go stdlib JPEG decoder
		// does NOT honor EXIF orientation, so the bytes we keep are exactly
		// the pixels the phone wrote — modern phones pre-rotate before saving
		// so this is correct for ~95% of devices. The privacy win (no GPS
		// metadata) is the load-bearing requirement; auto-rotate is
		// best-effort. See readEXIFOrientation for our heuristic helper.
		orient := readEXIFOrientation(buf)
		img, _, err := image.Decode(bytes.NewReader(buf))
		if err != nil {
			return fmt.Errorf("stitch: decode %s: %w", k, err)
		}
		img = applyOrientation(img, orient)
		imgs = append(imgs, img)
	}
	pdfBytes, err := pdfFromImages(imgs)
	if err != nil {
		return fmt.Errorf("stitch: build pdf: %w", err)
	}
	if err := h.Storage.PutDocument(ctx, finalKey, "application/pdf", bytes.NewReader(pdfBytes)); err != nil {
		return fmt.Errorf("stitch: put final: %w", err)
	}
	return nil
}

// stripEXIFInPlace decodes a single image, auto-rotates via EXIF, re-encodes
// (which drops the EXIF block including GPS), and writes back to the same
// (bucket, key) the bytes came from.
func (h *PortalHandler) stripEXIFInPlace(ctx context.Context, bucket, key, mime string) error {
	body, _, err := h.Storage.GetFromBucket(ctx, bucket, key)
	if err != nil {
		return err
	}
	defer body.Close()
	buf, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	orient := readEXIFOrientation(buf)
	img, _, err := image.Decode(bytes.NewReader(buf))
	if err != nil {
		return err
	}
	img = applyOrientation(img, orient)
	var out bytes.Buffer
	// Always re-encode as JPEG quality 90 — privacy outranks the original
	// container choice. Mime stays whatever it was on the row; if we had a
	// PNG we are now a JPEG inside a PNG-named key, but the bucket policy
	// keys off content-type at PUT, which we set explicitly here.
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 90}); err != nil {
		return err
	}
	_ = mime
	return h.Storage.PutToBucket(ctx, bucket, key, "image/jpeg", &out)
}

func isImage(m string) bool {
	return strings.HasPrefix(m, "image/")
}

// pdfFromImages builds a minimal one-page-per-image PDF. Each image is
// embedded as a JPEG at full resolution (page size matches the image at 72 dpi
// equivalent). This is enough to feed Mistral OCR; we are not pretending to
// produce print-grade PDFs.
func pdfFromImages(imgs []image.Image) ([]byte, error) {
	if len(imgs) == 0 {
		return nil, errors.New("no images")
	}
	// Encode each image as JPEG bytes once, since we'll reference the byte
	// stream from the PDF object table.
	type encoded struct {
		jpeg []byte
		w, h int
	}
	parts := make([]encoded, 0, len(imgs))
	for _, img := range imgs {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
			return nil, err
		}
		b := img.Bounds()
		parts = append(parts, encoded{jpeg: buf.Bytes(), w: b.Dx(), h: b.Dy()})
	}

	// Build a tiny PDF by hand. Object layout:
	//   1 Catalog   /Pages 2 0 R
	//   2 Pages     /Kids [3,5,7,...] /Count N
	//   3,5,...     Page  /MediaBox [0 0 W H] /Resources <</XObject<</Im0 4 0 R>>> /Contents 4-1 ... wait
	// Simpler: per image, two objects — page + image XObject + content stream.
	// We use sequential ids:
	//   id=1 Catalog
	//   id=2 Pages
	//   per image i (zero-indexed):
	//     pageId    = 3 + 3*i
	//     contentId = 4 + 3*i
	//     imageId   = 5 + 3*i
	var (
		buf      bytes.Buffer
		offsets  = []int{0}
		writeObj = func(id int, body string) {
			offsets = append(offsets, buf.Len())
			fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", id, body)
		}
		writeObjB = func(id int, header string, body []byte) {
			offsets = append(offsets, buf.Len())
			fmt.Fprintf(&buf, "%d 0 obj\n%s\nstream\n", id, header)
			buf.Write(body)
			fmt.Fprintf(&buf, "\nendstream\nendobj\n")
		}
	)
	buf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")

	// Catalog (1)
	writeObj(1, "<< /Type /Catalog /Pages 2 0 R >>")

	// Pages (2): kids list assembled below; we emit a placeholder and patch.
	// For determinism we'll just write it with a known layout after the rest.
	pagesOffsetIdx := len(offsets) // index that will receive the Pages object
	// reserve slot 2 by emitting a placeholder we'll overwrite at the end.
	pagesObjStart := buf.Len()
	offsets = append(offsets, pagesObjStart)
	pagesPlaceholder := []byte(strings.Repeat(" ", 256))
	buf.WriteString("2 0 obj\n")
	pagesBodyStart := buf.Len()
	buf.Write(pagesPlaceholder)
	buf.WriteString("\nendobj\n")

	pageIDs := make([]int, len(parts))
	for i, p := range parts {
		pageID := 3 + 3*i
		contentID := 4 + 3*i
		imgID := 5 + 3*i
		pageIDs[i] = pageID

		// Page object
		writeObj(pageID, fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %d %d] /Resources << /XObject << /Im0 %d 0 R >> >> /Contents %d 0 R >>",
			p.w, p.h, imgID, contentID))

		// Content stream (draw image full page)
		content := fmt.Sprintf("q\n%d 0 0 %d 0 0 cm\n/Im0 Do\nQ\n", p.w, p.h)
		writeObjB(contentID, fmt.Sprintf("<< /Length %d >>", len(content)), []byte(content))

		// Image XObject (JPEG = DCTDecode)
		header := fmt.Sprintf(
			"<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>",
			p.w, p.h, len(p.jpeg))
		writeObjB(imgID, header, p.jpeg)
	}

	// Patch the Pages object now that we know the page IDs.
	pagesBody := pagesBody(pageIDs)
	if len(pagesBody) > len(pagesPlaceholder) {
		// extremely unlikely (pages is at most ~16 bytes per page for IDs);
		// guard anyway.
		return nil, fmt.Errorf("pdf: pages body overflow (%d > %d)", len(pagesBody), len(pagesPlaceholder))
	}
	// Pad and overwrite
	out := buf.Bytes()
	copy(out[pagesBodyStart:pagesBodyStart+len(pagesBody)], pagesBody)
	for j := len(pagesBody); j < len(pagesPlaceholder); j++ {
		out[pagesBodyStart+j] = ' '
	}
	_ = pagesOffsetIdx

	// xref + trailer
	xrefStart := len(out)
	out = append(out, []byte(fmt.Sprintf("xref\n0 %d\n0000000000 65535 f \n", len(offsets)))...)
	for i := 1; i < len(offsets); i++ {
		out = append(out, []byte(fmt.Sprintf("%010d 00000 n \n", offsets[i]))...)
	}
	out = append(out, []byte(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		len(offsets), xrefStart))...)
	return out, nil
}

// readEXIFOrientation does a tiny scan of a JPEG file for the standard EXIF
// Orientation tag (0x0112). Returns the orientation value (1..8) or 1 (no-op)
// when the tag is absent or the file isn't a JPEG. We do this ourselves
// rather than pull in a full EXIF dep — the parser is minimal and only needs
// to find one 16-bit tag value.
func readEXIFOrientation(b []byte) int {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return 1
	}
	i := 2
	for i+4 < len(b) {
		if b[i] != 0xFF {
			return 1
		}
		marker := b[i+1]
		segLen := int(b[i+2])<<8 | int(b[i+3])
		if marker == 0xE1 && i+segLen < len(b) {
			// APP1 — possibly EXIF.
			start := i + 4
			if start+6 < len(b) && string(b[start:start+6]) == "Exif\x00\x00" {
				tiff := start + 6
				if tiff+8 >= len(b) {
					return 1
				}
				var be bool
				switch string(b[tiff : tiff+2]) {
				case "MM":
					be = true
				case "II":
					be = false
				default:
					return 1
				}
				readU16 := func(p int) int {
					if be {
						return int(b[p])<<8 | int(b[p+1])
					}
					return int(b[p+1])<<8 | int(b[p])
				}
				readU32 := func(p int) int {
					if be {
						return int(b[p])<<24 | int(b[p+1])<<16 | int(b[p+2])<<8 | int(b[p+3])
					}
					return int(b[p+3])<<24 | int(b[p+2])<<16 | int(b[p+1])<<8 | int(b[p])
				}
				ifd := tiff + readU32(tiff+4)
				if ifd+2 >= len(b) {
					return 1
				}
				count := readU16(ifd)
				for j := 0; j < count; j++ {
					entry := ifd + 2 + j*12
					if entry+12 > len(b) {
						return 1
					}
					tag := readU16(entry)
					if tag == 0x0112 {
						return readU16(entry + 8)
					}
				}
				return 1
			}
		}
		i += 2 + segLen
	}
	return 1
}

// applyOrientation rotates / flips the decoded image to undo what an EXIF
// Orientation tag would otherwise direct a viewer to do. We only handle the
// rotation cases (3, 6, 8) — the mirror cases (2, 4, 5, 7) are extremely
// rare on phone cameras and not worth the extra code in v1.
func applyOrientation(img image.Image, orient int) image.Image {
	switch orient {
	case 3:
		return rotate180(img)
	case 6:
		return rotate90CW(img)
	case 8:
		return rotate90CCW(img)
	default:
		return img
	}
}

func rotate90CW(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(h-1-y, x, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func rotate90CCW(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(y, w-1-x, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func rotate180(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(w-1-x, h-1-y, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func pagesBody(pageIDs []int) []byte {
	var sb strings.Builder
	sb.WriteString("<< /Type /Pages /Kids [")
	for i, id := range pageIDs {
		if i > 0 {
			sb.WriteString(" ")
		}
		fmt.Fprintf(&sb, "%d 0 R", id)
	}
	fmt.Fprintf(&sb, "] /Count %d >>", len(pageIDs))
	return []byte(sb.String())
}

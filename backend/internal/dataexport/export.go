// Package dataexport produces a provider's full self-service data export:
// every DB row scoped to a provider_id, plus every S3 object under
// providers/<id>/ in the documents + signed-pdfs buckets, packaged into a
// single ZIP and uploaded to exports/<provider_id>/<unix_ts>.zip in the
// audit-trail bucket (we reuse that bucket because it already has long-term
// durability + versioning; "exports" is effectively a sibling prefix to
// "audit").
//
// The caller (handlers/dataexport.go) kicks this off in a background goroutine
// in response to POST /api/exports. When this function returns successfully,
// the caller mints a short-lived presigned GET URL for the ZIP and emails it
// to the requesting user. The data_exports table records every attempt so the
// Settings UI can render the history.
//
// We intentionally use the standard library `archive/zip` + `encoding/csv`
// rather than a streaming/pluggable exporter framework. The exports are
// small (single facility — usually < 2000 rows across all tables, < 500 MB
// of files) and bounded; simplicity beats optimization here.
package dataexport

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/tachyurgy/compliancekit/backend/internal/storage"
)

// Manifest summarizes what the ZIP contains. Lives at MANIFEST.json at the ZIP root.
type Manifest struct {
	ProviderID   string            `json:"provider_id"`
	GeneratedAt  string            `json:"generated_at"`
	TableCounts  map[string]int    `json:"table_counts"`
	BucketCounts map[string]int    `json:"bucket_counts"`
	Tables       []string          `json:"tables"`
	Note         string            `json:"note,omitempty"`
	Prefixes     map[string]string `json:"s3_prefixes"`
}

// tableExports is the list of CSVs we write — one per relevant table.
// Each exporter produces a row-of-strings writer callback that the main
// ExportProvider loop can invoke.
//
// The predicates are intentionally permissive (no deleted_at IS NULL filter
// on parent tables): the export is meant to be the owner's own data in full
// fidelity, including rows they've soft-deleted. The only hard filter is
// "this row belongs to this provider_id."
type tableExport struct {
	Name    string // file name inside the ZIP (without .csv suffix)
	Query   string
	Args    int // how many times ? needs providerID repeated
	Columns []string
}

var tableExports = []tableExport{
	{
		Name:    "children",
		Query:   `SELECT id, provider_id, first_name, last_name, date_of_birth, enrollment_date, withdrawal_date, COALESCE(guardians,'[]'), COALESCE(allergies,''), COALESCE(medical_notes,''), created_at, updated_at, COALESCE(deleted_at,'') FROM children WHERE provider_id = ?`,
		Args:    1,
		Columns: []string{"id", "provider_id", "first_name", "last_name", "date_of_birth", "enrollment_date", "withdrawal_date", "guardians", "allergies", "medical_notes", "created_at", "updated_at", "deleted_at"},
	},
	{
		Name:    "staff",
		Query:   `SELECT id, provider_id, first_name, last_name, COALESCE(email,''), COALESCE(phone,''), COALESCE(hired_on,''), COALESCE(role,''), status, created_at, updated_at, COALESCE(deleted_at,'') FROM staff WHERE provider_id = ?`,
		Args:    1,
		Columns: []string{"id", "provider_id", "first_name", "last_name", "email", "phone", "hired_on", "role", "status", "created_at", "updated_at", "deleted_at"},
	},
	{
		Name:    "documents",
		Query:   `SELECT id, provider_id, owner_kind, owner_id, doc_type, COALESCE(original_filename,''), COALESCE(mime_type,''), s3_key, COALESCE(byte_size,0), uploaded_via, ocr_status, COALESCE(ocr_confidence,0), COALESCE(expiration_date,''), expiration_source, COALESCE(expiration_confidence,0), created_at, updated_at, COALESCE(deleted_at,'') FROM documents WHERE provider_id = ?`,
		Args:    1,
		Columns: []string{"id", "provider_id", "owner_kind", "owner_id", "doc_type", "original_filename", "mime_type", "s3_key", "byte_size", "uploaded_via", "ocr_status", "ocr_confidence", "expiration_date", "expiration_source", "expiration_confidence", "created_at", "updated_at", "deleted_at"},
	},
	{
		Name:    "drill_logs",
		Query:   `SELECT id, provider_id, drill_kind, drill_date, COALESCE(duration_seconds,0), COALESCE(notes,''), COALESCE(attachment_document_id,''), created_at, updated_at, COALESCE(deleted_at,'') FROM drill_logs WHERE provider_id = ?`,
		Args:    1,
		Columns: []string{"id", "provider_id", "drill_kind", "drill_date", "duration_seconds", "notes", "attachment_document_id", "created_at", "updated_at", "deleted_at"},
	},
	{
		Name:    "inspection_runs",
		Query:   `SELECT id, provider_id, state, started_at, COALESCE(completed_at,''), COALESCE(score,0), total_items, items_passed, items_failed, items_na, created_at FROM inspection_runs WHERE provider_id = ?`,
		Args:    1,
		Columns: []string{"id", "provider_id", "state", "started_at", "completed_at", "score", "total_items", "items_passed", "items_failed", "items_na", "created_at"},
	},
	{
		Name:    "inspection_responses",
		Query:   `SELECT r.id, r.run_id, r.item_id, r.answer, COALESCE(r.evidence_document_id,''), COALESCE(r.note,''), r.answered_at FROM inspection_responses r JOIN inspection_runs ir ON ir.id = r.run_id WHERE ir.provider_id = ?`,
		Args:    1,
		Columns: []string{"id", "run_id", "item_id", "answer", "evidence_document_id", "note", "answered_at"},
	},
	{
		Name:    "compliance_snapshots",
		Query:   `SELECT id, provider_id, score, violation_count, critical_count, payload, computed_at FROM compliance_snapshots WHERE provider_id = ?`,
		Args:    1,
		Columns: []string{"id", "provider_id", "score", "violation_count", "critical_count", "payload", "computed_at"},
	},
	{
		Name:    "chase_events",
		Query:   `SELECT id, provider_id, target_kind, target_id, document_type, trigger, channel, recipient_contact, COALESCE(sent_at,''), COALESCE(failed_at,''), COALESCE(failure_reason,''), created_at FROM chase_events WHERE provider_id = ?`,
		Args:    1,
		Columns: []string{"id", "provider_id", "target_kind", "target_id", "document_type", "trigger", "channel", "recipient_contact", "sent_at", "failed_at", "failure_reason", "created_at"},
	},
	{
		Name:    "signatures",
		Query:   `SELECT s.id, s.sign_session_id, s.document_id, COALESCE(s.signer_user_id,''), s.signer_declared_name, s.signed_at, hex(s.sha256_before), hex(s.sha256_after), s.s3_key_signed, s.s3_key_audit, COALESCE(s.signer_ip,''), COALESCE(s.signer_user_agent,''), s.consent_version_id, s.created_at FROM signatures s JOIN documents d ON d.id = s.document_id WHERE d.provider_id = ?`,
		Args:    1,
		Columns: []string{"id", "sign_session_id", "document_id", "signer_user_id", "signer_declared_name", "signed_at", "sha256_before_hex", "sha256_after_hex", "s3_key_signed", "s3_key_audit", "signer_ip", "signer_user_agent", "consent_version_id", "created_at"},
	},
	{
		Name:    "policy_acceptances",
		Query:   `SELECT pa.id, COALESCE(pa.user_id,''), COALESCE(pa.magic_link_token_id,''), pa.policy_version_id, pa.accepted_at, COALESCE(pa.ip,''), COALESCE(pa.user_agent,'') FROM policy_acceptances pa JOIN users u ON u.id = pa.user_id WHERE u.provider_id = ?`,
		Args:    1,
		Columns: []string{"id", "user_id", "magic_link_token_id", "policy_version_id", "accepted_at", "ip", "user_agent"},
	},
	{
		Name:    "audit_log",
		Query:   `SELECT id, COALESCE(provider_id,''), actor_kind, COALESCE(actor_id,''), action, COALESCE(target_kind,''), COALESCE(target_id,''), metadata, COALESCE(ip,''), COALESCE(user_agent,''), created_at FROM audit_log WHERE provider_id = ?`,
		Args:    1,
		Columns: []string{"id", "provider_id", "actor_kind", "actor_id", "action", "target_kind", "target_id", "metadata", "ip", "user_agent", "created_at"},
	},
	{
		Name:    "users",
		Query:   `SELECT id, provider_id, email, COALESCE(phone,''), full_name, role, COALESCE(last_login_at,''), COALESCE(email_verified_at,''), created_at, updated_at, COALESCE(deleted_at,'') FROM users WHERE provider_id = ?`,
		Args:    1,
		Columns: []string{"id", "provider_id", "email", "phone", "full_name", "role", "last_login_at", "email_verified_at", "created_at", "updated_at", "deleted_at"},
	},
}

// s3ExportPrefixes lists the (bucket, prefix) pairs we copy file-by-file
// into the ZIP. We do NOT include the audit-trail bucket here — its contents
// are private to us and already summarized in the audit_log CSV — and we do
// not include raw uploads (those are also present in the documents bucket
// after OCR finalization).
func s3ExportPrefixes(bk storage.Buckets, providerID string) []bucketPrefix {
	prefix := "providers/" + providerID + "/"
	return []bucketPrefix{
		{Bucket: bk.Documents, Prefix: prefix, ZipDir: "documents/"},
		{Bucket: bk.SignedPDFs, Prefix: prefix, ZipDir: "signed/"},
	}
}

type bucketPrefix struct {
	Bucket string
	Prefix string
	ZipDir string
}

// ExportProvider builds the ZIP, uploads it to the audit-trail bucket at
// exports/<provider_id>/<unix_ts>.zip, and returns the S3 key. The caller is
// expected to presign a short-lived GET URL and email it to the requester.
//
// s3c may be nil in unit tests; in that case only the CSV portion is produced
// and the ZIP is not uploaded (an empty key is returned).
func ExportProvider(ctx context.Context, pool *sql.DB, s3c *storage.Client, providerID string) (string, error) {
	if pool == nil {
		return "", fmt.Errorf("dataexport: nil pool")
	}
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)

	manifest := Manifest{
		ProviderID:   providerID,
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		TableCounts:  map[string]int{},
		BucketCounts: map[string]int{},
		Tables:       make([]string, 0, len(tableExports)),
		Prefixes:     map[string]string{},
		Note:         "ComplianceKit provider data export. Each CSV is a verbatim dump of the DB rows belonging to this provider_id. S3 objects are included at their original key paths under the documents/ and signed/ directories.",
	}

	// --- DB tables → CSV files at the ZIP root ---
	for _, te := range tableExports {
		manifest.Tables = append(manifest.Tables, te.Name)
		count, err := exportTableCSV(ctx, pool, zw, te, providerID)
		if err != nil {
			return "", fmt.Errorf("export %s: %w", te.Name, err)
		}
		manifest.TableCounts[te.Name] = count
	}

	// --- S3 objects → documents/... and signed/... inside the ZIP ---
	if s3c != nil {
		for _, bp := range s3ExportPrefixes(s3c.Buckets(), providerID) {
			if bp.Bucket == "" {
				continue
			}
			manifest.Prefixes[bp.ZipDir] = bp.Bucket + "/" + bp.Prefix
			n, err := streamS3Prefix(ctx, s3c, bp, zw)
			if err != nil {
				return "", fmt.Errorf("stream %s/%s: %w", bp.Bucket, bp.Prefix, err)
			}
			manifest.BucketCounts[bp.Bucket+"/"+bp.Prefix] = n
		}
	}

	// --- MANIFEST.json ---
	mw, err := zw.Create("MANIFEST.json")
	if err != nil {
		return "", fmt.Errorf("zip MANIFEST: %w", err)
	}
	mb, _ := json.MarshalIndent(manifest, "", "  ")
	if _, err := mw.Write(mb); err != nil {
		return "", fmt.Errorf("write MANIFEST: %w", err)
	}

	if err := zw.Close(); err != nil {
		return "", fmt.Errorf("zip close: %w", err)
	}

	// --- Upload the finished ZIP ---
	key := fmt.Sprintf("exports/%s/%d.zip", providerID, time.Now().Unix())
	if s3c != nil {
		bucket := s3c.Buckets().AuditTrail
		if bucket == "" {
			return "", fmt.Errorf("dataexport: audit-trail bucket not configured")
		}
		if err := s3c.PutObject(ctx, bucket, key, "application/zip", bytes.NewReader(zipBuf.Bytes())); err != nil {
			return "", fmt.Errorf("put export: %w", err)
		}
	}
	return key, nil
}

// exportTableCSV writes one CSV file into zw. Returns the row count.
func exportTableCSV(ctx context.Context, pool *sql.DB, zw *zip.Writer, te tableExport, providerID string) (int, error) {
	args := make([]any, te.Args)
	for i := range args {
		args[i] = providerID
	}
	rows, err := pool.QueryContext(ctx, te.Query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return 0, err
	}

	fw, err := zw.Create(te.Name + ".csv")
	if err != nil {
		return 0, err
	}
	csvw := csv.NewWriter(fw)
	// Prefer the canonical column list from the tableExport (matches SELECT
	// order with COALESCE aliases collapsed). Fall back to driver-reported
	// names if shorter.
	header := te.Columns
	if len(header) != len(cols) {
		header = cols
	}
	if err := csvw.Write(header); err != nil {
		return 0, err
	}

	count := 0
	rawRow := make([]any, len(cols))
	rawRowPtrs := make([]any, len(cols))
	for i := range rawRow {
		rawRowPtrs[i] = &rawRow[i]
	}
	for rows.Next() {
		if err := rows.Scan(rawRowPtrs...); err != nil {
			return 0, err
		}
		record := make([]string, len(cols))
		for i, v := range rawRow {
			record[i] = anyToCSV(v)
		}
		if err := csvw.Write(record); err != nil {
			return 0, err
		}
		count++
	}
	csvw.Flush()
	if err := csvw.Error(); err != nil {
		return 0, err
	}
	return count, rows.Err()
}

// anyToCSV converts an arbitrary SQL-scanned value into its CSV string
// representation. modernc.org/sqlite returns most values as string/int64/
// float64/bool/time.Time/[]byte; we collapse each to a printable form.
func anyToCSV(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []byte:
		return string(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int64:
		return fmt.Sprintf("%d", x)
	case int:
		return fmt.Sprintf("%d", x)
	case float64:
		return fmt.Sprintf("%v", x)
	case time.Time:
		return x.UTC().Format(time.RFC3339)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// streamS3Prefix lists every object under bp.Prefix in bp.Bucket and copies
// each one into the ZIP at <bp.ZipDir><rest-of-key>.
func streamS3Prefix(ctx context.Context, s3c *storage.Client, bp bucketPrefix, zw *zip.Writer) (int, error) {
	keys, err := s3c.ListPrefix(ctx, bp.Bucket, bp.Prefix)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, key := range keys {
		rel := strings.TrimPrefix(key, bp.Prefix)
		name := bp.ZipDir + rel
		body, err := s3c.GetObject(ctx, bp.Bucket, key)
		if err != nil {
			// Skip unreadable objects rather than fail the whole export.
			continue
		}
		fw, err := zw.Create(name)
		if err != nil {
			_ = body.Close()
			return count, err
		}
		if _, err := io.Copy(fw, body); err != nil {
			_ = body.Close()
			return count, err
		}
		_ = body.Close()
		count++
	}
	return count, nil
}

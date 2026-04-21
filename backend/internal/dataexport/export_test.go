package dataexport

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/tachyurgy/compliancekit/backend/internal/testhelp"
)

// buildZipForTest re-runs the same logic as ExportProvider's CSV + manifest
// phase and returns the ZIP bytes for inspection. It intentionally mirrors
// ExportProvider's structure but skips the s3 upload — used by tests only.
func buildZipForTest(t *testing.T, pool *sql.DB, providerID string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	manifest := Manifest{
		ProviderID:   providerID,
		TableCounts:  map[string]int{},
		BucketCounts: map[string]int{},
		Tables:       make([]string, 0, len(tableExports)),
		Prefixes:     map[string]string{},
	}
	for _, te := range tableExports {
		manifest.Tables = append(manifest.Tables, te.Name)
		count, err := exportTableCSV(context.Background(), pool, zw, te, providerID)
		if err != nil {
			t.Fatalf("exportTableCSV %s: %v", te.Name, err)
		}
		manifest.TableCounts[te.Name] = count
	}
	mw, err := zw.Create("MANIFEST.json")
	if err != nil {
		t.Fatalf("create manifest: %v", err)
	}
	mb, _ := json.MarshalIndent(manifest, "", "  ")
	_, _ = mw.Write(mb)
	_ = zw.Close()
	return buf.Bytes()
}

func TestExportProvider_NilPool_Errors(t *testing.T) {
	t.Parallel()
	_, err := ExportProvider(context.Background(), nil, nil, "p1")
	if err == nil {
		t.Fatal("expected error on nil pool")
	}
}

func TestExportProvider_EmptyProvider_ProducesValidZip(t *testing.T) {
	t.Parallel()
	pool := testhelp.OpenDB(t)
	_, err := pool.Exec(`INSERT INTO providers (id, legal_name, state) VALUES ('p1','A','CA')`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	zipBytes := buildZipForTest(t, pool, "p1")
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}

	// Must contain MANIFEST.json.
	foundManifest := false
	foundFiles := map[string]bool{}
	for _, f := range zr.File {
		foundFiles[f.Name] = true
		if f.Name == "MANIFEST.json" {
			foundManifest = true
			rc, _ := f.Open()
			body, _ := io.ReadAll(rc)
			_ = rc.Close()
			var m Manifest
			if err := json.Unmarshal(body, &m); err != nil {
				t.Fatalf("MANIFEST.json not parseable: %v", err)
			}
			if m.ProviderID != "p1" {
				t.Fatalf("manifest provider_id = %q, want p1", m.ProviderID)
			}
		}
	}
	if !foundManifest {
		t.Fatal("MANIFEST.json not present")
	}

	// Every table in tableExports must appear as a .csv file.
	for _, te := range tableExports {
		name := te.Name + ".csv"
		if !foundFiles[name] {
			t.Errorf("missing %s in zip", name)
		}
	}
}

func TestExportProvider_CSVsFilteredByProviderID(t *testing.T) {
	t.Parallel()
	pool := testhelp.OpenDB(t)
	_, err := pool.Exec(`
INSERT INTO providers (id, legal_name, state) VALUES ('p1','X','CA'), ('p2','Y','TX');
INSERT INTO children (id, provider_id, first_name, last_name, date_of_birth, enrollment_date, guardians) VALUES
  ('c1','p1','A','A','2020-01-01','2024-01-01','[]'),
  ('c2','p1','B','B','2020-01-01','2024-01-01','[]'),
  ('c3','p2','C','C','2020-01-01','2024-01-01','[]');
INSERT INTO staff (id, provider_id, first_name, last_name, status) VALUES
  ('s1','p1','Alice','A','active'),
  ('s2','p2','Bob','B','active');
INSERT INTO users (id, provider_id, email, full_name, role) VALUES
  ('u1','p1','a@x.com','A','provider_admin'),
  ('u2','p2','b@y.com','B','provider_admin');
`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	zipBytes := buildZipForTest(t, pool, "p1")
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}

	// Count data rows per CSV.
	counts := map[string]int{}
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".csv") {
			continue
		}
		rc, _ := f.Open()
		body, _ := io.ReadAll(rc)
		_ = rc.Close()

		r := csv.NewReader(bytes.NewReader(body))
		r.FieldsPerRecord = -1 // tolerate ragged last line
		records, err := r.ReadAll()
		if err != nil {
			t.Fatalf("csv parse %s: %v", f.Name, err)
		}
		// First row is the header.
		counts[f.Name] = len(records) - 1
	}
	if counts["children.csv"] != 2 {
		t.Fatalf("children.csv data rows = %d, want 2", counts["children.csv"])
	}
	if counts["staff.csv"] != 1 {
		t.Fatalf("staff.csv data rows = %d, want 1", counts["staff.csv"])
	}
	if counts["users.csv"] != 1 {
		t.Fatalf("users.csv data rows = %d, want 1", counts["users.csv"])
	}
}

func TestExportProvider_ManifestHasRowCounts(t *testing.T) {
	t.Parallel()
	pool := testhelp.OpenDB(t)
	_, err := pool.Exec(`
INSERT INTO providers (id, legal_name, state) VALUES ('p1','X','CA');
INSERT INTO children (id, provider_id, first_name, last_name, date_of_birth, enrollment_date, guardians) VALUES
  ('c1','p1','A','A','2020-01-01','2024-01-01','[]'),
  ('c2','p1','B','B','2020-01-01','2024-01-01','[]'),
  ('c3','p1','C','C','2020-01-01','2024-01-01','[]');
INSERT INTO staff (id, provider_id, first_name, last_name, status) VALUES
  ('s1','p1','Alice','A','active'),
  ('s2','p1','Bob','B','active');
`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	zipBytes := buildZipForTest(t, pool, "p1")
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	var m Manifest
	for _, f := range zr.File {
		if f.Name != "MANIFEST.json" {
			continue
		}
		rc, _ := f.Open()
		body, _ := io.ReadAll(rc)
		_ = rc.Close()
		if err := json.Unmarshal(body, &m); err != nil {
			t.Fatalf("parse manifest: %v", err)
		}
	}
	if m.TableCounts["children"] != 3 {
		t.Fatalf("children count = %d, want 3", m.TableCounts["children"])
	}
	if m.TableCounts["staff"] != 2 {
		t.Fatalf("staff count = %d, want 2", m.TableCounts["staff"])
	}
	if len(m.Tables) != len(tableExports) {
		t.Fatalf("tables len = %d, want %d", len(m.Tables), len(tableExports))
	}
}

func TestAnyToCSV_Types(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"hi", "hi"},
		{[]byte("bytes"), "bytes"},
		{true, "true"},
		{false, "false"},
		{int64(42), "42"},
		{int(7), "7"},
		{float64(3.14), "3.14"},
	}
	for _, tc := range cases {
		if got := anyToCSV(tc.in); got != tc.want {
			t.Errorf("anyToCSV(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestExportProvider_NonexistentProvider_EmptyCSVs(t *testing.T) {
	t.Parallel()
	pool := testhelp.OpenDB(t)
	// No provider row inserted. All tables should still appear with zero rows.
	zipBytes := buildZipForTest(t, pool, "nobody")
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	csvs := 0
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, ".csv") {
			csvs++
		}
	}
	if csvs != len(tableExports) {
		t.Fatalf("csvs = %d, want %d", csvs, len(tableExports))
	}
}

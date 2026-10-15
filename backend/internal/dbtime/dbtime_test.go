package dbtime

import (
	"database/sql"
	"testing"
	"time"
)

func TestParse_SQLiteCurrentTimestampFormat(t *testing.T) {
	// SQLite's CURRENT_TIMESTAMP returns "YYYY-MM-DD HH:MM:SS" (no T, no TZ).
	// This is the dominant on-disk format for our TEXT timestamp columns.
	got, err := Parse("2026-05-13 19:40:45")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := time.Date(2026, 5, 13, 19, 40, 45, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestParse_RFC3339(t *testing.T) {
	got, err := Parse("2026-05-13T19:40:45Z")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := time.Date(2026, 5, 13, 19, 40, 45, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestParse_BareDate(t *testing.T) {
	got, err := Parse("2099-01-01")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestParse_EmptyOrNull(t *testing.T) {
	for _, s := range []string{"", "  ", "NULL", "null"} {
		got, err := Parse(s)
		if err != nil {
			t.Fatalf("parse(%q): unexpected err %v", s, err)
		}
		if !got.IsZero() {
			t.Fatalf("parse(%q): want zero, got %s", s, got)
		}
	}
}

func TestParse_Garbage(t *testing.T) {
	if _, err := Parse("not a date"); err == nil {
		t.Fatal("expected error on garbage input")
	}
}

func TestNullTime_ScanString(t *testing.T) {
	var n NullTime
	if err := n.Scan("2026-05-13 19:40:45"); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !n.Valid {
		t.Fatal("want valid")
	}
	if !n.Time.Equal(time.Date(2026, 5, 13, 19, 40, 45, 0, time.UTC)) {
		t.Fatalf("unexpected time %s", n.Time)
	}
}

func TestNullTime_ScanNil(t *testing.T) {
	var n NullTime
	if err := n.Scan(nil); err != nil {
		t.Fatalf("scan nil: %v", err)
	}
	if n.Valid {
		t.Fatal("nil should scan invalid")
	}
}

func TestNullTime_ScanTimeTime(t *testing.T) {
	want := time.Date(2026, 5, 13, 19, 40, 45, 0, time.UTC)
	var n NullTime
	if err := n.Scan(want); err != nil {
		t.Fatalf("scan time.Time: %v", err)
	}
	if !n.Valid || !n.Time.Equal(want) {
		t.Fatalf("unexpected %+v", n)
	}
}

func TestFromNullString(t *testing.T) {
	if got := FromNullString(sql.NullString{}); got != nil {
		t.Fatal("invalid NullString should yield nil")
	}
	if got := FromNullString(sql.NullString{Valid: true, String: ""}); got != nil {
		t.Fatal("empty string should yield nil")
	}
	got := FromNullString(sql.NullString{Valid: true, String: "2026-05-13 19:40:45"})
	if got == nil || !got.Equal(time.Date(2026, 5, 13, 19, 40, 45, 0, time.UTC)) {
		t.Fatalf("unexpected %v", got)
	}
}

func TestParseDriverTimeString(t *testing.T) {
	got, err := Parse("2026-10-10 15:00:00 +0000 UTC")
	if err != nil || !got.Equal(time.Date(2026, 10, 10, 15, 0, 0, 0, time.UTC)) {
		t.Fatalf("got %v %v", got, err)
	}
}

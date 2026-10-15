// Package dbtime parses SQLite text timestamps into Go time.Time.
//
// Why this package exists: the modernc.org/sqlite driver only auto-parses
// columns declared with TIMESTAMP / DATE / DATETIME type affinity. Migration
// 000004 declared every timestamp on the documents table as TEXT (and similar
// migrations followed that pattern), so any scan of `created_at`,
// `updated_at`, `expires_at`, etc. directly into a `time.Time` field fails
// with "unsupported Scan, storing driver.Value type string into type
// *time.Time" the moment the column is non-NULL.
//
// Production writes use either `CURRENT_TIMESTAMP` (SQLite's
// "2006-01-02 15:04:05" format) or Go-bound `time.Time` (RFC3339Nano with a
// timezone offset), and OCR confirmations may bind bare dates like
// "2099-01-01". Parse() accepts all three and falls back gracefully.
package dbtime

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Layouts tried in order. SQLite's default CURRENT_TIMESTAMP comes first
// because production data is dominated by it.
var layouts = []string{
	"2006-01-02 15:04:05",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02T15:04:05Z07:00",
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02",
	// What the modernc driver writes when a Go time.Time is bound to a
	// TEXT column: time.Time.String() without the monotonic suffix.
	"2006-01-02 15:04:05.999999999 -0700 MST",
}

// Parse parses a SQLite text timestamp. Empty / "NULL" yield zero time, nil.
// An unparseable non-empty string yields an error so callers can surface it.
func Parse(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "null") {
		return time.Time{}, nil
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("dbtime: cannot parse %q", s)
}

// MustParse mirrors Parse but swallows errors, returning the zero time on
// failure. Use sparingly — preferred in scan paths where the underlying
// column is already constrained by a CHECK or where a bad value should
// surface as "empty" rather than a 500.
func MustParse(s string) time.Time {
	t, _ := Parse(s)
	return t
}

// NullTime is a scan target for nullable text timestamp columns. It mirrors
// sql.NullTime but parses SQLite TEXT formats. Use when the column is
// declared TEXT and may be NULL.
type NullTime struct {
	Time  time.Time
	Valid bool
}

// Scan accepts string, []byte, time.Time, or nil. Empty strings scan as
// Valid=false so callers can distinguish "absent" from "zero time".
func (n *NullTime) Scan(src any) error {
	if src == nil {
		n.Time, n.Valid = time.Time{}, false
		return nil
	}
	var s string
	switch v := src.(type) {
	case string:
		s = v
	case []byte:
		s = string(v)
	case time.Time:
		n.Time, n.Valid = v.UTC(), !v.IsZero()
		return nil
	default:
		return fmt.Errorf("dbtime.NullTime: unsupported source %T", src)
	}
	if strings.TrimSpace(s) == "" {
		n.Time, n.Valid = time.Time{}, false
		return nil
	}
	t, err := Parse(s)
	if err != nil {
		return err
	}
	n.Time, n.Valid = t, true
	return nil
}

// FromNullString promotes a sql.NullString that holds a SQLite text
// timestamp into a *time.Time (nil when invalid/empty). Useful when you've
// already scanned into a NullString for other reasons.
func FromNullString(ns sql.NullString) *time.Time {
	if !ns.Valid {
		return nil
	}
	t, err := Parse(ns.String)
	if err != nil || t.IsZero() {
		return nil
	}
	return &t
}

// ValueOrZero collapses a NullTime to a plain time.Time (zero when invalid).
// Use for non-pointer time.Time destinations like Document.CreatedAt.
func (n NullTime) ValueOrZero() time.Time {
	if !n.Valid {
		return time.Time{}
	}
	return n.Time
}

// Into adapts a *time.Time as a sql.Scanner for a TEXT timestamp column, so
// call sites can keep scanning straight into struct fields:
//
//	rows.Scan(&d.ID, dbtime.Into(&d.CreatedAt))
//
// NULL or empty leaves the zero time.
func Into(dst *time.Time) sql.Scanner { return intoScanner{dst: dst} }

// IntoPtr is Into for nullable *time.Time fields: NULL or empty sets nil.
func IntoPtr(dst **time.Time) sql.Scanner { return intoPtrScanner{dst: dst} }

type intoScanner struct{ dst *time.Time }

func (s intoScanner) Scan(src any) error {
	var n NullTime
	if err := n.Scan(src); err != nil {
		return err
	}
	*s.dst = n.ValueOrZero()
	return nil
}

type intoPtrScanner struct{ dst **time.Time }

func (s intoPtrScanner) Scan(src any) error {
	var n NullTime
	if err := n.Scan(src); err != nil {
		return err
	}
	if !n.Valid {
		*s.dst = nil
		return nil
	}
	t := n.Time
	*s.dst = &t
	return nil
}

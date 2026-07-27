package store

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// sqliteTimeLayouts covers every shape a timestamp can arrive in from an
// existing database: SQLAlchemy's microsecond form, its second-precision form
// when microseconds happened to be zero, and ISO-8601 in case a row was written
// by some other tool.
var sqliteTimeLayouts = []string{
	pyDatetimeLayout,
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05.999999999Z07:00",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02T15:04:05",
	"2006-01-02",
}

// NullTime scans the DATETIME columns of either backend and writes them back in
// the dialect's native format. Values are always naive UTC, matching what the
// Python application persisted.
type NullTime struct {
	Time    time.Time
	Valid   bool
	dialect Dialect
}

// NewTime wraps t for insertion. Passing the zero time yields SQL NULL.
func NewTime(d Dialect, t time.Time) NullTime {
	if t.IsZero() {
		return NullTime{dialect: d}
	}
	return NullTime{Time: t.UTC(), Valid: true, dialect: d}
}

// NewNullTime wraps an optional timestamp for insertion.
func NewNullTime(d Dialect, t *time.Time) NullTime {
	if t == nil || t.IsZero() {
		return NullTime{dialect: d}
	}
	return NullTime{Time: t.UTC(), Valid: true, dialect: d}
}

func (n *NullTime) Scan(src any) error {
	n.Valid = false
	n.Time = time.Time{}
	switch v := src.(type) {
	case nil:
		return nil
	case time.Time:
		n.Time, n.Valid = naiveUTC(v), true
		return nil
	case []byte:
		return n.scanString(string(v))
	case string:
		return n.scanString(v)
	case int64:
		// A Unix timestamp; only produced by non-SQLAlchemy writers.
		n.Time, n.Valid = time.Unix(v, 0).UTC(), true
		return nil
	default:
		return fmt.Errorf("store: cannot scan %T into NullTime", src)
	}
}

func (n *NullTime) scanString(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	for _, layout := range sqliteTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			n.Time, n.Valid = naiveUTC(t), true
			return nil
		}
	}
	return fmt.Errorf("store: unrecognised timestamp %q", s)
}

func (n NullTime) Value() (driver.Value, error) {
	if !n.Valid {
		return nil, nil
	}
	if n.dialect == SQLite {
		// Text in SQLAlchemy's exact layout, so ordering comparisons against
		// pre-existing rows stay correct.
		return n.Time.UTC().Format(pyDatetimeLayout), nil
	}
	return n.Time.UTC(), nil
}

// Ptr returns the timestamp, or nil when the column was NULL.
func (n NullTime) Ptr() *time.Time {
	if !n.Valid {
		return nil
	}
	t := n.Time
	return &t
}

// naiveUTC drops any timezone offset the driver attached without shifting the
// wall-clock reading, because the stored values are naive UTC to begin with.
func naiveUTC(t time.Time) time.Time {
	if t.Location() == time.UTC {
		return t
	}
	_, offset := t.Zone()
	if offset == 0 {
		return t.UTC()
	}
	return t.UTC()
}

// nullBool scans SQLite's integer booleans as well as Postgres' native ones.
type nullBool struct {
	Bool  bool
	Valid bool
}

func (n *nullBool) Scan(src any) error {
	n.Valid = false
	n.Bool = false
	switch v := src.(type) {
	case nil:
		return nil
	case bool:
		n.Bool, n.Valid = v, true
	case int64:
		n.Bool, n.Valid = v != 0, true
	case float64:
		n.Bool, n.Valid = v != 0, true
	case []byte:
		return n.scanString(string(v))
	case string:
		return n.scanString(v)
	default:
		return fmt.Errorf("store: cannot scan %T into bool", src)
	}
	return nil
}

func (n *nullBool) scanString(s string) error {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "null":
		return nil
	case "t", "true", "1", "y", "yes":
		n.Bool, n.Valid = true, true
	case "f", "false", "0", "n", "no":
		n.Bool, n.Valid = false, true
	default:
		i, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("store: cannot scan %q into bool", s)
		}
		n.Bool, n.Valid = i != 0, true
	}
	return nil
}

// orDefault reports the scanned value, falling back when the column was NULL.
func (n nullBool) orDefault(def bool) bool {
	if !n.Valid {
		return def
	}
	return n.Bool
}

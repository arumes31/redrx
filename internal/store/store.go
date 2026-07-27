// Package store is the persistence layer.
//
// The schema is byte-for-byte the one the previous SQLAlchemy application
// created, including the legacy column names (`urls.clicks` holds the click
// counter, `urls.rotate_targets` holds a JSON array). Existing SQLite files and
// Postgres databases are opened and used in place, with no migration step.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"modernc.org/sqlite"
)

// Dialect distinguishes the two supported backends.
type Dialect int

const (
	SQLite Dialect = iota
	Postgres
)

// ErrNotFound is returned by lookups that match no row.
var ErrNotFound = errors.New("store: not found")

// pyDatetimeLayout is how SQLAlchemy renders DATETIME values into SQLite TEXT.
// Writing anything else would break ordering comparisons against existing rows,
// so all SQLite timestamp writes go through it.
const pyDatetimeLayout = "2006-01-02 15:04:05.000000"

type DB struct {
	*sql.DB
	dialect Dialect
}

func (d *DB) Dialect() Dialect { return d.dialect }

// Open connects to the database named by a SQLAlchemy-style URL. Both the
// `sqlite:///path` and `postgresql://...` forms used by the Python config are
// accepted so existing DATABASE_URL values keep working.
func Open(ctx context.Context, databaseURL string) (*DB, error) {
	driver, dsn, dialect, err := parseURL(databaseURL)
	if err != nil {
		return nil, err
	}

	sqlDB, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if dialect == SQLite {
		// One writer at a time; SQLite serialises writes anyway and this avoids
		// spurious "database is locked" errors under concurrent redirects.
		sqlDB.SetMaxOpenConns(1)
	} else {
		sqlDB.SetMaxOpenConns(25)
		sqlDB.SetMaxIdleConns(5)
		sqlDB.SetConnMaxLifetime(time.Hour)
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}

	db := &DB{DB: sqlDB, dialect: dialect}

	if dialect == SQLite {
		for _, pragma := range []string{
			"PRAGMA journal_mode=WAL",
			"PRAGMA busy_timeout=5000",
			"PRAGMA foreign_keys=ON",
		} {
			if _, err := db.ExecContext(ctx, pragma); err != nil {
				db.Close()
				return nil, fmt.Errorf("apply %q: %w", pragma, err)
			}
		}
	}

	return db, nil
}

func parseURL(raw string) (driver, dsn string, d Dialect, err error) {
	switch {
	case strings.HasPrefix(raw, "sqlite:///"):
		return "sqlite", strings.TrimPrefix(raw, "sqlite:///"), SQLite, nil
	case strings.HasPrefix(raw, "sqlite://"):
		return "sqlite", strings.TrimPrefix(raw, "sqlite://"), SQLite, nil
	case strings.HasPrefix(raw, "postgresql+psycopg2://"):
		return "pgx", "postgresql://" + strings.TrimPrefix(raw, "postgresql+psycopg2://"), Postgres, nil
	case strings.HasPrefix(raw, "postgresql://"), strings.HasPrefix(raw, "postgres://"):
		return "pgx", raw, Postgres, nil
	default:
		return "", "", 0, fmt.Errorf("store: unsupported DATABASE_URL scheme in %q", raw)
	}
}

// rebind converts the `?` placeholders used throughout this package into the
// positional form Postgres requires.
func (d *DB) rebind(query string) string {
	if d.dialect != Postgres {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 8)
	n := 0
	for i := 0; i < len(query); i++ {
		if query[i] != '?' {
			b.WriteByte(query[i])
			continue
		}
		n++
		b.WriteByte('$')
		b.WriteString(strconv.Itoa(n))
	}
	return b.String()
}

func (d *DB) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return d.QueryContext(ctx, d.rebind(query), args...)
}

func (d *DB) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return d.QueryRowContext(ctx, d.rebind(query), args...)
}

func (d *DB) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return d.ExecContext(ctx, d.rebind(query), args...)
}

// now returns the current time in the naive-UTC form the Python app stored.
func now() time.Time { return time.Now().UTC() }

// IsUniqueViolation reports whether err is a unique-constraint failure from
// either backend. Callers use it to turn the race between checking a short code
// and inserting it into a conflict response rather than a server error.
func IsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgUniqueViolation
	}

	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		switch sqliteErr.Code() {
		case sqliteConstraintUnique, sqliteConstraintPrimaryKey:
			return true
		}
	}
	return false
}

const (
	// 23505 is unique_violation in the SQLSTATE table.
	pgUniqueViolation = "23505"
	// Extended SQLite result codes for the two constraints that can reject a
	// duplicate short code.
	sqliteConstraintUnique     = 2067
	sqliteConstraintPrimaryKey = 1555
)

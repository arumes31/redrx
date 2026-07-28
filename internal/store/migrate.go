package store

import (
	"context"
	"fmt"
	"strings"
)

// column describes one column of the expected schema, with the type used both
// when creating a table and when adding it to an existing table.
type column struct {
	name       string
	sqliteType string
	pgType     string
}

type table struct {
	name    string
	columns []column
	// extra holds constraints appended after the column list at CREATE time.
	extra []string
}

// schema mirrors the SQLAlchemy models the Python application declared. The
// column names are load-bearing: `urls.clicks` is the click counter and
// `urls.rotate_targets` is a JSON array, both inherited from that schema.
var schema = []table{
	{
		name: "users",
		columns: []column{
			{"id", "INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT", "SERIAL PRIMARY KEY"},
			{"username", "VARCHAR(80) NOT NULL", "VARCHAR(80) NOT NULL"},
			{"email", "VARCHAR(120) NOT NULL", "VARCHAR(120) NOT NULL"},
			{"password_hash", "VARCHAR(255) NOT NULL", "VARCHAR(255) NOT NULL"},
			{"api_key", "VARCHAR(36)", "VARCHAR(36)"},
			{"created_at", "DATETIME", "TIMESTAMP"},
		},
	},
	{
		name: "urls",
		columns: []column{
			{"id", "INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT", "SERIAL PRIMARY KEY"},
			{"user_id", "INTEGER", "INTEGER"},
			{"short_code", "VARCHAR(20) NOT NULL", "VARCHAR(20) NOT NULL"},
			{"long_url", "TEXT NOT NULL", "TEXT NOT NULL"},
			{"rotate_targets", "TEXT", "TEXT"},
			{"ios_target_url", "TEXT", "TEXT"},
			{"android_target_url", "TEXT", "TEXT"},
			{"password_hash", "VARCHAR(255)", "VARCHAR(255)"},
			{"qr_color", "VARCHAR(32)", "VARCHAR(32)"},
			{"qr_background", "VARCHAR(32)", "VARCHAR(32)"},
			{"preview_mode", "BOOLEAN", "BOOLEAN"},
			{"stats_enabled", "BOOLEAN", "BOOLEAN"},
			{"is_enabled", "BOOLEAN", "BOOLEAN"},
			{"clicks", "INTEGER", "INTEGER"},
			{"created_at", "DATETIME", "TIMESTAMP"},
			{"expires_at", "DATETIME", "TIMESTAMP"},
			{"start_at", "DATETIME", "TIMESTAMP"},
			{"end_at", "DATETIME", "TIMESTAMP"},
			{"last_accessed_at", "DATETIME", "TIMESTAMP"},
		},
		extra: []string{"FOREIGN KEY(user_id) REFERENCES users (id)"},
	},
	{
		name: "clicks",
		columns: []column{
			{"id", "INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT", "SERIAL PRIMARY KEY"},
			{"url_id", "INTEGER NOT NULL", "INTEGER NOT NULL"},
			{"timestamp", "DATETIME", "TIMESTAMP"},
			{"ip_address", "VARCHAR(45)", "VARCHAR(45)"},
			{"country", "VARCHAR(100)", "VARCHAR(100)"},
			{"browser", "VARCHAR(50)", "VARCHAR(50)"},
			{"platform", "VARCHAR(50)", "VARCHAR(50)"},
			{"referrer", "VARCHAR(255)", "VARCHAR(255)"},
		},
		extra: []string{"FOREIGN KEY(url_id) REFERENCES urls (id)"},
	},
}

// indexes reproduces the indexes the SQLAlchemy models declared. Names match so
// they are recognised as already present on an existing database.
var indexes = []struct{ name, ddl string }{
	{"ix_users_username", "CREATE UNIQUE INDEX IF NOT EXISTS ix_users_username ON users (username)"},
	{"ix_users_email", "CREATE UNIQUE INDEX IF NOT EXISTS ix_users_email ON users (email)"},
	{"ix_users_api_key", "CREATE UNIQUE INDEX IF NOT EXISTS ix_users_api_key ON users (api_key)"},
	{"ix_urls_short_code", "CREATE UNIQUE INDEX IF NOT EXISTS ix_urls_short_code ON urls (short_code)"},
	{"idx_url_user_created", "CREATE INDEX IF NOT EXISTS idx_url_user_created ON urls (user_id, created_at)"},
	{"idx_url_code_enabled", "CREATE INDEX IF NOT EXISTS idx_url_code_enabled ON urls (short_code, is_enabled)"},
	{"idx_click_url_timestamp", "CREATE INDEX IF NOT EXISTS idx_click_url_timestamp ON clicks (url_id, timestamp)"},
}

// Migrate creates any missing tables, columns and indexes. It is additive only:
// an existing database keeps all of its rows, and columns that are already
// present are left untouched.
func (d *DB) Migrate(ctx context.Context) error {
	// Serialise migrating instances. The checks below are read-then-act, so two
	// replicas starting together can both see a column missing and both try to
	// add it; the loser's error propagates out of run() and exits the process,
	// flapping a rolling deploy. SQLite needs nothing here — it already allows
	// a single connection.
	if d.dialect == Postgres {
		// An arbitrary but stable key, scoped to this application.
		const migrationLockID = 0x7265647278 // "redrx"

		// A session-scoped advisory lock is released only by a pg_advisory_unlock
		// on the same connection that acquired it. Going through the pool could
		// unlock on a different connection — leaving the real lock held for the
		// life of the process and blocking the next replica's migration — so pin
		// one connection for both. It stays open for the whole migration, so the
		// lock is held throughout even though the DDL below runs on the pool.
		conn, err := d.Conn(ctx)
		if err != nil {
			return fmt.Errorf("acquire migration connection: %w", err)
		}
		defer func() { _ = conn.Close() }()

		if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
			return fmt.Errorf("acquire migration lock: %w", err)
		}
		defer func() {
			if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", migrationLockID); err != nil {
				// The lock is released when the pinned connection closes regardless.
				_ = err
			}
		}()
	}

	for _, t := range schema {
		exists, err := d.tableExists(ctx, t.name)
		if err != nil {
			return err
		}
		if !exists {
			if _, err := d.ExecContext(ctx, d.createTableDDL(t)); err != nil {
				return fmt.Errorf("create table %s: %w", t.name, err)
			}
			continue
		}
		if err := d.addMissingColumns(ctx, t); err != nil {
			return err
		}
	}

	for _, idx := range indexes {
		if _, err := d.ExecContext(ctx, idx.ddl); err != nil {
			return fmt.Errorf("create index %s: %w", idx.name, err)
		}
	}
	return nil
}

func (d *DB) createTableDDL(t table) string {
	parts := make([]string, 0, len(t.columns)+len(t.extra))
	for _, c := range t.columns {
		typ := c.sqliteType
		if d.dialect == Postgres {
			typ = c.pgType
		}
		parts = append(parts, fmt.Sprintf("%s %s", c.name, typ))
	}
	parts = append(parts, t.extra...)
	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n\t%s\n)", t.name, strings.Join(parts, ",\n\t"))
}

func (d *DB) addMissingColumns(ctx context.Context, t table) error {
	existing, err := d.columnNames(ctx, t.name)
	if err != nil {
		return err
	}
	for _, c := range t.columns {
		if _, ok := existing[c.name]; ok {
			continue
		}
		typ := c.sqliteType
		if d.dialect == Postgres {
			typ = c.pgType
		}
		// A column added to a populated table cannot carry PRIMARY KEY or a
		// NOT NULL without default, so strip those qualifiers.
		typ = stripUnaddableQualifiers(typ)
		stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", t.name, c.name, typ)
		if _, err := d.ExecContext(ctx, stmt); err != nil {
			// Another instance may have added it between the inspection above
			// and this statement. That is the outcome we wanted either way.
			if isDuplicateColumn(err) {
				continue
			}
			return fmt.Errorf("add column %s.%s: %w", t.name, c.name, err)
		}
	}
	return nil
}

func stripUnaddableQualifiers(typ string) string {
	for _, bad := range []string{"PRIMARY KEY", "AUTOINCREMENT", "NOT NULL", "SERIAL"} {
		typ = strings.ReplaceAll(typ, bad, "")
	}
	typ = strings.Join(strings.Fields(typ), " ")
	if typ == "" {
		typ = "TEXT"
	}
	return typ
}

func (d *DB) tableExists(ctx context.Context, name string) (bool, error) {
	var query string
	if d.dialect == SQLite {
		query = "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?"
	} else {
		query = "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=?"
	}
	var n int
	if err := d.QueryRow(ctx, query, name).Scan(&n); err != nil {
		return false, fmt.Errorf("check table %s: %w", name, err)
	}
	return n > 0, nil
}

func (d *DB) columnNames(ctx context.Context, table string) (map[string]struct{}, error) {
	out := map[string]struct{}{}

	if d.dialect == SQLite {
		// PRAGMA does not accept bind parameters; the table name is a package
		// constant, never user input.
		rows, err := d.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
		if err != nil {
			return nil, fmt.Errorf("inspect %s: %w", table, err)
		}
		defer rows.Close()
		for rows.Next() {
			var (
				cid       int
				name, typ string
				notNull   int
				dfltValue any
				pk        int
			)
			if err := rows.Scan(&cid, &name, &typ, &notNull, &dfltValue, &pk); err != nil {
				return nil, fmt.Errorf("inspect %s: %w", table, err)
			}
			out[name] = struct{}{}
		}
		return out, rows.Err()
	}

	rows, err := d.Query(ctx,
		"SELECT column_name FROM information_schema.columns WHERE table_schema=current_schema() AND table_name=?",
		table)
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("inspect %s: %w", table, err)
		}
		out[name] = struct{}{}
	}
	return out, rows.Err()
}

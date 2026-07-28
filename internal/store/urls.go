package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const urlColumns = `id, user_id, short_code, long_url, COALESCE(rotate_targets, ''),
	COALESCE(ios_target_url, ''), COALESCE(android_target_url, ''), COALESCE(password_hash, ''),
	preview_mode, stats_enabled, is_enabled, COALESCE(clicks, 0),
	COALESCE(qr_color, ''), COALESCE(qr_background, ''),
	created_at, expires_at, start_at, end_at, last_accessed_at`

func scanURL(row interface{ Scan(...any) error }) (*URL, error) {
	var (
		u                              URL
		userID                         sql.NullInt64
		rotateRaw                      string
		preview, stats, enabled        nullBool
		createdAt, expiresAt           NullTime
		startAt, endAt, lastAccessedAt NullTime
	)
	err := row.Scan(
		&u.ID, &userID, &u.ShortCode, &u.LongURL, &rotateRaw,
		&u.IOSTargetURL, &u.AndroidTargetURL, &u.PasswordHash,
		&preview, &stats, &enabled, &u.ClicksCount,
		&u.QRColor, &u.QRBackground,
		&createdAt, &expiresAt, &startAt, &endAt, &lastAccessedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if userID.Valid {
		id := userID.Int64
		u.UserID = &id
	}
	u.RotateTargets = decodeRotateTargets(rotateRaw)
	// The Python models defaulted these to true; NULL rows predate the columns.
	u.PreviewMode = preview.orDefault(true)
	u.StatsEnabled = stats.orDefault(true)
	u.IsEnabled = enabled.orDefault(true)
	u.CreatedAt = createdAt.Time
	u.ExpiresAt = expiresAt.Ptr()
	u.StartAt = startAt.Ptr()
	u.EndAt = endAt.Ptr()
	u.LastAccessedAt = lastAccessedAt.Ptr()
	return &u, nil
}

func (d *DB) URLByShortCode(ctx context.Context, code string) (*URL, error) {
	// A NUL byte cannot occur in a stored code — short codes are alphanumerics —
	// and Postgres rejects it in a text parameter outright, turning what should
	// be a plain miss into a query error and a 500. Every lookup path funnels
	// through here, so guarding once covers the redirect, stats, QR and API
	// handlers. Treat it as simply not found.
	if strings.IndexByte(code, 0) >= 0 {
		return nil, ErrNotFound
	}
	return scanURL(d.QueryRow(ctx, "SELECT "+urlColumns+" FROM urls WHERE short_code = ?", code))
}

func (d *DB) ShortCodeTaken(ctx context.Context, code string) (bool, error) {
	return d.exists(ctx, "SELECT COUNT(*) FROM urls WHERE short_code = ?", code)
}

func (d *DB) CreateURL(ctx context.Context, u *URL) error {
	createdAt := NewTime(d.dialect, now())
	u.CreatedAt = createdAt.Time

	const q = `INSERT INTO urls (
		user_id, short_code, long_url, rotate_targets, ios_target_url, android_target_url,
		password_hash, preview_mode, stats_enabled, is_enabled, clicks,
		qr_color, qr_background,
		created_at, expires_at, start_at, end_at, last_accessed_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	id, err := d.insertReturningID(ctx, q, "urls",
		nullInt64(u.UserID), u.ShortCode, u.LongURL, encodeRotateTargets(u.RotateTargets),
		nullString(u.IOSTargetURL), nullString(u.AndroidTargetURL), nullString(u.PasswordHash),
		u.PreviewMode, u.StatsEnabled, u.IsEnabled, u.ClicksCount,
		nullString(u.QRColor), nullString(u.QRBackground),
		createdAt, NewNullTime(d.dialect, u.ExpiresAt), NewNullTime(d.dialect, u.StartAt),
		NewNullTime(d.dialect, u.EndAt), NewNullTime(d.dialect, u.LastAccessedAt),
	)
	if err != nil {
		return fmt.Errorf("create url: %w", err)
	}
	u.ID = id
	return nil
}

// UpdateURL persists the fields the edit form can change.
//
// is_enabled is deliberately absent: no edit-form field sets it, so writing it
// back would carry whatever value was read when the form was opened and undo a
// toggle made from the dashboard in the meantime.
func (d *DB) UpdateURL(ctx context.Context, u *URL) error {
	const q = `UPDATE urls SET
		long_url = ?, ios_target_url = ?, android_target_url = ?, rotate_targets = ?,
		preview_mode = ?, stats_enabled = ?, expires_at = ?
		WHERE id = ?`
	_, err := d.Exec(ctx, q,
		u.LongURL, nullString(u.IOSTargetURL), nullString(u.AndroidTargetURL),
		encodeRotateTargets(u.RotateTargets),
		u.PreviewMode, u.StatsEnabled,
		NewNullTime(d.dialect, u.ExpiresAt), u.ID)
	return err
}

func (d *DB) SetURLEnabled(ctx context.Context, id int64, enabled bool) error {
	_, err := d.Exec(ctx, "UPDATE urls SET is_enabled = ? WHERE id = ?", enabled, id)
	return err
}

func (d *DB) TouchLastAccessed(ctx context.Context, id int64, at time.Time) error {
	_, err := d.Exec(ctx, "UPDATE urls SET last_accessed_at = ? WHERE id = ?",
		NewTime(d.dialect, at), id)
	return err
}

// DeleteURL removes a link and its click history in one transaction, so an
// interruption between the two statements cannot leave the link live with its
// history gone — which is the opposite of what every caller intends.
func (d *DB) DeleteURL(ctx context.Context, id int64) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, d.rebind("DELETE FROM clicks WHERE url_id = ?"), id); err != nil {
		return fmt.Errorf("delete clicks: %w", err)
	}
	if _, err := tx.ExecContext(ctx, d.rebind("DELETE FROM urls WHERE id = ?"), id); err != nil {
		return fmt.Errorf("delete url: %w", err)
	}
	return tx.Commit()
}

// SetURLEnabledToggle flips is_enabled in a single statement and reports the new
// value. Read-modify-write across two requests loses updates: two toggles racing
// both read the old value and both write the same new one, so the link ends up
// in the state one click should have produced, and the JSON response describes a
// state the row may not be in.
func (d *DB) SetURLEnabledToggle(ctx context.Context, id int64) (bool, error) {
	q := "UPDATE urls SET is_enabled = NOT COALESCE(is_enabled, ?) WHERE id = ?"
	if d.dialect == SQLite {
		// SQLite has no NOT operator for its integer booleans in this position.
		q = "UPDATE urls SET is_enabled = CASE WHEN COALESCE(is_enabled, ?) THEN 0 ELSE 1 END WHERE id = ?"
	}
	if _, err := d.Exec(ctx, q, true, id); err != nil {
		return false, err
	}

	var enabled nullBool
	if err := d.QueryRow(ctx, "SELECT is_enabled FROM urls WHERE id = ?", id).Scan(&enabled); err != nil {
		return false, err
	}
	return enabled.orDefault(true), nil
}

// deleteBatchSize caps how many ids go into one IN clause, so a large
// multi-select stays within the driver's bind-parameter ceiling (SQLite is a
// few hundred to ~32k depending on the build; Postgres is 65535). Each id costs
// one parameter and the query adds one for user_id, so 500 is safe everywhere.
const deleteBatchSize = 500

// DeleteUserURLs removes the given links, ignoring any not owned by the user.
// It returns the number of links actually deleted. Everything runs in one
// transaction, matching DeleteURL, so an interruption cannot leave a link's
// click history deleted while the link itself survives.
func (d *DB) DeleteUserURLs(ctx context.Context, userID int64, ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var deleted int64
	for start := 0; start < len(ids); start += deleteBatchSize {
		end := start + deleteBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]

		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")
		args := make([]any, 0, len(chunk)+1)
		for _, id := range chunk {
			args = append(args, id)
		}
		args = append(args, userID)

		// Clear the child rows first; SQLite files created by SQLAlchemy have no
		// ON DELETE CASCADE on this foreign key.
		if _, err := tx.ExecContext(ctx, d.rebind(fmt.Sprintf(
			"DELETE FROM clicks WHERE url_id IN (SELECT id FROM urls WHERE id IN (%s) AND user_id = ?)",
			placeholders)), args...); err != nil {
			return 0, err
		}

		res, err := tx.ExecContext(ctx, d.rebind(fmt.Sprintf(
			"DELETE FROM urls WHERE id IN (%s) AND user_id = ?", placeholders)), args...)
		if err != nil {
			return 0, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, err
		}
		deleted += n
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deleted, nil
}

// UserURLs returns one page of a user's links, newest first.
func (d *DB) UserURLs(ctx context.Context, userID int64, limit, offset int) ([]*URL, error) {
	rows, err := d.Query(ctx, "SELECT "+urlColumns+
		" FROM urls WHERE user_id = ? ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?",
		userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectURLs(rows)
}

// AllUserURLs returns every link owned by a user, for CSV export.
func (d *DB) AllUserURLs(ctx context.Context, userID int64) ([]*URL, error) {
	rows, err := d.Query(ctx, "SELECT "+urlColumns+
		" FROM urls WHERE user_id = ? ORDER BY created_at DESC, id DESC", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectURLs(rows)
}

func collectURLs(rows *sql.Rows) ([]*URL, error) {
	var out []*URL
	for rows.Next() {
		u, err := scanURL(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// DashboardStats is the summary shown above the link table.
type DashboardStats struct {
	TotalLinks   int64
	TotalClicks  int64
	ActiveLinks  int64
	TopPerformer *URL
}

func (d *DB) DashboardStats(ctx context.Context, userID int64) (*DashboardStats, error) {
	var s DashboardStats

	if err := d.QueryRow(ctx,
		"SELECT COUNT(*), COALESCE(SUM(clicks), 0) FROM urls WHERE user_id = ?", userID,
	).Scan(&s.TotalLinks, &s.TotalClicks); err != nil {
		return nil, err
	}

	// COALESCE to match scanURL, which reads a NULL is_enabled as true. Migrate
	// adds the column to an older table with no default, so every pre-existing
	// row is NULL — and `NULL = TRUE` is unknown, which would report 0 active
	// links while every row in the table below showed as enabled.
	if err := d.QueryRow(ctx,
		"SELECT COUNT(*) FROM urls WHERE user_id = ? AND COALESCE(is_enabled, ?) = ?", userID, true, true,
	).Scan(&s.ActiveLinks); err != nil {
		return nil, err
	}

	top, err := scanURL(d.QueryRow(ctx, "SELECT "+urlColumns+
		" FROM urls WHERE user_id = ? ORDER BY clicks DESC, id ASC LIMIT 1", userID))
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if err == nil {
		s.TopPerformer = top
	}
	return &s, nil
}

// EachURL streams every link, for the phishing sweep.
func (d *DB) EachURL(ctx context.Context, fn func(*URL) error) error {
	rows, err := d.Query(ctx, "SELECT "+urlColumns+" FROM urls ORDER BY id")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		u, err := scanURL(rows)
		if err != nil {
			return err
		}
		if err := fn(u); err != nil {
			return err
		}
	}
	return rows.Err()
}

func nullInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

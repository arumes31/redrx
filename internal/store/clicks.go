package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// RecordClick appends a click row and bumps the counter on the link. Both
// writes happen in one transaction so the counter can never drift from the log.
func (d *DB) RecordClick(ctx context.Context, c *Click) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if c.Timestamp.IsZero() {
		c.Timestamp = now()
	}

	const insert = `INSERT INTO clicks (url_id, timestamp, ip_address, country, browser, platform, referrer)
	                VALUES (?, ?, ?, ?, ?, ?, ?)`
	if _, err := tx.ExecContext(ctx, d.rebind(insert),
		c.URLID, NewTime(d.dialect, c.Timestamp), c.IPAddress, c.Country,
		c.Browser, c.Platform, c.Referrer); err != nil {
		return fmt.Errorf("insert click: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		d.rebind("UPDATE urls SET clicks = COALESCE(clicks, 0) + 1 WHERE id = ?"),
		c.URLID); err != nil {
		return fmt.Errorf("increment click counter: %w", err)
	}

	return tx.Commit()
}

// RecentClicks returns the newest clicks for a link.
func (d *DB) RecentClicks(ctx context.Context, urlID int64, limit int) ([]*Click, error) {
	rows, err := d.Query(ctx,
		`SELECT id, url_id, timestamp, COALESCE(ip_address, ''), COALESCE(country, 'Unknown'),
		        COALESCE(browser, ''), COALESCE(platform, ''), COALESCE(referrer, 'Direct')
		 FROM clicks WHERE url_id = ? ORDER BY timestamp DESC, id DESC LIMIT ?`, urlID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Click
	for rows.Next() {
		var (
			c  Click
			ts NullTime
		)
		if err := rows.Scan(&c.ID, &c.URLID, &ts, &c.IPAddress, &c.Country,
			&c.Browser, &c.Platform, &c.Referrer); err != nil {
			return nil, err
		}
		c.Timestamp = ts.Time
		out = append(out, &c)
	}
	return out, rows.Err()
}

// Bucket is one label/count pair from an aggregation query.
type Bucket struct {
	Label string
	Count int64
}

// ClicksByTimeBucket groups clicks into hourly or daily buckets since cutoff.
// hourly selects the '%H:00' bucket used by the 24h range; otherwise days.
func (d *DB) ClicksByTimeBucket(ctx context.Context, urlID int64, cutoff time.Time, hourly bool) ([]Bucket, error) {
	var expr string
	switch {
	case d.dialect == SQLite && hourly:
		expr = "strftime('%H:00', timestamp)"
	case d.dialect == SQLite:
		expr = "strftime('%Y-%m-%d', timestamp)"
	case hourly:
		expr = "to_char(timestamp, 'HH24:00')"
	default:
		expr = "to_char(timestamp, 'YYYY-MM-DD')"
	}

	rows, err := d.Query(ctx, fmt.Sprintf(
		"SELECT %s, COUNT(id) FROM clicks WHERE url_id = ? AND timestamp >= ? GROUP BY %s",
		expr, expr), urlID, NewTime(d.dialect, cutoff))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectBuckets(rows, "")
}

// ClicksGroupedBy aggregates clicks over one of the categorical columns.
func (d *DB) ClicksGroupedBy(ctx context.Context, urlID int64, cutoff time.Time, column string) ([]Bucket, error) {
	// Only the fixed set of analytics columns is ever grouped on; anything else
	// is a programming error, not user input.
	switch column {
	case "country", "browser", "platform", "referrer":
	default:
		return nil, fmt.Errorf("store: cannot group clicks by %q", column)
	}

	fallback := "Unknown"
	if column == "referrer" {
		fallback = "Direct"
	}

	rows, err := d.Query(ctx, fmt.Sprintf(
		"SELECT %s, COUNT(id) FROM clicks WHERE url_id = ? AND timestamp >= ? GROUP BY %s",
		column, column), urlID, NewTime(d.dialect, cutoff))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectBuckets(rows, fallback)
}

func collectBuckets(rows *sql.Rows, fallback string) ([]Bucket, error) {
	var out []Bucket
	for rows.Next() {
		var (
			label sql.NullString
			count int64
		)
		if err := rows.Scan(&label, &count); err != nil {
			return nil, err
		}
		name := label.String
		if !label.Valid || name == "" {
			name = fallback
		}
		out = append(out, Bucket{Label: name, Count: count})
	}
	return out, rows.Err()
}

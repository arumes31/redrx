package store

import (
	"encoding/json"
	"time"
)

// User mirrors the `users` table.
type User struct {
	ID           int64
	Username     string
	Email        string
	PasswordHash string
	APIKey       string
	CreatedAt    time.Time
}

// URL mirrors the `urls` table. RotateTargets is stored as a JSON array in the
// `rotate_targets` TEXT column, exactly as the Python model wrote it.
type URL struct {
	ID               int64
	UserID           *int64
	ShortCode        string
	LongURL          string
	RotateTargets    []string
	IOSTargetURL     string
	AndroidTargetURL string
	PasswordHash     string
	PreviewMode      bool
	StatsEnabled     bool
	IsEnabled        bool
	ClicksCount      int64
	CreatedAt        time.Time
	ExpiresAt        *time.Time
	StartAt          *time.Time
	EndAt            *time.Time
	LastAccessedAt   *time.Time
}

// IsActive reports whether the link should currently redirect, applying the
// enable flag, the scheduling window and the expiry.
func (u *URL) IsActive() bool {
	if !u.IsEnabled {
		return false
	}
	now := time.Now().UTC()
	if u.StartAt != nil && now.Before(*u.StartAt) {
		return false
	}
	if u.EndAt != nil && now.After(*u.EndAt) {
		return false
	}
	if u.ExpiresAt != nil && now.After(*u.ExpiresAt) {
		return false
	}
	return true
}

// IsPasswordProtected reports whether the link requires a password.
func (u *URL) IsPasswordProtected() bool { return u.PasswordHash != "" }

// Click mirrors the `clicks` table.
type Click struct {
	ID        int64
	URLID     int64
	Timestamp time.Time
	IPAddress string
	Country   string
	Browser   string
	Platform  string
	Referrer  string
}

// encodeRotateTargets renders the JSON stored in `urls.rotate_targets`. An
// empty list becomes SQL NULL, matching the Python property setter.
func encodeRotateTargets(targets []string) any {
	if len(targets) == 0 {
		return nil
	}
	b, err := json.Marshal(targets)
	if err != nil {
		return nil
	}
	return string(b)
}

// decodeRotateTargets parses `urls.rotate_targets`. Unparseable values yield an
// empty list rather than an error, so one bad row cannot break a page.
func decodeRotateTargets(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

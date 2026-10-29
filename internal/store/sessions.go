package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

func sessionIDHash(sid string) string {
	digest := sha256.Sum256([]byte(sid))
	return hex.EncodeToString(digest[:])
}

// RevokeSession records a logged-out session id so a retained copy of its
// cookie stops working. expiresAt is when the cookie would have lapsed anyway;
// the row is purged after that.
func (d *DB) RevokeSession(ctx context.Context, sid string, expiresAt time.Time) error {
	if sid == "" {
		return nil
	}
	if _, err := d.Exec(ctx, "DELETE FROM revoked_sessions WHERE expires_at <= ?",
		NewTime(d.dialect, now())); err != nil {
		return fmt.Errorf("delete expired revoked sessions: %w", err)
	}
	if _, err := d.Exec(ctx, `INSERT INTO revoked_sessions (sid_hash, expires_at)
		VALUES (?, ?) ON CONFLICT (sid_hash) DO NOTHING`,
		sessionIDHash(sid), NewTime(d.dialect, expiresAt)); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

// SessionRevoked reports whether sid was ended by RevokeSession.
func (d *DB) SessionRevoked(ctx context.Context, sid string) (bool, error) {
	return d.exists(ctx, "SELECT COUNT(*) FROM revoked_sessions WHERE sid_hash = ?", sessionIDHash(sid))
}

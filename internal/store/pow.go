package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// ConsumePoWChallenge atomically records a solved challenge and reports
// whether this caller was the first to use it. Only a digest is retained.
func (d *DB) ConsumePoWChallenge(
	ctx context.Context,
	challenge string,
	expiresAt time.Time,
) (bool, error) {
	if challenge == "" {
		return false, nil
	}

	currentTime := now()
	if _, err := d.Exec(ctx, "DELETE FROM used_pow_challenges WHERE expires_at <= ?",
		NewTime(d.dialect, currentTime)); err != nil {
		return false, fmt.Errorf("delete expired proof-of-work challenges: %w", err)
	}

	digest := sha256.Sum256([]byte(challenge))
	result, err := d.Exec(ctx, `INSERT INTO used_pow_challenges (challenge_hash, expires_at)
		VALUES (?, ?) ON CONFLICT (challenge_hash) DO NOTHING`,
		hex.EncodeToString(digest[:]), NewTime(d.dialect, expiresAt))
	if err != nil {
		return false, fmt.Errorf("consume proof-of-work challenge: %w", err)
	}

	inserted, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check proof-of-work challenge consumption: %w", err)
	}
	return inserted == 1, nil
}

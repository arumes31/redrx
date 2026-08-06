package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// ClaimPoWChallenge atomically records a challenge digest until its signed
// expiry. It returns false when another request or server instance claimed the
// same challenge first.
func (d *DB) ClaimPoWChallenge(ctx context.Context, challenge string, expiresAt time.Time) (bool, error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, d.rebind(
		"DELETE FROM pow_challenge_claims WHERE expires_at <= ?"), NewTime(d.dialect, now())); err != nil {
		return false, fmt.Errorf("delete expired proof-of-work claims: %w", err)
	}

	digest := sha256.Sum256([]byte(challenge))
	res, err := tx.ExecContext(ctx, d.rebind(
		"INSERT INTO pow_challenge_claims (challenge_hash, expires_at) VALUES (?, ?) "+
			"ON CONFLICT (challenge_hash) DO NOTHING"),
		hex.EncodeToString(digest[:]), NewTime(d.dialect, expiresAt))
	if err != nil {
		return false, fmt.Errorf("claim proof-of-work challenge: %w", err)
	}
	claimed, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read proof-of-work claim result: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit proof-of-work claim: %w", err)
	}
	return claimed == 1, nil
}

package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
)

const userColumns = `id, username, email, password_hash, COALESCE(api_key, ''),
	COALESCE(totp_secret, ''), totp_enabled, created_at, COALESCE(session_epoch, 0)`

// apiKeyHashLen is the length of a hex-encoded SHA-256 digest.
const apiKeyHashLen = sha256.Size * 2

// HashAPIKey returns the form an API key is stored and looked up in. Keys are
// random UUIDs, so an unsalted fast hash is enough: there is nothing to
// brute-force, and lookup by equality keeps working.
func HashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	var (
		u           User
		createdAt   NullTime
		totpEnabled nullBool
	)
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.APIKey,
		&u.TOTPSecret, &totpEnabled, &createdAt, &u.SessionEpoch)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.CreatedAt = createdAt.Time
	u.TOTPEnabled = totpEnabled.orDefault(false)
	return &u, nil
}

func (d *DB) UserByID(ctx context.Context, id int64) (*User, error) {
	return scanUser(d.QueryRow(ctx, "SELECT "+userColumns+" FROM users WHERE id = ?", id))
}

// UserByLogin resolves the "username or email" login field. When the same
// string is one account's username and another's email, the username match
// wins, so the result is deterministic rather than whichever row the planner
// returned first.
func (d *DB) UserByLogin(ctx context.Context, login string) (*User, error) {
	return scanUser(d.QueryRow(ctx,
		"SELECT "+userColumns+" FROM users WHERE username = ? OR email = ? "+
			"ORDER BY CASE WHEN username = ? THEN 0 ELSE 1 END LIMIT 1",
		login, login, login))
}

// UserByAPIKey resolves a plaintext key presented by an API client.
func (d *DB) UserByAPIKey(ctx context.Context, key string) (*User, error) {
	if key == "" {
		return nil, ErrNotFound
	}
	return scanUser(d.QueryRow(ctx, "SELECT "+userColumns+" FROM users WHERE api_key = ?", HashAPIKey(key)))
}

// UsernameTaken and EmailTaken back the registration form's uniqueness checks.
//
// A username is rejected when it collides with an existing username OR email:
// UserByLogin matches on either column, so allowing a username equal to some
// account's email would make that login string ambiguous.
func (d *DB) UsernameTaken(ctx context.Context, username string) (bool, error) {
	return d.exists(ctx, "SELECT COUNT(*) FROM users WHERE username = ? OR email = ?", username, username)
}

func (d *DB) EmailTaken(ctx context.Context, email string) (bool, error) {
	return d.exists(ctx, "SELECT COUNT(*) FROM users WHERE email = ?", email)
}

func (d *DB) exists(ctx context.Context, query string, args ...any) (bool, error) {
	var n int
	if err := d.QueryRow(ctx, query, args...).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

func (d *DB) CreateUser(ctx context.Context, u *User) error {
	createdAt := NewTime(d.dialect, now())
	u.CreatedAt = createdAt.Time

	// u.APIKey arrives as the plaintext key and is stored hashed.
	var apiKey any
	if u.APIKey != "" {
		u.APIKey = HashAPIKey(u.APIKey)
		apiKey = u.APIKey
	}

	const q = `INSERT INTO users (username, email, password_hash, api_key, created_at)
	           VALUES (?, ?, ?, ?, ?)`

	id, err := d.insertReturningID(ctx, q, "users", u.Username, u.Email, u.PasswordHash, apiKey, createdAt)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	u.ID = id
	return nil
}

// SetAPIKey stores the hash of a newly generated plaintext key.
func (d *DB) SetAPIKey(ctx context.Context, userID int64, key string) error {
	_, err := d.Exec(ctx, "UPDATE users SET api_key = ? WHERE id = ?", HashAPIKey(key), userID)
	return err
}

// BumpSessionEpoch invalidates every existing session cookie of the user and
// returns the new epoch.
func (d *DB) BumpSessionEpoch(ctx context.Context, userID int64) (int64, error) {
	if _, err := d.Exec(ctx,
		"UPDATE users SET session_epoch = COALESCE(session_epoch, 0) + 1 WHERE id = ?", userID); err != nil {
		return 0, err
	}
	var epoch int64
	err := d.QueryRow(ctx, "SELECT COALESCE(session_epoch, 0) FROM users WHERE id = ?", userID).Scan(&epoch)
	return epoch, err
}

// ConsumeTOTPStep records a successfully verified TOTP time-step. It reports
// false when that step (or a later one) was already used, so one code cannot
// complete two logins. The conditional UPDATE makes it race-free.
func (d *DB) ConsumeTOTPStep(ctx context.Context, userID, step int64) (bool, error) {
	res, err := d.Exec(ctx,
		"UPDATE users SET totp_last_step = ? WHERE id = ? AND (totp_last_step IS NULL OR totp_last_step < ?)",
		step, userID, step)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (d *DB) SetUserPasswordHash(ctx context.Context, userID int64, hash string) error {
	_, err := d.Exec(ctx, "UPDATE users SET password_hash = ? WHERE id = ?", hash, userID)
	return err
}

// SetTOTPPending stores an encrypted enrollment secret without enabling 2FA.
func (d *DB) SetTOTPPending(ctx context.Context, userID int64, encryptedSecret string) error {
	_, err := d.Exec(ctx,
		"UPDATE users SET totp_secret = ?, totp_enabled = ?, totp_last_step = NULL WHERE id = ?",
		encryptedSecret, false, userID)
	return err
}

// EnableTOTP atomically enables TOTP and replaces the one-time recovery codes.
// It also bumps the session epoch, so sessions opened before 2FA was turned on
// (possibly by whoever prompted the user to enable it) stop working.
func (d *DB) EnableTOTP(ctx context.Context, userID int64, codeHashes []string) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, d.rebind(
		"UPDATE users SET totp_enabled = ?, session_epoch = COALESCE(session_epoch, 0) + 1 WHERE id = ?"),
		true, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, d.rebind(
		"DELETE FROM recovery_codes WHERE user_id = ?"), userID); err != nil {
		return err
	}
	createdAt := NewTime(d.dialect, now())
	for _, hash := range codeHashes {
		if _, err := tx.ExecContext(ctx, d.rebind(
			"INSERT INTO recovery_codes (user_id, code_hash, created_at) VALUES (?, ?, ?)"),
			userID, hash, createdAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DisableTOTP clears the secret and every unused recovery code together, and
// revokes existing sessions like EnableTOTP does.
func (d *DB) DisableTOTP(ctx context.Context, userID int64) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, d.rebind(
		"UPDATE users SET totp_secret = NULL, totp_enabled = ?, totp_last_step = NULL, "+
			"session_epoch = COALESCE(session_epoch, 0) + 1 WHERE id = ?"), false, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, d.rebind(
		"DELETE FROM recovery_codes WHERE user_id = ?"), userID); err != nil {
		return err
	}
	return tx.Commit()
}

// ConsumeRecoveryCode removes a matching code in one statement, so it can
// never succeed twice even when two login attempts race.
func (d *DB) ConsumeRecoveryCode(ctx context.Context, userID int64, codeHash string) (bool, error) {
	res, err := d.Exec(ctx,
		"DELETE FROM recovery_codes WHERE user_id = ? AND code_hash = ?", userID, codeHash)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// insertReturningID runs an INSERT and reports the new primary key, using
// whichever mechanism the backend provides.
func (d *DB) insertReturningID(ctx context.Context, query, table string, args ...any) (int64, error) {
	if d.dialect == Postgres {
		var id int64
		err := d.QueryRow(ctx, query+" RETURNING id", args...).Scan(&id)
		return id, err
	}
	res, err := d.Exec(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"
)

const recoveryAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"

// SealAccountSecret encrypts account security material with an AES-GCM key
// derived from the configured application secret.
func SealAccountSecret(secretKey []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(deriveAccountKey(secretKey))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), []byte("redrx.totp.v1"))
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

// OpenAccountSecret decrypts a value written by SealAccountSecret.
func OpenAccountSecret(secretKey []byte, encoded string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(deriveAccountKey(secretKey))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("security: encrypted account secret is truncated")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	opened, err := gcm.Open(nil, nonce, ciphertext, []byte("redrx.totp.v1"))
	if err != nil {
		return "", errors.New("security: encrypted account secret is invalid")
	}
	return string(opened), nil
}

func deriveAccountKey(secretKey []byte) []byte {
	sum := sha256.Sum256(append([]byte("redrx.account-secrets.v1|"), secretKey...))
	return sum[:]
}

// GenerateRecoveryCodes returns high-entropy codes intended to be displayed
// once. Ambiguous characters are omitted to make manual entry less error-prone.
func GenerateRecoveryCodes(count int) ([]string, error) {
	if count < 1 || count > 100 {
		return nil, errors.New("security: invalid recovery-code count")
	}
	out := make([]string, count)
	limit := big.NewInt(int64(len(recoveryAlphabet)))
	for i := range out {
		b := make([]byte, 12)
		for j := range b {
			n, err := rand.Int(rand.Reader, limit)
			if err != nil {
				return nil, err
			}
			b[j] = recoveryAlphabet[n.Int64()]
		}
		out[i] = string(b[:4]) + "-" + string(b[4:8]) + "-" + string(b[8:])
	}
	return out, nil
}

// RecoveryCodeHash creates the keyed digest persisted in the database.
func RecoveryCodeHash(secretKey []byte, code string) string {
	key := sha256.Sum256(append([]byte("redrx.recovery-codes.v1|"), secretKey...))
	mac := hmac.New(sha256.New, key[:])
	mac.Write([]byte(normalizeRecoveryCode(code)))
	return hex.EncodeToString(mac.Sum(nil))
}

func normalizeRecoveryCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	return strings.NewReplacer("-", "", " ", "").Replace(code)
}

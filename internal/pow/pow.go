// Package pow implements short-lived proof-of-work challenges for anonymous
// link creation. Challenges are stateless and authenticated by the server.
package pow

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

var ErrInvalid = errors.New("proof of work is invalid or expired")

// Issue returns a signed challenge valid for ttl.
func Issue(secret []byte, ttl time.Duration) (string, error) {
	nonce := make([]byte, 18)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	payload := strconv.FormatInt(time.Now().Add(ttl).Unix(), 10) + "." +
		base64.RawURLEncoding.EncodeToString(nonce)
	return payload + "." + sign(secret, payload), nil
}

// Verify checks the challenge signature, expiry, numeric solution and required
// number of leading zero bits in SHA-256(challenge + ":" + solution).
func Verify(secret []byte, challenge, solution string, difficulty int) error {
	_, err := VerifyWithExpiry(secret, challenge, solution, difficulty)
	return err
}

// VerifyWithExpiry verifies a challenge and returns its authenticated expiry.
func VerifyWithExpiry(secret []byte, challenge, solution string, difficulty int) (time.Time, error) {
	if difficulty < 1 || difficulty > 28 || len(challenge) > 256 || len(solution) > 20 {
		return time.Time{}, ErrInvalid
	}
	expRaw, rest, ok := strings.Cut(challenge, ".")
	if !ok {
		return time.Time{}, ErrInvalid
	}
	_, sig, ok := strings.Cut(rest, ".")
	if !ok {
		return time.Time{}, ErrInvalid
	}
	payload := expRaw + "." + strings.TrimSuffix(rest, "."+sig)
	want, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || subtle.ConstantTimeCompare(want, signature(secret, payload)) != 1 {
		return time.Time{}, ErrInvalid
	}
	exp, err := strconv.ParseInt(expRaw, 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return time.Time{}, ErrInvalid
	}
	if solution == "" {
		return time.Time{}, ErrInvalid
	}
	if _, err := strconv.ParseUint(solution, 10, 64); err != nil {
		return time.Time{}, ErrInvalid
	}
	sum := sha256.Sum256([]byte(challenge + ":" + solution))
	if !hasLeadingZeroBits(sum[:], difficulty) {
		return time.Time{}, ErrInvalid
	}
	// Verify treats the whole Unix second named by exp as valid. Keep the replay
	// claim through that same second so cleanup cannot reopen a tiny replay
	// window at the boundary.
	return time.Unix(exp, 0).Add(time.Second), nil
}

func sign(secret []byte, payload string) string {
	return base64.RawURLEncoding.EncodeToString(signature(secret, payload))
}

func signature(secret []byte, payload string) []byte {
	key := sha256.Sum256(append([]byte("redrx.anonymous-pow.v1|"), secret...))
	mac := hmac.New(sha256.New, key[:])
	mac.Write([]byte(payload))
	return mac.Sum(nil)
}

func hasLeadingZeroBits(hash []byte, bits int) bool {
	for bits >= 8 {
		if hash[0] != 0 {
			return false
		}
		hash = hash[1:]
		bits -= 8
	}
	return bits == 0 || hash[0]>>(8-bits) == 0
}

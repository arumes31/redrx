// Package security implements Werkzeug-compatible password hashing.
//
// Hashes written by the previous Python deployment must keep verifying, and
// hashes written here must stay readable by Werkzeug, so both the parsing and
// the generation follow werkzeug.security's on-disk format exactly:
//
//	method[:params...]$salt$hexdigest
//
// Supported methods are scrypt (Werkzeug >= 2.3 default), pbkdf2 (older
// default) and the legacy bare digest methods.
package security

import (
	"crypto/hmac"
	// md5 and sha1 are here to *verify* hashes Werkzeug wrote years ago, not to
	// create new ones: GeneratePasswordHash only ever emits scrypt. Dropping
	// them would lock out any account still on a legacy digest.
	"crypto/md5" // #nosec G501
	"crypto/rand"
	"crypto/sha1" // #nosec G505
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"math/big"
	"strconv"
	"strings"

	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/crypto/scrypt"
)

const (
	saltChars  = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	saltLength = 16

	// Werkzeug's current scrypt defaults.
	scryptN = 32768
	scryptR = 8
	scryptP = 1
	// Werkzeug passes dklen=64 to hashlib.scrypt.
	scryptKeyLen = 64

	// Bounds on values read out of a stored hash. Our own hashes sit far below
	// these; the caps stop a crafted hash from making a single verify derive a
	// huge key or, since scrypt's memory is ~128*N*r bytes, exhaust memory.
	maxDeriveKeyLen = 128
	maxScryptN      = 1 << 17 // 131072, 4x Werkzeug's default (~128 MB at r=8)
	maxScryptR      = 16
	maxScryptP      = 16
)

var errBadHash = errors.New("security: malformed password hash")

// GenerateSalt returns a Werkzeug-style alphanumeric salt.
func GenerateSalt() (string, error) {
	b := make([]byte, saltLength)
	limit := big.NewInt(int64(len(saltChars)))
	for i := range b {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		b[i] = saltChars[n.Int64()]
	}
	return string(b), nil
}

// GeneratePasswordHash hashes a password in Werkzeug's default scrypt format.
func GeneratePasswordHash(password string) (string, error) {
	salt, err := GenerateSalt()
	if err != nil {
		return "", err
	}
	dk, err := scrypt.Key([]byte(password), []byte(salt), scryptN, scryptR, scryptP, scryptKeyLen)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("scrypt:%d:%d:%d$%s$%s", scryptN, scryptR, scryptP, salt, hex.EncodeToString(dk)), nil
}

// CheckPasswordHash reports whether password matches the stored Werkzeug hash.
// It never returns an error for a wrong password — only false.
func CheckPasswordHash(stored, password string) bool {
	method, salt, want, err := splitHash(stored)
	if err != nil {
		return false
	}
	wantLen := len(want) / 2
	if wantLen > maxDeriveKeyLen {
		// A digest longer than any real hash is a crafted one; refuse it rather
		// than derive that many bytes of key material.
		return false
	}
	got, err := derive(method, salt, password, wantLen)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.ToLower(want)), []byte(hex.EncodeToString(got))) == 1
}

// NeedsRehash reports whether a valid hash uses an outdated method and should
// be upgraded to the current default on the next successful login.
func NeedsRehash(stored string) bool {
	method, _, _, err := splitHash(stored)
	if err != nil {
		return false
	}
	return method != fmt.Sprintf("scrypt:%d:%d:%d", scryptN, scryptR, scryptP)
}

func splitHash(stored string) (method, salt, digest string, err error) {
	parts := strings.Split(stored, "$")
	if len(parts) != 3 || parts[0] == "" || parts[2] == "" {
		return "", "", "", errBadHash
	}
	return parts[0], parts[1], parts[2], nil
}

// derive reproduces werkzeug.security._hash_internal for the given method.
// wantLen is the digest length in bytes implied by the stored hash; it matters
// only for scrypt, whose dklen is not encoded in the method string.
func derive(method, salt, password string, wantLen int) ([]byte, error) {
	switch {
	case strings.HasPrefix(method, "scrypt:"):
		n, r, p, err := parseScryptParams(method)
		if err != nil {
			return nil, err
		}
		// Bound the cost parameters. scrypt's memory use is ~128*N*r bytes, so a
		// crafted hash with a huge N or r would otherwise let one verify exhaust
		// memory. Our own hashes use the fixed defaults, well under these.
		if n < 2 || n > maxScryptN || r < 1 || r > maxScryptR || p < 1 || p > maxScryptP {
			return nil, errBadHash
		}
		if wantLen <= 0 {
			wantLen = scryptKeyLen
		}
		return scrypt.Key([]byte(password), []byte(salt), n, r, p, wantLen)

	case strings.HasPrefix(method, "pbkdf2:"):
		newHash, iter, err := parsePBKDF2Params(method)
		if err != nil {
			return nil, err
		}
		return pbkdf2.Key([]byte(password), []byte(salt), iter, newHash().Size(), newHash), nil

	default:
		// Legacy Werkzeug: bare digest name, optionally HMAC when salted.
		newHash, err := hashByName(method)
		if err != nil {
			return nil, err
		}
		if salt == "" {
			h := newHash()
			h.Write([]byte(password))
			return h.Sum(nil), nil
		}
		mac := hmac.New(newHash, []byte(salt))
		mac.Write([]byte(password))
		return mac.Sum(nil), nil
	}
}

func parseScryptParams(method string) (n, r, p int, err error) {
	fields := strings.Split(method, ":")
	if len(fields) != 4 {
		return 0, 0, 0, errBadHash
	}
	if n, err = strconv.Atoi(fields[1]); err != nil {
		return 0, 0, 0, errBadHash
	}
	if r, err = strconv.Atoi(fields[2]); err != nil {
		return 0, 0, 0, errBadHash
	}
	if p, err = strconv.Atoi(fields[3]); err != nil {
		return 0, 0, 0, errBadHash
	}
	return n, r, p, nil
}

func parsePBKDF2Params(method string) (func() hash.Hash, int, error) {
	// Accepted forms: pbkdf2:sha256, pbkdf2:sha256:600000
	fields := strings.Split(method, ":")
	if len(fields) < 2 || len(fields) > 3 {
		return nil, 0, errBadHash
	}
	newHash, err := hashByName(fields[1])
	if err != nil {
		return nil, 0, err
	}
	iter := 260000 // Werkzeug's historical default when omitted.
	if len(fields) == 3 {
		if iter, err = strconv.Atoi(fields[2]); err != nil || iter <= 0 {
			return nil, 0, errBadHash
		}
	}
	return newHash, iter, nil
}

func hashByName(name string) (func() hash.Hash, error) {
	switch strings.ToLower(name) {
	case "sha256":
		return sha256.New, nil
	case "sha512":
		return sha512.New, nil
	case "sha1":
		return sha1.New, nil
	case "sha224":
		return sha256.New224, nil
	case "sha384":
		return sha512.New384, nil
	case "md5":
		return md5.New, nil
	default:
		return nil, fmt.Errorf("security: unsupported hash method %q", name)
	}
}

// Package shortcode generates and validates short link codes.
package shortcode

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

// alphabet is uppercase alphanumerics. Codes have always been stored uppercase,
// and this stays inside the character set custom codes already allow, so
// existing links and existing validation rules are unaffected.
const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

const (
	MinLength = 3
	MaxLength = 20
)

// validCustom matches the custom-code rule the API has always enforced.
var validCustom = regexp.MustCompile(`^[A-Z0-9_-]+$`)

// Generate returns a cryptographically random code of the given length.
func Generate(length int) (string, error) {
	if length < MinLength {
		length = MinLength
	}
	if length > MaxLength {
		length = MaxLength
	}

	b := make([]byte, length)
	limit := big.NewInt(int64(len(alphabet)))
	for i := range b {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", fmt.Errorf("shortcode: %w", err)
		}
		b[i] = alphabet[n.Int64()]
	}
	return string(b), nil
}

// Normalize applies the canonical form: trimmed and uppercased.
func Normalize(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// ValidateCustom checks a user-supplied code, returning a message suitable for
// showing to the caller.
func ValidateCustom(code string) (string, error) {
	code = Normalize(code)
	if len(code) < MinLength || len(code) > MaxLength {
		return "", fmt.Errorf("custom_code must be between %d and %d characters", MinLength, MaxLength)
	}
	if !validCustom.MatchString(code) {
		return "", fmt.Errorf("custom_code must contain only alphanumeric characters, hyphens, or underscores")
	}
	return code, nil
}

// Taken reports whether a code already exists.
type Taken func(code string) (bool, error)

// GenerateUnique retries until it finds a code no existing link uses.
func GenerateUnique(length int, taken Taken) (string, error) {
	for attempt := 0; attempt < 20; attempt++ {
		code, err := Generate(length)
		if err != nil {
			return "", err
		}
		used, err := taken(code)
		if err != nil {
			return "", err
		}
		if !used {
			return code, nil
		}
	}
	return "", fmt.Errorf("shortcode: could not find an unused code of length %d", length)
}

package pow

import (
	"crypto/sha256"
	"strconv"
	"testing"
	"time"
)

func TestIssueAndVerify(t *testing.T) {
	secret := []byte("test-secret")
	challenge, err := Issue(secret, time.Minute)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	const difficulty = 8
	solution := ""
	for n := uint64(0); n < 1<<20; n++ {
		candidate := strconv.FormatUint(n, 10)
		sum := sha256.Sum256([]byte(challenge + ":" + candidate))
		if hasLeadingZeroBits(sum[:], difficulty) {
			solution = candidate
			break
		}
	}
	if solution == "" {
		t.Fatal("failed to find a low-difficulty solution")
	}
	if err := Verify(secret, challenge, solution, difficulty); err != nil {
		t.Fatalf("Verify valid solution: %v", err)
	}
	if err := Verify([]byte("other-key"), challenge, solution, difficulty); !errorsIsInvalid(err) {
		t.Errorf("Verify with wrong key = %v, want ErrInvalid", err)
	}
}

func TestVerifyRejectsExpiredAndMalformedChallenges(t *testing.T) {
	secret := []byte("test-secret")
	challenge, err := Issue(secret, -time.Second)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	for _, tc := range []struct{ challenge, solution string }{
		{challenge, "0"},
		{"not-a-challenge", "0"},
		{challenge, "not-a-number"},
	} {
		if err := Verify(secret, tc.challenge, tc.solution, 8); !errorsIsInvalid(err) {
			t.Errorf("Verify(%q, %q) = %v, want ErrInvalid", tc.challenge, tc.solution, err)
		}
	}
}

func errorsIsInvalid(err error) bool { return err == ErrInvalid }

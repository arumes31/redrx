package security

import "testing"

func TestAccountSecretRoundTrip(t *testing.T) {
	key := []byte("application-secret")
	sealed, err := SealAccountSecret(key, "TOTPSECRET")
	if err != nil {
		t.Fatalf("SealAccountSecret: %v", err)
	}
	if sealed == "TOTPSECRET" {
		t.Fatal("secret was stored in plaintext")
	}
	opened, err := OpenAccountSecret(key, sealed)
	if err != nil {
		t.Fatalf("OpenAccountSecret: %v", err)
	}
	if opened != "TOTPSECRET" {
		t.Errorf("opened = %q, want TOTPSECRET", opened)
	}
	if _, err := OpenAccountSecret([]byte("wrong-key"), sealed); err == nil {
		t.Fatal("decrypting with the wrong key succeeded")
	}
}

func TestRecoveryCodesAreUniqueAndNormalize(t *testing.T) {
	codes, err := GenerateRecoveryCodes(10)
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes: %v", err)
	}
	seen := map[string]bool{}
	for _, code := range codes {
		if seen[code] {
			t.Fatalf("duplicate recovery code %q", code)
		}
		seen[code] = true
	}
	key := []byte("application-secret")
	want := RecoveryCodeHash(key, codes[0])
	withoutSeparators := normalizeRecoveryCode(codes[0])
	if got := RecoveryCodeHash(key, withoutSeparators); got != want {
		t.Errorf("normalized hash = %q, want %q", got, want)
	}
	if got := RecoveryCodeHash([]byte("other-key"), codes[0]); got == want {
		t.Error("different application keys produced the same recovery hash")
	}
}

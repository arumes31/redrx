package security

import "testing"

// Vectors produced by Werkzeug 3.1.8, the version the Python deployment ran.
// They guard the promise that existing accounts keep logging in.
var werkzeugVectors = []struct {
	name, password, hash string
}{
	{
		"scrypt default",
		"correct horse battery staple",
		"scrypt:32768:8:1$8ZJ3Qe1bK82POrMW$06a92da362912c563a7a75e89f5a4c7501c491e80e3e26c8b60d40b68b605b028a9c2ef2c5ca8dff824c1c996a03ef43275a215db1a878b8f2174000728cbf97",
	},
	{
		"scrypt tuned params",
		"correct horse battery staple",
		"scrypt:16384:8:1$vDGkNmwmVC7KU0dG$5c465a2333db33ad1e0b95acfd6b51ba4619f51e8a70b48f27fc50815485d473320db4749d14778de4cb82ae2696f75e3d4bd30954ac97c3c749350a3fcb6cb5",
	},
	{
		"pbkdf2 sha256 600k",
		"correct horse battery staple",
		"pbkdf2:sha256:600000$m461LnbRrfPy8wxQ$4ded5703dc2ab814df2960f729232b4454c9ca6310ea51c9ca4e1c0cc905c160",
	},
	{
		"pbkdf2 sha256 1m",
		"correct horse battery staple",
		"pbkdf2:sha256:1000000$90gil6NzgDu3rB72$b6eca1cd328bf97608a86c548f35900180885053fa02db42fde69a25f1e83069",
	},
	{
		"pbkdf2 sha512",
		"correct horse battery staple",
		"pbkdf2:sha512:1000$x4jsyxpCSfpkGpLn$a7f14f4650a86c01ae06fde444cdc4346695cd52fd15ed4fec79d42c49c228049f01a45af55f836dbd8e514582b734d7b4f3c8b0fab62e5e424078dfb2011eab",
	},
	{
		"scrypt other password",
		"p@ssw0rd!",
		"scrypt:32768:8:1$qay0LW0tGj18KQ7z$8f7000426058de8cc99a0ae0272a279bd8b3b9e7e0edb70bd8c517c0a3a08d1f7fdfafc0dc0c1c704816ed449635da6addf91e6a19eed2c2bc7c6324dd24eb09",
	},
}

func TestCheckPasswordHashAcceptsWerkzeugHashes(t *testing.T) {
	for _, v := range werkzeugVectors {
		t.Run(v.name, func(t *testing.T) {
			if !CheckPasswordHash(v.hash, v.password) {
				t.Errorf("correct password rejected for %s", v.hash)
			}
			if CheckPasswordHash(v.hash, v.password+"x") {
				t.Errorf("wrong password accepted for %s", v.hash)
			}
		})
	}
}

func TestGenerateProducesWerkzeugFormat(t *testing.T) {
	h, err := GeneratePasswordHash("hunter2")
	if err != nil {
		t.Fatalf("GeneratePasswordHash: %v", err)
	}
	method, salt, digest, err := splitHash(h)
	if err != nil {
		t.Fatalf("generated hash is not parseable: %v", err)
	}
	if method != "scrypt:32768:8:1" {
		t.Errorf("method = %q, want Werkzeug's current default scrypt:32768:8:1", method)
	}
	if len(salt) != saltLength {
		t.Errorf("salt length = %d, want %d", len(salt), saltLength)
	}
	if len(digest) != scryptKeyLen*2 {
		t.Errorf("digest length = %d hex chars, want %d", len(digest), scryptKeyLen*2)
	}
	if !CheckPasswordHash(h, "hunter2") {
		t.Error("generated hash does not verify against its own password")
	}
	if CheckPasswordHash(h, "hunter3") {
		t.Error("generated hash verifies against the wrong password")
	}
}

func TestCheckPasswordHashRejectsMalformed(t *testing.T) {
	for _, bad := range []string{
		"",
		"nodollars",
		"only$two",
		"scrypt:32768:8:1$salt$",        // empty digest
		"scrypt:bogus$salt$aabb",        // unparseable params
		"unknownmethod$salt$aabb",       // unsupported digest
		"pbkdf2:sha256:0$salt$aabbccdd", // zero iterations
	} {
		if CheckPasswordHash(bad, "anything") {
			t.Errorf("malformed hash %q was accepted", bad)
		}
	}
}

func TestNeedsRehash(t *testing.T) {
	if !NeedsRehash("pbkdf2:sha256:600000$m461LnbRrfPy8wxQ$4ded5703dc2ab814df2960f729232b4454c9ca6310ea51c9ca4e1c0cc905c160") {
		t.Error("pbkdf2 hash should be flagged for upgrade")
	}
	if NeedsRehash(werkzeugVectors[0].hash) {
		t.Error("current-default scrypt hash should not be flagged for upgrade")
	}
}

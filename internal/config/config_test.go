package config

import (
	"strings"
	"testing"
)

// clearEnv unsets every variable Load reads, so one test cannot leak into
// another through the process environment.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"SECRET_KEY", "FLASK_DEBUG", "REDRX_DEBUG", "DATABASE_URL", "BASE_DOMAIN",
		"BLOCKED_DOMAINS", "EXPIRY_HOURS", "SHORT_CODE_LENGTH", "ENABLE_PHISHING_CHECK",
		"DISABLE_REGISTRATION", "RATELIMIT_DEFAULT", "RATELIMIT_STORAGE_URL", "LISTEN_ADDR",
		"USE_CLOUDFLARE", "ENABLE_SEO", "PHISHING_LIST_URLS", "MAXMIND_LICENSE_KEY",
	} {
		t.Setenv(k, "")
		_ = k
	}
	// t.Setenv cannot unset, so clear by setting empty and rely on Load
	// treating empty SECRET_KEY as missing.
}

// TestSecretKeyRequiredInProduction guards the documented rule that the service
// must refuse to start without an explicit key outside debug mode.
func TestSecretKeyRequiredInProduction(t *testing.T) {
	clearEnv(t)
	t.Setenv("SECRET_KEY", "")
	t.Setenv("REDRX_DEBUG", "false")

	if _, err := Load(); err == nil {
		t.Fatal("Load succeeded without SECRET_KEY in production mode")
	} else if !strings.Contains(err.Error(), "SECRET_KEY") {
		t.Errorf("error = %v, want it to name SECRET_KEY", err)
	}
}

func TestSecretKeyFallsBackOnlyInDebug(t *testing.T) {
	clearEnv(t)
	t.Setenv("SECRET_KEY", "")
	t.Setenv("REDRX_DEBUG", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load in debug mode: %v", err)
	}
	if len(cfg.SecretKey) == 0 {
		t.Error("debug mode did not supply a fallback SECRET_KEY")
	}
	// A static key keeps sessions valid across restarts, which is the point.
	cfg2, err := Load()
	if err != nil {
		t.Fatalf("second Load: %v", err)
	}
	if string(cfg.SecretKey) != string(cfg2.SecretKey) {
		t.Error("the debug fallback key is not stable across restarts")
	}
}

// TestLegacyFlaskDebugStillWorks keeps existing deployments that set
// FLASK_DEBUG from silently losing debug mode.
func TestLegacyFlaskDebugStillWorks(t *testing.T) {
	clearEnv(t)
	t.Setenv("SECRET_KEY", "")
	t.Setenv("FLASK_DEBUG", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load with FLASK_DEBUG=true: %v", err)
	}
	if !cfg.Debug {
		t.Error("FLASK_DEBUG=true did not enable debug mode")
	}
}

func TestDefaultsMatchPreviousDeployment(t *testing.T) {
	clearEnv(t)
	// The BASE_DOMAIN default under test is the placeholder, which Load refuses
	// outside debug, so observe it in debug mode.
	t.Setenv("REDRX_DEBUG", "true")
	t.Setenv("SECRET_KEY", "a-key")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"BaseDomain", cfg.BaseDomain, "short.example.com"},
		{"ExpiryHours", cfg.ExpiryHours, 24},
		{"ShortCodeLength", cfg.ShortCodeLength, 6},
		{"DefaultQRColor", cfg.DefaultQRColor, "black"},
		{"DefaultQRBG", cfg.DefaultQRBG, "white"},
		{"EnablePhishingCheck", cfg.EnablePhishingCheck, true},
		{"EnableAutoRemovePhish", cfg.EnableAutoRemovePhish, false},
		{"DisableRegistration", cfg.DisableRegistration, false},
		{"DisableAnonymousCreate", cfg.DisableAnonymousCreate, false},
		{"UseCloudflare", cfg.UseCloudflare, false},
		{"EnableSEO", cfg.EnableSEO, false},
		{"SEODomain", cfg.SEODomain, "redrx.eu"},
		{"RateLimitDefault", cfg.RateLimitDefault, "200 per day;50 per hour"},
		{"RateLimitStorageURI", cfg.RateLimitStorageURI, "memory://"},
		{"PhishingCheckInterval", cfg.PhishingCheckInterval, 24},
		{"Listen", cfg.Listen, ":5000"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}

	if !strings.HasPrefix(cfg.DatabaseURL, "sqlite:///") {
		t.Errorf("DatabaseURL = %q, want the SQLite fallback", cfg.DatabaseURL)
	}
}

// TestEmptyEnvVarFallsBackToDefault covers the compose pattern
// `- BASE_DOMAIN=${BASE_DOMAIN}`, which sets the variable to the empty string
// when the operator has not defined it.
func TestEmptyEnvVarFallsBackToDefault(t *testing.T) {
	clearEnv(t)
	// The BASE_DOMAIN default under test is the placeholder, which Load refuses
	// outside debug, so observe it in debug mode.
	t.Setenv("REDRX_DEBUG", "true")
	t.Setenv("SECRET_KEY", "a-key")
	t.Setenv("BASE_DOMAIN", "")
	t.Setenv("RATELIMIT_DEFAULT", "   ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BaseDomain != "short.example.com" {
		t.Errorf("BaseDomain = %q, want the default when the variable is empty", cfg.BaseDomain)
	}
	if cfg.RateLimitDefault != "200 per day;50 per hour" {
		t.Errorf("RateLimitDefault = %q, want the default when the variable is blank", cfg.RateLimitDefault)
	}
}

func TestBlockedDomainsAreNormalised(t *testing.T) {
	clearEnv(t)
	// Load refuses to start on the placeholder BASE_DOMAIN outside debug.
	t.Setenv("BASE_DOMAIN", "links.example.org")
	t.Setenv("SECRET_KEY", "a-key")
	t.Setenv("BLOCKED_DOMAINS", " Evil.COM , spam.example ,, ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"evil.com", "spam.example"}
	if len(cfg.BlockedDomains) != len(want) {
		t.Fatalf("BlockedDomains = %v, want %v", cfg.BlockedDomains, want)
	}
	for i := range want {
		if cfg.BlockedDomains[i] != want[i] {
			t.Errorf("BlockedDomains[%d] = %q, want %q", i, cfg.BlockedDomains[i], want[i])
		}
	}
}

func TestCanonicalHostStripsScheme(t *testing.T) {
	cases := map[string]string{
		"short.example.com":          "short.example.com",
		"https://short.example.com":  "short.example.com",
		"http://short.example.com/":  "short.example.com",
		"localhost:5000":             "localhost:5000",
		"https://short.example.com/": "short.example.com",
	}
	for in, want := range cases {
		c := &Config{BaseDomain: in}
		if got := c.CanonicalHost(); got != want {
			t.Errorf("CanonicalHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShortURL(t *testing.T) {
	c := &Config{BaseDomain: "https://short.example.com"}
	if got := c.ShortURL("ABC123"); got != "https://short.example.com/ABC123" {
		t.Errorf("ShortURL = %q", got)
	}
}

// TestPlaceholderBaseDomainRejectedInProduction guards against shipping the
// example host. canonicalDomain 301s every request to BASE_DOMAIN, so booting
// with the placeholder sends every short link to a domain the operator does not
// control — and a 301 is cached, so it outlives the misconfiguration.
func TestPlaceholderBaseDomainRejectedInProduction(t *testing.T) {
	clearEnv(t)
	t.Setenv("SECRET_KEY", "a-real-key")
	t.Setenv("REDRX_DEBUG", "false")

	if _, err := Load(); err == nil {
		t.Fatal("Load accepted the placeholder BASE_DOMAIN in production mode")
	} else if !strings.Contains(err.Error(), "BASE_DOMAIN") {
		t.Errorf("error = %v, want it to name BASE_DOMAIN", err)
	}

	// A real host boots.
	t.Setenv("BASE_DOMAIN", "links.example.org")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load with a real BASE_DOMAIN: %v", err)
	}
	if cfg.BaseDomain != "links.example.org" {
		t.Errorf("BaseDomain = %q", cfg.BaseDomain)
	}
}

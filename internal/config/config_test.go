package config

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// clearEnv unsets every variable Load reads, so one test cannot leak into
// another through the process environment.
func clearEnv(t *testing.T) {
	t.Helper()
	// Every variable Load reads is listed, so a value in the tester's own
	// environment cannot leak into a defaults assertion.
	for _, k := range []string{
		"SECRET_KEY", "FLASK_DEBUG", "REDRX_DEBUG", "DATABASE_URL", "BASE_DOMAIN",
		"BLOCKED_DOMAINS", "BLOCKED_DOMAINS_PATH", "EXPIRY_HOURS", "SHORT_CODE_LENGTH",
		"DEFAULT_QR_COLOR", "DEFAULT_QR_BACKGROUND", "GEOIP_DB_PATH",
		"PHISHING_LIST_URLS", "PHISHING_CHECK_INTERVAL", "PHISHING_REMOVE_INTERVAL",
		"ENABLE_PHISHING_CHECK", "ENABLE_AUTO_REMOVE_PHISHING",
		"DISABLE_ANONYMOUS_CREATE", "DISABLE_REGISTRATION", "USE_CLOUDFLARE",
		"ANONYMIZE_LOGS", "ENABLE_SEO", "SEO_DOMAIN", "TRUSTED_PROXIES",
		"ANONYMOUS_POW_DIFFICULTY", "ENABLE_CONSENT_BANNER", "HONOR_DO_NOT_TRACK",
		"RATELIMIT_DEFAULT", "RATELIMIT_STORAGE_URL", "RATELIMIT_LOGIN",
		"RATELIMIT_REGISTER", "RATELIMIT_AUTH", "RATELIMIT_API", "RATELIMIT_CREATE",
		"RATELIMIT_REDIRECT",
		"LISTEN_ADDR", "MAXMIND_LICENSE_KEY",
	} {
		t.Setenv(k, "")
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
		{"EnableConsentBanner", cfg.EnableConsentBanner, false},
		{"HonorDoNotTrack", cfg.HonorDoNotTrack, true},
		{"AnonymousPoWDifficulty", cfg.AnonymousPoWDifficulty, 16},
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
	baseDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if want := filepath.Join(baseDir, "data", "blocked_domains.txt"); cfg.BlockedDomainsPath != want {
		t.Errorf("BlockedDomainsPath = %q, want %q", cfg.BlockedDomainsPath, want)
	}
}

func TestBlockedDomainsPathOverrideIsPreserved(t *testing.T) {
	clearEnv(t)
	t.Setenv("REDRX_DEBUG", "true")
	t.Setenv("SECRET_KEY", "a-key")
	t.Setenv("BLOCKED_DOMAINS_PATH", "/srv/redrx/blocklist.txt")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BlockedDomainsPath != "/srv/redrx/blocklist.txt" {
		t.Errorf("BlockedDomainsPath = %q, want the explicit override", cfg.BlockedDomainsPath)
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

// TestTrustedProxyChain covers the deployed topology: Cloudflare -> nginx -> app.
// The peer is nginx, and X-Forwarded-For arrives as "<client>, <cloudflare edge>",
// so the rightmost entry is an edge address rather than the visitor.
func TestTrustedProxyChain(t *testing.T) {
	clearEnv(t)
	t.Setenv("SECRET_KEY", "k")
	t.Setenv("BASE_DOMAIN", "links.example.org")
	t.Setenv("TRUSTED_PROXIES", "172.18.0.0/16,127.0.0.1")
	t.Setenv("USE_CLOUDFLARE", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !cfg.IsTrustedProxy("172.18.0.5:41234") {
		t.Error("nginx on the compose network should be trusted")
	}
	if cfg.IsTrustedProxy("203.0.113.9:1234") {
		t.Error("an arbitrary internet peer must not be trusted")
	}

	// With Cloudflare in front, CF-Connecting-IP names the real visitor.
	if got := cfg.ClientIPFromForwarded("198.51.100.7, 172.71.1.1", "198.51.100.7"); got != "198.51.100.7" {
		t.Errorf("client = %q, want the CF-Connecting-IP value", got)
	}

	// Without the Cloudflare header, and with the edge range not configured,
	// the last hop is indistinguishable from a client — so it is returned. This
	// is the documented reason a Cloudflare deployment must either set
	// USE_CLOUDFLARE=true or list Cloudflare's ranges in TRUSTED_PROXIES;
	// otherwise every visitor buckets under a few edge addresses.
	cfg.UseCloudflare = false
	if got := cfg.ClientIPFromForwarded("198.51.100.7, 172.71.1.1", ""); got != "172.71.1.1" {
		t.Errorf("client = %q; with no CF header and no edge range configured "+
			"the last hop is all we can identify", got)
	}

	// Listing the edge range as trusted makes the walk skip it.
	cfg.TrustedProxies = append(cfg.TrustedProxies, mustCIDR(t, "172.71.0.0/16"))
	if got := cfg.ClientIPFromForwarded("198.51.100.7, 172.71.1.1", ""); got != "198.51.100.7" {
		t.Errorf("client = %q, want 198.51.100.7 after skipping the trusted hop", got)
	}
}

// TestNonPositiveIntsFallBackToDefault covers a zero or negative EXPIRY_HOURS /
// SHORT_CODE_LENGTH, which must not reach link generation.
func TestNonPositiveIntsFallBackToDefault(t *testing.T) {
	clearEnv(t)
	t.Setenv("SECRET_KEY", "k")
	t.Setenv("BASE_DOMAIN", "links.example.org")
	t.Setenv("EXPIRY_HOURS", "0")
	t.Setenv("SHORT_CODE_LENGTH", "-4")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ExpiryHours != 24 {
		t.Errorf("ExpiryHours = %d, want the default 24 for a non-positive value", cfg.ExpiryHours)
	}
	if cfg.ShortCodeLength != 6 {
		t.Errorf("ShortCodeLength = %d, want the default 6 for a non-positive value", cfg.ShortCodeLength)
	}

	// A valid positive value is still honoured.
	t.Setenv("EXPIRY_HOURS", "48")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ExpiryHours != 48 {
		t.Errorf("ExpiryHours = %d, want 48", cfg.ExpiryHours)
	}
}

// TestForwardedChainCapped rejects an implausibly long X-Forwarded-For by
// falling back to the peer, so a client cannot pad the header without bound.
func TestForwardedChainCapped(t *testing.T) {
	c := &Config{}
	parts := make([]string, 0, maxForwardedHops+5)
	for i := 0; i < maxForwardedHops+5; i++ {
		parts = append(parts, "203.0.113.1")
	}
	if got := c.ClientIPFromForwarded(strings.Join(parts, ", "), ""); got != "" {
		t.Errorf("client = %q, want the peer fallback for an over-long chain", got)
	}

	// A chain within the cap still resolves.
	if got := c.ClientIPFromForwarded("198.51.100.7, 172.71.1.1", ""); got != "172.71.1.1" {
		t.Errorf("client = %q, want the last hop for a normal chain", got)
	}
}

func TestUntrustedPeerIsNeverTrusted(t *testing.T) {
	clearEnv(t)
	t.Setenv("SECRET_KEY", "k")
	t.Setenv("BASE_DOMAIN", "links.example.org")
	// No TRUSTED_PROXIES at all: the default must be to trust nothing, or the
	// rate-limit key becomes client-controlled.
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.IsTrustedProxy("10.0.0.1:1234") || cfg.IsTrustedProxy("127.0.0.1:1234") {
		t.Error("no proxy should be trusted when TRUSTED_PROXIES is unset")
	}
}

func TestTrustedProxiesRejectsGarbage(t *testing.T) {
	clearEnv(t)
	t.Setenv("SECRET_KEY", "k")
	t.Setenv("BASE_DOMAIN", "links.example.org")
	t.Setenv("TRUSTED_PROXIES", "not-an-ip")
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted an invalid TRUSTED_PROXIES entry")
	}
}

func mustCIDR(t *testing.T, s string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatalf("ParseCIDR(%q): %v", s, err)
	}
	return n
}

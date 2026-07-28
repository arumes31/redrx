// Package config loads runtime configuration from the environment.
//
// Every variable name and default matches the previous Python deployment so
// existing .env files and compose stacks keep working unchanged.
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Debug     bool
	SecretKey []byte

	DatabaseURL   string
	MaxUploadSize int64

	BaseDomain       string
	BlockedDomains   []string
	ExpiryHours      int
	ShortCodeLength  int
	DefaultQRColor   string
	DefaultQRBG      string
	GeoIPDBPath      string
	PhishingListURLs []string

	BlockedDomainsPath     string
	PhishingCheckInterval  int
	EnablePhishingCheck    bool
	EnableAutoRemovePhish  bool
	PhishingRemoveInterval int

	// TrustedProxies lists the peer addresses and CIDR blocks whose
	// X-Forwarded-* and CF-* headers are believed. Empty means trust none.
	TrustedProxies []*net.IPNet

	DisableAnonymousCreate bool
	DisableRegistration    bool
	UseCloudflare          bool
	AnonymizeLogs          bool
	EnableSEO              bool
	SEODomain              string

	RateLimitDefault    string
	RateLimitStorageURI string
	RateLimitLogin      string
	RateLimitRegister   string
	RateLimitAuth       string
	RateLimitAPI        string
	RateLimitCreate     string
	RateLimitRedirect   string
	RateLimitHealth     string
	RateLimitMetrics    string

	Listen string
}

// env reads a variable, treating an empty value as absent. Compose files
// interpolate an unset variable to the empty string (`- BASE_DOMAIN=${BASE_DOMAIN}`),
// and taking that literally would blank out settings the operator never
// intended to override.
func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "t", "yes", "y":
		return true
	case "false", "0", "f", "no", "n":
		return false
	}
	return def
}

func envInt(key string, def int) int {
	v, ok := os.LookupEnv(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return n
}

// envPositiveInt is envInt for settings that must be positive, so a zero or
// negative value in the environment falls back to the default rather than
// reaching link generation where a length or expiry of 0 makes no sense.
func envPositiveInt(key string, def int) int {
	if n := envInt(key, def); n > 0 {
		return n
	}
	return def
}

// envList splits a comma-separated variable, trimming and dropping empties.
func envList(key, def string) []string {
	raw := env(key, def)
	var out []string
	for _, p := range strings.Split(raw, ",") {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// placeholderBaseDomain is the example value shipped in the Dockerfile; it is
// never a host anyone actually serves from.
const placeholderBaseDomain = "short.example.com"

// maxForwardedHops caps how many entries of X-Forwarded-For are parsed, so a
// client cannot pad the header without bound.
const maxForwardedHops = 20

// Load reads configuration from the environment and validates it.
func Load() (*Config, error) {
	baseDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("determine working directory: %w", err)
	}

	c := &Config{
		Debug:         envBool("FLASK_DEBUG", false) || envBool("REDRX_DEBUG", false),
		MaxUploadSize: 1 * 1024 * 1024,

		BaseDomain:      env("BASE_DOMAIN", placeholderBaseDomain),
		ExpiryHours:     envPositiveInt("EXPIRY_HOURS", 24),
		ShortCodeLength: envPositiveInt("SHORT_CODE_LENGTH", 6),
		DefaultQRColor:  env("DEFAULT_QR_COLOR", "black"),
		DefaultQRBG:     env("DEFAULT_QR_BACKGROUND", "white"),
		GeoIPDBPath:     env("GEOIP_DB_PATH", filepath.Join(baseDir, "GeoLite2-Country.mmdb")),

		PhishingListURLs: envList("PHISHING_LIST_URLS",
			"https://raw.githubusercontent.com/mitchellkrogza/Phishing.Database/master/phishing-domains-ACTIVE.txt"),
		BlockedDomainsPath:     env("BLOCKED_DOMAINS_PATH", filepath.Join(baseDir, "blocked_domains.txt")),
		PhishingCheckInterval:  envInt("PHISHING_CHECK_INTERVAL", 24),
		EnablePhishingCheck:    envBool("ENABLE_PHISHING_CHECK", true),
		EnableAutoRemovePhish:  envBool("ENABLE_AUTO_REMOVE_PHISHING", false),
		PhishingRemoveInterval: envInt("PHISHING_REMOVE_INTERVAL", 24),

		DisableAnonymousCreate: envBool("DISABLE_ANONYMOUS_CREATE", false),
		DisableRegistration:    envBool("DISABLE_REGISTRATION", false),
		UseCloudflare:          envBool("USE_CLOUDFLARE", false),
		AnonymizeLogs:          envBool("ANONYMIZE_LOGS", false),
		EnableSEO:              envBool("ENABLE_SEO", false),
		SEODomain:              env("SEO_DOMAIN", "redrx.eu"),

		RateLimitDefault:    env("RATELIMIT_DEFAULT", "200 per day;50 per hour"),
		RateLimitStorageURI: env("RATELIMIT_STORAGE_URL", "memory://"),
		RateLimitLogin:      env("RATELIMIT_LOGIN", "10 per minute"),
		RateLimitRegister:   env("RATELIMIT_REGISTER", "5 per hour"),
		RateLimitAuth:       env("RATELIMIT_AUTH", "10 per minute"),
		RateLimitAPI:        env("RATELIMIT_API", "60 per minute"),
		RateLimitCreate:     env("RATELIMIT_CREATE", "10 per minute"),
		RateLimitRedirect:   env("RATELIMIT_REDIRECT", "100 per minute"),
		RateLimitHealth:     env("RATELIMIT_HEALTH", "10 per minute"),
		RateLimitMetrics:    env("RATELIMIT_METRICS", "10 per minute"),

		Listen: env("LISTEN_ADDR", ":5000"),
	}

	for _, b := range envList("BLOCKED_DOMAINS", "") {
		c.BlockedDomains = append(c.BlockedDomains, strings.ToLower(b))
	}

	// TRUSTED_PROXIES accepts addresses and CIDR blocks, e.g.
	// "10.0.0.0/8,172.18.0.1". "*" trusts any peer, which is only correct when
	// nothing but a proxy can reach the listener.
	proxies, err := parseTrustedProxies(envList("TRUSTED_PROXIES", ""))
	if err != nil {
		return nil, err
	}
	c.TrustedProxies = proxies

	c.DatabaseURL = env("DATABASE_URL", "")
	if c.DatabaseURL == "" {
		c.DatabaseURL = "sqlite:///" + filepath.Join(baseDir, "db", "shortener.db")
	}

	secret := env("SECRET_KEY", "")
	if secret == "" {
		if !c.Debug {
			return nil, errors.New("SECRET_KEY must be set in production environments for security")
		}
		// A fixed value, deliberately: generating a random one per boot would
		// silently invalidate every session on restart and make the insecure
		// default indistinguishable from a real key. Production is guarded by
		// the error above.
		secret = "dev-secret-key-do-not-use-in-production" // #nosec G101 -- documented insecure dev default
	}
	c.SecretKey = []byte(secret)

	// The Dockerfile bakes in the placeholder and the ghcr compose file passes
	// ${BASE_DOMAIN}, which interpolates to empty when unset. Left unnoticed,
	// canonicalDomain 301s every short link to a domain the operator does not
	// own — and browsers and CDNs cache a 301, so it outlives the fix.
	if !c.Debug && c.BaseDomain == placeholderBaseDomain {
		return nil, errors.New("BASE_DOMAIN is still " + placeholderBaseDomain +
			"; set it to the host this instance serves, or every link will redirect off-site")
	}

	return c, nil
}

// parseTrustedProxies turns the configured entries into networks. A bare
// address becomes a single-host network so both forms compare the same way.
func parseTrustedProxies(entries []string) ([]*net.IPNet, error) {
	var out []*net.IPNet
	for _, raw := range entries {
		e := strings.TrimSpace(raw)
		switch {
		case e == "":
			continue
		case e == "*":
			// Trust every peer. Correct only when the listener is unreachable
			// except through a proxy.
			_, all4, _ := net.ParseCIDR("0.0.0.0/0")
			_, all6, _ := net.ParseCIDR("::/0")
			out = append(out, all4, all6)
		case strings.Contains(e, "/"):
			_, network, err := net.ParseCIDR(e)
			if err != nil {
				return nil, fmt.Errorf("TRUSTED_PROXIES: %q is not a valid CIDR block: %w", e, err)
			}
			out = append(out, network)
		default:
			ip := net.ParseIP(e)
			if ip == nil {
				return nil, fmt.Errorf("TRUSTED_PROXIES: %q is not a valid IP address or CIDR block", e)
			}
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			out = append(out, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
		}
	}
	return out, nil
}

// IsTrustedProxy reports whether headers from this peer may be believed.
func (c *Config) IsTrustedProxy(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	return c.isTrustedIP(strings.TrimSpace(host))
}

func (c *Config) isTrustedIP(host string) bool {
	if len(c.TrustedProxies) == 0 {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, n := range c.TrustedProxies {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// ClientIPFromForwarded resolves the originating client address from a chain of
// proxies. The caller must already have established that the immediate peer is
// trusted.
//
// The typical deployment is Cloudflare -> nginx -> app, which is two hops. In
// that shape X-Forwarded-For arrives as "<client>, <cloudflare edge>", so the
// rightmost entry is an edge address, not the visitor: taking it would collapse
// the whole internet into the handful of Cloudflare IPs serving this zone and
// make every per-IP rate limit effectively global.
//
// Cloudflare's CF-Connecting-IP always names the true client, so it wins when
// USE_CLOUDFLARE is on. Otherwise the chain is walked right to left, skipping
// addresses that are themselves configured proxies, and the first address that
// is not one is the client.
func (c *Config) ClientIPFromForwarded(xff, cfConnectingIP string) string {
	if c.UseCloudflare {
		if ip := strings.TrimSpace(cfConnectingIP); net.ParseIP(ip) != nil {
			return ip
		}
	}

	// A real chain is a couple of hops. A long one is either broken or a client
	// padding the header to bury its own address past the trusted proxies, or to
	// make the split allocate. Count commas first — that does not allocate — and
	// fall back to the peer when the chain is implausibly long.
	if strings.Count(xff, ",")+1 > maxForwardedHops {
		return ""
	}

	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		ip := strings.TrimSpace(parts[i])
		if net.ParseIP(ip) == nil {
			// A malformed hop means the chain cannot be reasoned about; fall
			// back to the peer rather than trusting a guess.
			return ""
		}
		if c.isTrustedIP(ip) {
			continue
		}
		return ip
	}
	return ""
}

// CanonicalHost strips any scheme from BaseDomain, leaving just host[:port].
func (c *Config) CanonicalHost() string {
	d := c.BaseDomain
	if i := strings.Index(d, "://"); i >= 0 {
		d = d[i+3:]
	}
	return strings.TrimSuffix(d, "/")
}

// ShortURL builds the public https URL for a short code.
func (c *Config) ShortURL(code string) string {
	return "https://" + c.CanonicalHost() + "/" + code
}

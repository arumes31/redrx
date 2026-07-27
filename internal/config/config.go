// Package config loads runtime configuration from the environment.
//
// Every variable name and default matches the previous Python deployment so
// existing .env files and compose stacks keep working unchanged.
package config

import (
	"errors"
	"fmt"
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
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return v
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

// Load reads configuration from the environment and validates it.
func Load() (*Config, error) {
	baseDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("determine working directory: %w", err)
	}

	c := &Config{
		Debug:         envBool("FLASK_DEBUG", false) || envBool("REDRX_DEBUG", false),
		MaxUploadSize: 1 * 1024 * 1024,

		BaseDomain:      env("BASE_DOMAIN", "short.example.com"),
		ExpiryHours:     envInt("EXPIRY_HOURS", 24),
		ShortCodeLength: envInt("SHORT_CODE_LENGTH", 6),
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

	c.DatabaseURL = env("DATABASE_URL", "")
	if c.DatabaseURL == "" {
		c.DatabaseURL = "sqlite:///" + filepath.Join(baseDir, "db", "shortener.db")
	}

	secret := env("SECRET_KEY", "")
	if secret == "" {
		if !c.Debug {
			return nil, errors.New("SECRET_KEY must be set in production environments for security")
		}
		secret = "dev-secret-key-do-not-use-in-production"
	}
	c.SecretKey = []byte(secret)

	return c, nil
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

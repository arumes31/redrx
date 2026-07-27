// Package geo resolves a client IP to a country name.
//
// Lookups are tried in the same order the previous implementation used: Redis
// cache, Cloudflare's CF-IPCountry header, private-network shortcut, then the
// local MaxMind database.
package geo

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/geoip2-golang"
	"github.com/redis/go-redis/v9"
)

// trustedProxyKey marks a request whose forwarding headers arrived from a
// proxy listed in TRUSTED_PROXIES.
type trustedProxyKey struct{}

// WithTrustedProxy marks ctx as having come through a trusted proxy.
func WithTrustedProxy(ctx context.Context) context.Context {
	return context.WithValue(ctx, trustedProxyKey{}, true)
}

// IsFromTrustedProxy reports whether WithTrustedProxy marked this context.
func IsFromTrustedProxy(ctx context.Context) bool {
	v, _ := ctx.Value(trustedProxyKey{}).(bool)
	return v
}

const (
	cachePrefix = "geo:"
	cacheTTL    = 5 * time.Minute
)

// Resolver looks up countries and caches the answers.
type Resolver struct {
	dbPath        string
	useCloudflare bool
	cache         *redis.Client
	log           *slog.Logger

	// mu guards every field below. Lookups take it for reading and hold it for
	// the duration of the query, so Close cannot unmap the database underneath
	// an in-flight read.
	mu     sync.RWMutex
	reader *geoip2.Reader
	opened string
	// modTime and size fingerprint the open file, so a database replaced on
	// disk by the updater sidecar is picked up instead of serving a stale mmap
	// of the unlinked inode forever.
	modTime time.Time
	size    int64
	// checkedAt bounds how often that fingerprint is re-stat'ed.
	checkedAt time.Time
	closed    bool
}

// readerRecheckInterval is how long an open database is trusted before its
// mtime is checked again. Without it every lookup would need a stat syscall.
const readerRecheckInterval = time.Minute

type Options struct {
	DatabasePath  string
	UseCloudflare bool
	Cache         *redis.Client
	Logger        *slog.Logger
}

func New(opts Options) *Resolver {
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Resolver{
		dbPath:        opts.DatabasePath,
		useCloudflare: opts.UseCloudflare,
		cache:         opts.Cache,
		log:           log,
	}
}

// ClientIP returns the caller's address.
//
// The proxy middleware has already resolved the real visitor out of the
// forwarding chain and written it to RemoteAddr, and it only does so for a
// trusted peer -- so reading RemoteAddr here is both correct and the single
// place that decision is made.
func (r *Resolver) ClientIP(req *http.Request) string {
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return req.RemoteAddr
	}
	return host
}

// Country resolves the country for ip, consulting the request headers first
// when Cloudflare integration is enabled.
func (r *Resolver) Country(ctx context.Context, ip string, req *http.Request) string {
	if ip == "" {
		return "Unknown"
	}

	if v := r.cached(ctx, ip); v != "" {
		return v
	}

	if r.useCloudflare && req != nil && IsFromTrustedProxy(req.Context()) {
		// Validate the shape before it becomes a stored analytics value: the
		// column is VARCHAR(100), so an over-long header fails the insert on
		// Postgres, and any value poisons the geo cache for this IP.
		if cc := strings.TrimSpace(req.Header.Get("CF-IPCountry")); isCountryCode(cc) {
			cc = strings.ToUpper(cc)
			r.store(ctx, ip, cc)
			return cc
		}
	}

	if isLocal(ip) {
		return "Local Network"
	}

	country := r.lookupDB(ip)
	if country != "" && !strings.Contains(country, "Unknown") {
		r.store(ctx, ip, country)
	}
	return country
}

func (r *Resolver) cached(ctx context.Context, ip string) string {
	if r.cache == nil {
		return ""
	}
	v, err := r.cache.Get(ctx, cachePrefix+ip).Result()
	if err != nil {
		return ""
	}
	return v
}

func (r *Resolver) store(ctx context.Context, ip, country string) {
	if r.cache == nil || country == "" {
		return
	}
	if err := r.cache.Set(ctx, cachePrefix+ip, country, cacheTTL).Err(); err != nil {
		r.log.Debug("geo cache write failed", "error", err)
	}
}

// isCountryCode reports whether s looks like an ISO 3166-1 alpha-2 code, the
// only thing CF-IPCountry is documented to carry (plus "XX"/"T1").
func isCountryCode(s string) bool {
	if len(s) != 2 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i] | 0x20 // lowercase
		if c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}

func isLocal(ip string) bool {
	addr := net.ParseIP(ip)
	if addr == nil {
		return false
	}
	return addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsUnspecified()
}

// lookupDB queries the MaxMind database, opening it on first use and keeping
// the handle for subsequent lookups.
//
// The read lock is held across the query itself, not just while fetching the
// handle: geoip2 reads straight out of a memory mapping that Close unmaps, so a
// Close racing an in-flight lookup would otherwise read freed pages.
func (r *Resolver) lookupDB(ip string) string {
	addr := net.ParseIP(ip)
	if addr == nil {
		return "Unknown"
	}

	// Fast path: a reader that is open and was verified recently answers under
	// the read lock alone. Going through ensureReader every time would funnel
	// every concurrent redirect through the exclusive lock — and, with no
	// database present, add a stat syscall per lookup while holding it.
	if country, ok := r.lookupWithOpenReader(addr); ok {
		return country
	}

	if err := r.ensureReader(); err != nil {
		return "Unknown (DB Missing)"
	}

	if country, ok := r.lookupWithOpenReader(addr); ok {
		return country
	}
	return "Unknown (DB Missing)"
}

// lookupWithOpenReader answers from the currently open database, reporting
// false when there is none or its fingerprint is due to be rechecked. The read
// lock is held across the query itself: geoip2 reads out of a memory mapping
// that Close unmaps, so releasing it first would risk reading freed pages.
func (r *Resolver) lookupWithOpenReader(addr net.IP) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.reader == nil || r.closed || time.Since(r.checkedAt) > readerRecheckInterval {
		return "", false
	}

	record, err := r.reader.Country(addr)
	if err != nil || record == nil {
		return "Unknown", true
	}
	if name := record.Country.Names["en"]; name != "" {
		return name, true
	}
	if record.Country.IsoCode != "" {
		return record.Country.IsoCode, true
	}
	return "Unknown", true
}

// ensureReader opens the database, or reopens it when the file on disk has been
// replaced. It takes the write lock, so it must never be called while the read
// lock is held.
func (r *Resolver) ensureReader() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return os.ErrClosed
	}
	if r.dbPath == "" {
		return os.ErrNotExist
	}

	info, err := os.Stat(r.dbPath)
	if err != nil {
		return err
	}

	// Open and unchanged: just extend the recheck deadline.
	if r.reader != nil && r.opened == r.dbPath &&
		info.ModTime().Equal(r.modTime) && info.Size() == r.size {
		r.checkedAt = time.Now()
		return nil
	}

	reader, err := geoip2.Open(r.dbPath)
	if err != nil {
		r.log.Warn("cannot open GeoIP database", "path", r.dbPath, "error", err)
		return err
	}
	if err := r.closeReaderLocked(); err != nil {
		r.log.Warn("closing the previous GeoIP database failed", "error", err)
	}
	r.reader, r.opened = reader, r.dbPath
	r.modTime, r.size, r.checkedAt = info.ModTime(), info.Size(), time.Now()
	return nil
}

// closeReaderLocked releases the current reader. The caller must hold the write
// lock, so no lookup can be reading through it.
func (r *Resolver) closeReaderLocked() error {
	if r.reader == nil {
		return nil
	}
	err := r.reader.Close()
	r.reader, r.opened = nil, ""
	r.modTime, r.size, r.checkedAt = time.Time{}, 0, time.Time{}
	return err
}

// Close releases the MaxMind reader, waiting for any in-flight lookup to finish
// first. It is terminal: a lookup arriving afterwards must not reopen the
// database, or a request still running past the shutdown deadline would leak a
// fresh mapping that nothing closes.
func (r *Resolver) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	return r.closeReaderLocked()
}

// AnonymizeIP masks an address down to the precision the privacy policy
// promises: two octets for IPv4, two groups for IPv6.
func AnonymizeIP(ip string) string {
	if ip == "" {
		return "Unknown"
	}
	if strings.Contains(ip, ".") {
		parts := strings.Split(ip, ".")
		if len(parts) == 4 {
			return parts[0] + "." + parts[1] + ".xxx.xxx"
		}
	} else if strings.Contains(ip, ":") {
		parts := strings.Split(ip, ":")
		if len(parts) >= 2 {
			return parts[0] + ":" + parts[1] + ":xxxx:xxxx"
		}
	}
	return "xxxx"
}

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

	// mu guards reader and opened. Lookups take it for reading and hold it for
	// the duration of the query, so Close cannot unmap the database underneath
	// an in-flight read.
	mu     sync.RWMutex
	reader *geoip2.Reader
	opened string
}

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

// ClientIP returns the caller's address, preferring Cloudflare's header when
// USE_CLOUDFLARE is set. RemoteAddr is used otherwise; the proxy middleware has
// already applied X-Forwarded-For by this point.
func (r *Resolver) ClientIP(req *http.Request) string {
	if r.useCloudflare {
		if ip := strings.TrimSpace(req.Header.Get("CF-Connecting-IP")); ip != "" {
			return ip
		}
	}
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

	if r.useCloudflare && req != nil {
		if cc := strings.TrimSpace(req.Header.Get("CF-IPCountry")); cc != "" {
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

	if err := r.ensureReader(); err != nil {
		return "Unknown (DB Missing)"
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Close may have run between opening the database and acquiring this lock.
	if r.reader == nil {
		return "Unknown (DB Missing)"
	}

	record, err := r.reader.Country(addr)
	if err != nil || record == nil {
		return "Unknown"
	}
	if name := record.Country.Names["en"]; name != "" {
		return name
	}
	if record.Country.IsoCode != "" {
		return record.Country.IsoCode
	}
	return "Unknown"
}

// ensureReader opens the database when it is not open yet, or when the
// configured path has changed. It takes the write lock, so it must never be
// called while the read lock is held.
func (r *Resolver) ensureReader() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.reader != nil && r.opened == r.dbPath {
		return nil
	}
	if r.dbPath == "" {
		return os.ErrNotExist
	}
	if _, err := os.Stat(r.dbPath); err != nil {
		return err
	}

	reader, err := geoip2.Open(r.dbPath)
	if err != nil {
		r.log.Warn("cannot open GeoIP database", "path", r.dbPath, "error", err)
		return err
	}
	if r.reader != nil {
		r.reader.Close()
	}
	r.reader, r.opened = reader, r.dbPath
	return nil
}

// Close releases the MaxMind reader, waiting for any in-flight lookup to
// finish first.
func (r *Resolver) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.reader != nil {
		err := r.reader.Close()
		r.reader, r.opened = nil, ""
		return err
	}
	return nil
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

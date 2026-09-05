// Package safety decides whether a destination URL may be shortened or served.
//
// Two independent sources are consulted: the operator's BLOCKED_DOMAINS list
// and a periodically downloaded phishing-domain feed. As in the previous
// implementation, an unreadable feed fails closed — if the list is configured
// but cannot be loaded and nothing is cached, every URL is rejected rather than
// silently allowing traffic through an unenforced filter.
package safety

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ErrListUnavailable means the blocklist is configured but unreadable, so no
// URL can be judged safe.
var ErrListUnavailable = errors.New("safety: blocked domains list unavailable")

// Checker holds the blocklist configuration and the parsed feed.
type Checker struct {
	enabled     bool
	listPath    string
	feedURLs    []string
	refresheAge time.Duration
	manual      []string
	log         *slog.Logger

	mu       sync.RWMutex
	domains  map[string]struct{}
	loadedAt time.Time
	modTime  time.Time
	size     int64

	// reloadMu serialises reloads. Without it, every request arriving while the
	// list is stale starts its own full parse of a multi-megabyte feed and
	// builds its own map — hundreds of concurrent copies right after a refresh
	// or at a cold start, which is enough to OOM a memory-capped container.
	reloadMu sync.Mutex
}

type Options struct {
	Enabled         bool
	BlockedListPath string
	FeedURLs        []string
	RefreshInterval time.Duration
	ManualDomains   []string
	Logger          *slog.Logger
}

func New(opts Options) *Checker {
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	manual := make([]string, 0, len(opts.ManualDomains))
	for _, d := range opts.ManualDomains {
		if d = strings.ToLower(strings.TrimSpace(d)); d != "" {
			manual = append(manual, d)
		}
	}
	return &Checker{
		enabled:     opts.Enabled,
		listPath:    opts.BlockedListPath,
		feedURLs:    opts.FeedURLs,
		refresheAge: opts.RefreshInterval,
		manual:      manual,
		log:         log,
	}
}

// IsSafeURL reports whether target may be used as a redirect destination.
func (c *Checker) IsSafeURL(target string) bool {
	ok, err := c.CheckURL(target)
	if err != nil {
		return false
	}
	return ok
}

// CheckURL is IsSafeURL with the reason separated from the verdict: err is
// non-nil only when the blocklist itself could not be consulted.
func (c *Checker) CheckURL(target string) (bool, error) {
	host, ok := parseHost(target)
	if !ok {
		return false, nil
	}

	for _, b := range c.manual {
		if host == b || strings.HasSuffix(host, "."+b) {
			return false, nil
		}
	}

	if !c.enabled {
		return true, nil
	}

	domains, err := c.blockedDomains()
	if err != nil {
		return false, err
	}
	return !matchesDomainOrParent(host, domains), nil
}

// IsAbsoluteHTTPURL reports whether target is an absolute http(s) URL with a
// host. CheckURL answers false for such a URL and for a blocked one alike, so
// callers that act destructively on "unsafe" need this to tell them apart.
func IsAbsoluteHTTPURL(target string) bool {
	_, ok := parseHost(target)
	return ok
}

// parseHost extracts the lowercase hostname, rejecting anything that is not an
// absolute http(s) URL.
func parseHost(target string) (string, bool) {
	if target == "" {
		return "", false
	}
	u, err := url.Parse(strings.TrimSpace(target))
	if err != nil {
		return "", false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return "", false
	}
	// A trailing dot is the DNS absolute-root marker: "evil.com." is the same
	// host as "evil.com" for resolution, but is a different string from the
	// blocklist entry, so it must be normalised before matching.
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "" {
		return "", false
	}
	return host, true
}

// matchesDomainOrParent walks a host up its parent domains, so a blocked
// "evil.com" also blocks "login.evil.com".
func matchesDomainOrParent(host string, domains map[string]struct{}) bool {
	if len(domains) == 0 {
		return false
	}
	for {
		if _, ok := domains[host]; ok {
			return true
		}
		_, rest, found := strings.Cut(host, ".")
		if !found {
			return false
		}
		host = rest
	}
}

// blockedDomains returns the parsed feed, reloading it when the file changed.
func (c *Checker) blockedDomains() (map[string]struct{}, error) {
	if c.listPath == "" {
		c.mu.RLock()
		defer c.mu.RUnlock()
		return c.domains, nil
	}

	info, statErr := os.Stat(c.listPath)
	if statErr == nil {
		if cached, fresh := c.cachedIfFresh(info); fresh {
			return cached, nil
		}
		return c.reload()
	}

	// Cannot stat the file: serve a previously loaded copy, else fail closed.
	c.mu.RLock()
	cached := c.domains
	c.mu.RUnlock()
	if cached != nil {
		return cached, nil
	}
	return nil, fmt.Errorf("%w: %w", ErrListUnavailable, statErr)
}

// cachedIfFresh returns the parsed list when it already matches the file on
// disk described by info.
func (c *Checker) cachedIfFresh(info os.FileInfo) (map[string]struct{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	fresh := c.domains != nil && info.ModTime().Equal(c.modTime) && info.Size() == c.size
	return c.domains, fresh
}

func (c *Checker) cached() map[string]struct{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.domains
}

// reload parses the list from disk. Only one caller does the work; the rest
// wait and take the result.
func (c *Checker) reload() (map[string]struct{}, error) {
	c.reloadMu.Lock()
	defer c.reloadMu.Unlock()

	// Another caller may have reloaded while this one waited for the lock.
	if info, err := os.Stat(c.listPath); err == nil {
		if cached, fresh := c.cachedIfFresh(info); fresh {
			return cached, nil
		}
	}

	f, err := os.Open(c.listPath)
	if err != nil {
		if cached := c.cached(); cached != nil {
			return cached, nil
		}
		return nil, fmt.Errorf("%w: %w", ErrListUnavailable, err)
	}
	defer func() { _ = f.Close() }()

	// Fingerprint the handle actually being read, not a stat taken earlier: a
	// rename between the two would record the old file's metadata against the
	// new file's contents, and every later request would re-parse.
	info, err := f.Stat()
	if err != nil {
		if cached := c.cached(); cached != nil {
			return cached, nil
		}
		return nil, fmt.Errorf("%w: %w", ErrListUnavailable, err)
	}

	domains, err := parseDomainList(f)
	if err != nil {
		if cached := c.cached(); cached != nil {
			return cached, nil
		}
		return nil, fmt.Errorf("%w: %w", ErrListUnavailable, err)
	}

	c.mu.Lock()
	c.domains = domains
	c.modTime = info.ModTime()
	c.size = info.Size()
	c.loadedAt = time.Now()
	c.mu.Unlock()

	c.log.Info("loaded blocked domains list", "path", c.listPath, "domains", len(domains))
	return domains, nil
}

func parseDomainList(r io.Reader) (map[string]struct{}, error) {
	domains := map[string]struct{}{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.ToLower(strings.TrimSpace(sc.Text()))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip an inline comment first. A domain never contains '#', so any '#'
		// starts a comment: "0.0.0.0 evil.com # note" must keep evil.com, not the
		// trailing "note" that taking the last field would otherwise store.
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}
		// Hosts-file feeds prefix entries with an address; keep the last field.
		if fields := strings.Fields(line); len(fields) > 1 {
			line = fields[len(fields)-1]
		}
		if line == "" || line == "localhost" {
			continue
		}
		domains[line] = struct{}{}
	}
	return domains, sc.Err()
}

// Refresh downloads the phishing feeds when the cached file is older than the
// configured interval. It is a no-op when the check is disabled.
func (c *Checker) Refresh(ctx context.Context) error {
	if !c.enabled || c.listPath == "" || len(c.feedURLs) == 0 {
		return nil
	}

	// Floor the interval: an unset or zero refresheAge would make every call see
	// the file as "older than 0" and re-download the multi-megabyte feed, at
	// boot (across the backoff retries) and on every tick.
	minAge := c.refresheAge
	if minAge <= 0 {
		minAge = time.Hour
	}
	if info, err := os.Stat(c.listPath); err == nil {
		if time.Since(info.ModTime()) < minAge {
			return nil
		}
	}

	bodies := make([][]byte, len(c.feedURLs))
	errs := make([]error, len(c.feedURLs))
	var wg sync.WaitGroup
	client := &http.Client{Timeout: 60 * time.Second}

	for i, feed := range c.feedURLs {
		feed = strings.TrimSpace(feed)
		if feed == "" {
			continue
		}
		wg.Add(1)
		go func(i int, feed string) {
			defer wg.Done()
			body, err := fetch(ctx, client, feed)
			if err != nil {
				c.log.Warn("phishing feed download failed", "url", feed, "error", err)
				errs[i] = err
				return
			}
			bodies[i] = body
		}(i, feed)
	}
	wg.Wait()

	// Every configured feed must succeed before the list is replaced. A partial
	// download would install a shorter list and silently un-block every domain
	// only the failing feed carried, so keep the existing list instead.
	for i, err := range errs {
		if err != nil {
			return fmt.Errorf("safety: phishing feed %q failed, keeping the existing list: %w",
				strings.TrimSpace(c.feedURLs[i]), err)
		}
	}

	var combined []byte
	for _, body := range bodies {
		if len(body) == 0 {
			continue
		}
		combined = append(combined, body...)
		combined = append(combined, '\n')
	}
	if len(combined) == 0 {
		return errors.New("safety: every phishing feed download failed")
	}

	if dir := filepath.Dir(c.listPath); dir != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("safety: create list directory: %w", err)
		}
	}

	// Write to a sibling temp file and rename, so a reader never observes a
	// half-written list and fails closed on it.
	tmp, err := os.CreateTemp(filepath.Dir(c.listPath), ".blocked_domains-*.tmp")
	if err != nil {
		return fmt.Errorf("safety: create temp list: %w", err)
	}
	tmpName := tmp.Name()
	// Best-effort cleanup: after a successful rename there is nothing left to
	// remove, and on failure the error being returned is the useful one.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(combined); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("safety: write temp list: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("safety: close temp list: %w", err)
	}
	if err := os.Rename(tmpName, c.listPath); err != nil {
		return fmt.Errorf("safety: install list: %w", err)
	}

	c.log.Info("refreshed phishing domain list", "path", c.listPath, "bytes", len(combined))
	return nil
}

func fetch(ctx context.Context, client *http.Client, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %s", resp.Status)
	}
	// Cap the download so a hostile or broken feed cannot exhaust memory.
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

// Enabled reports whether phishing checking is switched on.
func (c *Checker) Enabled() bool { return c.enabled }

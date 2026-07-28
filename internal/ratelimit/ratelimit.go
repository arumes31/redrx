// Package ratelimit implements fixed-window request limiting.
//
// Limit strings use the Flask-Limiter syntax the previous deployment
// configured, so existing RATELIMIT_* environment values keep their meaning:
//
//	"200 per day;50 per hour"   "10 per minute"   "5/hour"
package ratelimit

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Rule is one parsed "N per window" clause.
type Rule struct {
	Limit  int
	Window time.Duration
}

func (r Rule) String() string {
	return fmt.Sprintf("%d per %s", r.Limit, r.Window)
}

// Rules is a set of clauses that must all be satisfied.
type Rules []Rule

// Parse reads a Flask-Limiter limit string. An empty string yields no rules,
// which means unlimited.
func Parse(spec string) (Rules, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}

	var rules Rules
	for _, clause := range strings.FieldsFunc(spec, func(r rune) bool { return r == ';' || r == ',' }) {
		clause = strings.TrimSpace(clause)
		if clause == "" {
			continue
		}
		r, err := parseClause(clause)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, nil
}

// safeDefaultRules is applied when a configured limit string cannot be parsed.
// It fails closed to a conservative limit: a typo must not silently remove the
// protection a limit was there to provide.
var safeDefaultRules = Rules{{Limit: 60, Window: time.Minute}}

// MustParse is Parse for configuration defaults that are known to be valid.
// A malformed spec neither panics (a typo cannot take the service down at boot)
// nor disables the limit: it logs and falls back to a conservative default. An
// empty spec still means unlimited, which Parse reports without error.
func MustParse(spec string) Rules {
	r, err := Parse(spec)
	if err != nil {
		slog.Warn("ratelimit: invalid limit spec, applying a conservative default",
			"spec", spec, "default", "60 per minute", "error", err)
		return safeDefaultRules
	}
	return r
}

func parseClause(clause string) (Rule, error) {
	fields := strings.Fields(strings.ReplaceAll(clause, "/", " per "))
	// Accepted shapes: "10 per minute", "10 per 5 minutes".
	if len(fields) < 3 || !strings.EqualFold(fields[1], "per") {
		return Rule{}, fmt.Errorf("ratelimit: cannot parse %q", clause)
	}

	n, err := strconv.Atoi(fields[0])
	if err != nil || n <= 0 {
		return Rule{}, fmt.Errorf("ratelimit: bad limit count in %q", clause)
	}

	multiplier := 1
	unitField := fields[2]
	if len(fields) >= 4 {
		if multiplier, err = strconv.Atoi(fields[2]); err != nil || multiplier <= 0 {
			return Rule{}, fmt.Errorf("ratelimit: bad window multiplier in %q", clause)
		}
		unitField = fields[3]
	}

	unit, err := parseUnit(unitField)
	if err != nil {
		return Rule{}, fmt.Errorf("ratelimit: %w in %q", err, clause)
	}
	return Rule{Limit: n, Window: time.Duration(multiplier) * unit}, nil
}

func parseUnit(s string) (time.Duration, error) {
	switch strings.TrimSuffix(strings.ToLower(s), "s") {
	case "second", "sec":
		return time.Second, nil
	case "minute", "min":
		return time.Minute, nil
	case "hour":
		return time.Hour, nil
	case "day":
		return 24 * time.Hour, nil
	case "week":
		return 7 * 24 * time.Hour, nil
	case "month":
		return 30 * 24 * time.Hour, nil
	case "year":
		return 365 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown window unit %q", s)
	}
}

// Backend counts hits per key and window.
type Backend interface {
	// Incr adds one hit to the window bucket and reports the new total.
	Incr(ctx context.Context, key string, window time.Duration) (int64, error)
	// Ping reports backend health.
	Ping(ctx context.Context) error
	Close() error
}

// Limiter applies rule sets against a backend.
type Limiter struct {
	backend Backend
}

func New(backend Backend) *Limiter { return &Limiter{backend: backend} }

func (l *Limiter) Backend() Backend { return l.backend }

// Result reports the outcome of an Allow check.
type Result struct {
	Allowed    bool
	RetryAfter time.Duration
	Rule       Rule
}

// Allow records a hit for scope+identity and reports whether it is within every
// rule. A backend error fails open: availability matters more than a perfectly
// enforced limit, and the previous deployment behaved the same way.
func (l *Limiter) Allow(ctx context.Context, scope, identity string, rules Rules) Result {
	if l == nil || l.backend == nil || len(rules) == 0 {
		return Result{Allowed: true}
	}

	now := time.Now()
	for _, r := range rules {
		bucket := now.Truncate(r.Window).Unix()
		key := fmt.Sprintf("rl:%s:%s:%d:%d", scope, identity, int64(r.Window/time.Second), bucket)

		count, err := l.backend.Incr(ctx, key, r.Window)
		if err != nil {
			continue
		}
		if count > int64(r.Limit) {
			reset := now.Truncate(r.Window).Add(r.Window)
			return Result{Allowed: false, RetryAfter: time.Until(reset), Rule: r}
		}
	}
	return Result{Allowed: true}
}

// MemoryBackend counts in process memory. It is the default and matches the
// previous "memory://" storage URI.
type MemoryBackend struct {
	mu      sync.Mutex
	counts  map[string]*entry
	stop    chan struct{}
	stopped sync.Once
}

type entry struct {
	n       int64
	expires time.Time
}

func NewMemoryBackend() *MemoryBackend {
	m := &MemoryBackend{counts: map[string]*entry{}, stop: make(chan struct{})}
	go m.reap()
	return m
}

func (m *MemoryBackend) Incr(_ context.Context, key string, window time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	e, ok := m.counts[key]
	if !ok || now.After(e.expires) {
		e = &entry{expires: now.Add(window)}
		m.counts[key] = e
	}
	e.n++
	return e.n, nil
}

func (m *MemoryBackend) Ping(context.Context) error { return nil }

func (m *MemoryBackend) Close() error {
	m.stopped.Do(func() { close(m.stop) })
	return nil
}

// reap drops expired buckets so the map cannot grow without bound.
func (m *MemoryBackend) reap() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-m.stop:
			return
		case now := <-t.C:
			m.mu.Lock()
			for k, e := range m.counts {
				if now.After(e.expires) {
					delete(m.counts, k)
				}
			}
			m.mu.Unlock()
		}
	}
}

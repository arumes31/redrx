// Command redrx serves the URL shortener.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/redis/go-redis/v9"

	"github.com/arumes31/redrx/internal/config"
	"github.com/arumes31/redrx/internal/geo"
	"github.com/arumes31/redrx/internal/ratelimit"
	"github.com/arumes31/redrx/internal/safety"
	"github.com/arumes31/redrx/internal/store"
	"github.com/arumes31/redrx/internal/web"
)

func main() {
	if err := run(); err != nil {
		slog.Error("startup failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := newLogger(cfg)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		return err
	}
	log.Info("database ready", "dialect", dialectName(db.Dialect()))

	limiterBackend, cache := buildRateLimitBackend(cfg, log)
	defer func() { _ = limiterBackend.Close() }()

	checker := safety.New(safety.Options{
		Enabled:         cfg.EnablePhishingCheck,
		BlockedListPath: cfg.BlockedDomainsPath,
		FeedURLs:        cfg.PhishingListURLs,
		RefreshInterval: time.Duration(cfg.PhishingCheckInterval) * time.Hour,
		ManualDomains:   cfg.BlockedDomains,
		Logger:          log,
	})

	resolver := geo.New(geo.Options{
		DatabasePath:  cfg.GeoIPDBPath,
		UseCloudflare: cfg.UseCloudflare,
		Cache:         cache,
		Logger:        log,
	})
	// srv.Shutdown closes this too on the graceful path; Close is idempotent, so
	// this only covers the paths that return before the server is built.
	defer func() { _ = resolver.Close() }()

	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	srv, err := web.NewServer(web.Options{
		Config:   cfg,
		DB:       db,
		Logger:   log,
		Limiter:  ratelimit.New(limiterBackend),
		Safety:   checker,
		Geo:      resolver,
		Registry: registry,
	})
	if err != nil {
		return err
	}

	// Refresh the phishing feed in the background so a slow or unreachable
	// upstream never delays the first request. The WaitGroup lets shutdown wait
	// for the worker to stop touching db, checker and the limiter before the
	// deferred Close calls release them.
	var bg sync.WaitGroup
	bg.Add(1)
	go func() {
		defer bg.Done()
		maintainBlocklist(ctx, cfg, checker, db, log)
	}()

	httpServer := newHTTPServer(cfg.Listen, srv, slog.NewLogLogger(log.Handler(), slog.LevelWarn))

	errCh := make(chan error, 1)
	go func() {
		log.Info("redrx listening", "addr", cfg.Listen, "base_domain", cfg.BaseDomain)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		// The server never ran; stop the background worker and wait for it
		// before the deferred db/limiter Close calls run.
		stop()
		bg.Wait()
		return err
	case <-ctx.Done():
		log.Info("shutting down")
	}

	// Each stage gets its own budget: draining in-flight requests can consume
	// the whole timeout, and releasing the server's resources afterwards must
	// not inherit an already-expired deadline.
	httpCtx, cancelHTTP := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelHTTP()
	if err := httpServer.Shutdown(httpCtx); err != nil {
		log.Error("graceful shutdown failed", "error", err)
	}

	// ctx is already cancelled, so the worker's refresh/sweep return promptly.
	// Wait for it before the deferred db.Close and limiterBackend.Close run.
	bg.Wait()

	srvCtx, cancelSrv := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelSrv()
	return srv.Shutdown(srvCtx)
}

func newHTTPServer(addr string, handler http.Handler, errorLog *log.Logger) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          errorLog,
	}
}

func newLogger(cfg *config.Config) *slog.Logger {
	level := slog.LevelInfo
	if cfg.Debug {
		level = slog.LevelDebug
	}

	var handler slog.Handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	if cfg.AnonymizeLogs {
		handler = anonymizingHandler{handler}
	}
	return slog.New(handler)
}

// buildRateLimitBackend returns the configured limiter storage. A Redis URL
// also provides the GeoIP cache, so only one connection pool is opened.
func buildRateLimitBackend(cfg *config.Config, log *slog.Logger) (ratelimit.Backend, *redis.Client) {
	uri := cfg.RateLimitStorageURI
	if uri == "" || uri == "memory://" {
		return ratelimit.NewMemoryBackend(), nil
	}

	backend, err := ratelimit.NewRedisBackend(uri)
	if err != nil {
		log.Warn("falling back to in-memory rate limiting", "error", err)
		return ratelimit.NewMemoryBackend(), nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := backend.Ping(ctx); err != nil {
		log.Warn("redis unreachable, using in-memory rate limiting", "error", err)
		_ = backend.Close()
		return ratelimit.NewMemoryBackend(), nil
	}

	log.Info("using redis for rate limiting and geo cache")
	return backend, backend.Client()
}

// maintainBlocklist refreshes the phishing feed on PHISHING_CHECK_INTERVAL and,
// when auto-removal is on, sweeps links pointing at blocked domains on its own
// PHISHING_REMOVE_INTERVAL. The two are independent because they answer to
// different operator settings.
func maintainBlocklist(ctx context.Context, cfg *config.Config, checker *safety.Checker, db *store.DB, log *slog.Logger) {
	if !cfg.EnablePhishingCheck {
		return
	}

	refresh := func() {
		if err := checker.Refresh(ctx); err != nil {
			log.Warn("phishing list refresh failed", "error", err)
		}
	}

	// The sweep is safe to run on its own schedule: sweepBlockedLinks deletes
	// only on a positive blocklist match and aborts without deleting anything
	// when the list cannot be consulted, so it never empties the table on an
	// unreadable or absent list — the case the old refresh-then-sweep coupling
	// was guarding against.
	sweep := func() {
		if !cfg.EnableAutoRemovePhish {
			return
		}
		if err := sweepBlockedLinks(ctx, checker, db, log); err != nil {
			log.Warn("phishing sweep failed", "error", err)
		}
	}

	// Retry the first load with backoff. Until the list is on disk every URL is
	// rejected, so a brief upstream blip at container start would otherwise cost
	// a full interval — 24 hours by default — of the service refusing every
	// redirect and every new link.
	backoff := 5 * time.Second
	for attempt := 1; ; attempt++ {
		if err := checker.Refresh(ctx); err == nil {
			break
		} else if attempt >= 8 {
			log.Error("giving up on the initial phishing list download; "+
				"every URL will be rejected until the next scheduled refresh",
				"attempts", attempt, "error", err)
			break
		} else {
			log.Warn("phishing list download failed, retrying",
				"attempt", attempt, "retry_in", backoff, "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 2*time.Minute {
			backoff *= 2
		}
	}

	// Apply the freshly loaded list once at boot.
	sweep()

	refreshTicker := time.NewTicker(intervalHours(cfg.PhishingCheckInterval))
	defer refreshTicker.Stop()

	// The sweep ticker exists only when auto-removal is enabled, so a disabled
	// sweep costs nothing.
	var sweepC <-chan time.Time
	if cfg.EnableAutoRemovePhish {
		sweepTicker := time.NewTicker(intervalHours(cfg.PhishingRemoveInterval))
		defer sweepTicker.Stop()
		sweepC = sweepTicker.C
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-refreshTicker.C:
			refresh()
		case <-sweepC:
			sweep()
		}
	}
}

// intervalHours turns an hour count into a ticker interval, falling back to a
// day for a non-positive value.
func intervalHours(hours int) time.Duration {
	if hours <= 0 {
		return 24 * time.Hour
	}
	return time.Duration(hours) * time.Hour
}

// sweepBlockedLinks deletes links whose destination or any rotation target is
// now on the blocklist.
//
// It deletes only on a positive blocklist match. IsSafeURL cannot be used here:
// it reports false both for "this domain is blocked" and for "the blocklist
// could not be consulted", and the second collapses into the first. Serving
// traffic that way is the correct fail-closed choice — deleting rows on it is
// not. An unreadable list would otherwise mean every URL is unsafe and this
// function would empty the links table, which is exactly the state a fresh
// container is in before its first feed download succeeds.
func sweepBlockedLinks(ctx context.Context, checker *safety.Checker, db *store.DB, log *slog.Logger) error {
	// blocked reports a definite match. An error means the list is unavailable,
	// which aborts the sweep rather than condemning the row.
	blocked := func(target string) (bool, error) {
		// A row whose URL does not parse cannot match a domain. Skip it instead
		// of treating "unparseable" as "blocked".
		if !safety.IsAbsoluteHTTPURL(target) {
			return false, nil
		}
		ok, err := checker.CheckURL(target)
		if err != nil {
			return false, err
		}
		return !ok, nil
	}

	var doomed []int64

	err := db.EachURL(ctx, func(u *store.URL) error {
		hit, err := blocked(u.LongURL)
		if err != nil {
			return err
		}
		if hit {
			doomed = append(doomed, u.ID)
			return nil
		}
		for _, t := range u.RotateTargets {
			hit, err := blocked(t)
			if err != nil {
				return err
			}
			if hit {
				doomed = append(doomed, u.ID)
				return nil
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("sweep aborted without deleting anything: %w", err)
	}

	for _, id := range doomed {
		if err := db.DeleteURL(ctx, id); err != nil {
			log.Warn("could not remove blocked link", "id", id, "error", err)
		}
	}
	if len(doomed) > 0 {
		log.Info("removed links pointing at blocked domains", "count", len(doomed))
	}
	return nil
}

func dialectName(d store.Dialect) string {
	if d == store.Postgres {
		return "postgres"
	}
	return "sqlite"
}

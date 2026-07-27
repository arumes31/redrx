// Command redrx serves the URL shortener.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
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
	// upstream never delays the first request.
	go maintainBlocklist(ctx, cfg, checker, db, log)

	httpServer := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("redrx listening", "addr", cfg.Listen, "base_domain", cfg.BaseDomain)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
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

	srvCtx, cancelSrv := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelSrv()
	return srv.Shutdown(srvCtx)
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

// maintainBlocklist refreshes the phishing feed at boot and on a timer, and
// optionally sweeps links that now point at blocked domains.
func maintainBlocklist(ctx context.Context, cfg *config.Config, checker *safety.Checker, db *store.DB, log *slog.Logger) {
	if !cfg.EnablePhishingCheck {
		return
	}

	interval := time.Duration(cfg.PhishingCheckInterval) * time.Hour
	if interval <= 0 {
		interval = 24 * time.Hour
	}

	run := func() {
		if err := checker.Refresh(ctx); err != nil {
			log.Warn("phishing list refresh failed", "error", err)
			// Do not sweep on a stale or absent list: the sweep deletes rows,
			// and acting on a list we could not refresh risks deleting on the
			// basis of data we do not have.
			if cfg.EnableAutoRemovePhish {
				log.Warn("skipping phishing sweep because the list could not be refreshed")
			}
			return
		}
		if cfg.EnableAutoRemovePhish {
			if err := sweepBlockedLinks(ctx, checker, db, log); err != nil {
				log.Warn("phishing sweep failed", "error", err)
			}
		}
	}

	run()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
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

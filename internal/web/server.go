// Package web serves the HTML site and the JSON API.
package web

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/arumes31/redrx/internal/config"
	"github.com/arumes31/redrx/internal/geo"
	"github.com/arumes31/redrx/internal/ratelimit"
	"github.com/arumes31/redrx/internal/safety"
	"github.com/arumes31/redrx/internal/session"
	"github.com/arumes31/redrx/internal/store"
)

// Server holds everything the handlers need.
type Server struct {
	cfg      *config.Config
	db       *store.DB
	log      *slog.Logger
	renderer *renderer
	sessions *session.Manager
	limiter  *ratelimit.Limiter
	safety   *safety.Checker
	geo      *geo.Resolver
	metrics  *metrics
	limits   limits
	registry *prometheus.Registry

	handler http.Handler
}

// limits holds the parsed rate-limit rules, resolved once at boot.
type limits struct {
	Default   ratelimit.Rules
	Login     ratelimit.Rules
	Register  ratelimit.Rules
	Auth      ratelimit.Rules
	API       ratelimit.Rules
	Create    ratelimit.Rules
	Redirect  ratelimit.Rules
	Stats     ratelimit.Rules
	QR        ratelimit.Rules
	Pages     ratelimit.Rules
	Dashboard ratelimit.Rules
	Export    ratelimit.Rules
	Bulk      ratelimit.Rules
}

type Options struct {
	Config   *config.Config
	DB       *store.DB
	Logger   *slog.Logger
	Limiter  *ratelimit.Limiter
	Safety   *safety.Checker
	Geo      *geo.Resolver
	Registry *prometheus.Registry
}

func NewServer(opts Options) (*Server, error) {
	r, err := newRenderer()
	if err != nil {
		return nil, err
	}

	registry := opts.Registry
	if registry == nil {
		registry = prometheus.NewRegistry()
	}

	s := &Server{
		cfg:      opts.Config,
		db:       opts.DB,
		log:      opts.Logger,
		renderer: r,
		// Cookies are marked Secure unless running in debug, where the dev
		// server is plain HTTP and a Secure cookie would never be sent back.
		sessions: session.NewManager(opts.Config.SecretKey, !opts.Config.Debug),
		limiter:  opts.Limiter,
		safety:   opts.Safety,
		geo:      opts.Geo,
		metrics:  newMetrics(registry),
		registry: registry,
	}

	s.limits = limits{
		Default:   ratelimit.MustParse(s.cfg.RateLimitDefault),
		Login:     ratelimit.MustParse(s.cfg.RateLimitLogin),
		Register:  ratelimit.MustParse(s.cfg.RateLimitRegister),
		Auth:      ratelimit.MustParse(s.cfg.RateLimitAuth),
		API:       ratelimit.MustParse(s.cfg.RateLimitAPI),
		Create:    ratelimit.MustParse(s.cfg.RateLimitCreate),
		Redirect:  ratelimit.MustParse(s.cfg.RateLimitRedirect),
		Stats:     ratelimit.MustParse("20 per minute"),
		QR:        ratelimit.MustParse("30 per minute"),
		Pages:     ratelimit.MustParse("30 per minute"),
		Dashboard: ratelimit.MustParse("60 per minute"),
		Export:    ratelimit.MustParse("5 per minute"),
		Bulk:      ratelimit.MustParse("10 per minute"),
	}

	s.handler, err = s.routes()
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

func (s *Server) routes() (http.Handler, error) {
	mux := http.NewServeMux()

	staticSub, err := staticFiles()
	if err != nil {
		return nil, fmt.Errorf("mount static assets: %w", err)
	}
	fileServer := http.FileServer(http.FS(staticSub))
	mux.Handle("GET /static/", cacheStatic(http.StripPrefix("/static/", fileServer)))

	// Home and link creation. Each route carries a distinct scope, since the
	// rate-limit key is namespaced by it: routes that shared a scope shared one
	// counter, so viewing the login page could spend the budget meant for the
	// terms page. Scopes are grouped only where routes are genuinely the same
	// activity — the two halves of one form, the dashboard actions, the API.
	mux.Handle("GET /{$}", s.limit("home", s.limits.Default, s.handleIndex))
	mux.Handle("POST /{$}", s.limit("create", s.limits.Create, s.handleCreate))

	// Accounts. The GET form views are separate scopes from the POST actions, so
	// loading the login page never consumes the anti-brute-force login budget.
	mux.Handle("GET /login", s.limit("login_page", s.limits.Pages, s.handleLoginForm))
	mux.Handle("POST /login", s.limit("login", s.limits.Login, s.handleLogin))
	mux.Handle("GET /register", s.limit("register_page", s.limits.Pages, s.handleRegisterForm))
	mux.Handle("POST /register", s.limit("register", s.limits.Register, s.handleRegister))
	// POST only: a GET logout is triggered by any third-party <img> tag, and by
	// link prefetchers. The nav uses a form.
	mux.Handle("POST /logout", s.wrap(s.handleLogout))

	// Dashboard and link management.
	mux.Handle("GET /dashboard", s.limit("dashboard", s.limits.Dashboard, s.requireLogin(s.handleDashboard)))
	// Its own scope: regenerating a key is unrelated to unlocking a link, and
	// the two shared the "auth" counter before.
	mux.Handle("POST /regenerate-api-key", s.limit("regen_key", s.limits.Auth, s.requireLogin(s.handleRegenerateAPIKey)))
	mux.Handle("POST /toggle-status/{code}", s.limit("dashboard", s.limits.Dashboard, s.requireLogin(s.handleToggleStatus)))
	mux.Handle("GET /export-links", s.limit("export", s.limits.Export, s.requireLogin(s.handleExportLinks)))
	mux.Handle("POST /bulk-delete", s.limit("bulk", s.limits.Bulk, s.requireLogin(s.handleBulkDelete)))
	mux.Handle("GET /edit/{code}", s.limit("dashboard", s.limits.Dashboard, s.requireLogin(s.handleEditForm)))
	mux.Handle("POST /edit/{code}", s.limit("dashboard", s.limits.Dashboard, s.requireLogin(s.handleEdit)))
	mux.Handle("POST /delete/{code}", s.limit("dashboard", s.limits.Dashboard, s.requireLogin(s.handleDelete)))

	// Static content pages, each on its own counter.
	mux.Handle("GET /api-docs", s.limit("apidocs", s.limits.Pages, s.handleAPIDocs))
	mux.Handle("GET /data-usage", s.limit("datausage", s.limits.Pages, s.handleDataUsage))
	mux.Handle("GET /terms", s.limit("terms", s.limits.Pages, s.handleTerms))
	mux.Handle("GET /robots.txt", s.wrap(s.handleRobots))
	mux.Handle("GET /sitemap.xml", s.wrap(s.handleSitemap))

	// Operations. Deliberately unlimited, and deliberately not configurable: a
	// Kubernetes liveness+readiness pair at the default 10s interval is 12
	// requests a minute, which a 10/min limit would 429 into a
	// CrashLoopBackOff, and a scrape that drops metrics under load drops them
	// exactly when they are needed. These are also exempted from the
	// canonical-domain redirect below.
	mux.Handle("GET /health", s.wrap(s.handleHealth))
	mux.Handle("GET /metrics", s.wrap(s.handleMetrics))

	// JSON API. Read and write each get the configured RATELIMIT_API budget on
	// their own counter, so a client polling link details cannot exhaust the
	// budget for creating links (and vice versa). Both still honour the same
	// operator-configured rate; they simply no longer share one bucket.
	mux.Handle("POST /api/v1/shorten", s.limit("api_write", s.limits.API, s.handleAPIShorten))
	mux.Handle("GET /api/v1/{code}", s.limit("api_read", s.limits.API, s.handleAPIGetURL))

	// Link password gate. GET and POST share one scope — they are the two halves
	// of unlocking a link — but no longer share with regenerate-api-key.
	mux.Handle("GET /link-auth/{code}", s.limit("link_auth", s.limits.Auth, s.handleLinkAuthForm))
	mux.Handle("POST /link-auth/{code}", s.limit("link_auth", s.limits.Auth, s.handleLinkAuth))

	// Short codes cannot be registered as `/{code}` patterns: they would overlap
	// the literal routes above (`/edit/{code}` and `/{code}/stats` both match
	// "/edit/stats"), which ServeMux rejects as ambiguous. They are dispatched
	// from the catch-all instead, which every pattern above outranks.
	mux.Handle("/", s.shortCodeRouter())

	// Outermost first: panic recovery must see everything, and the proxy
	// headers must be applied before anything reads the client IP.
	var h http.Handler = mux
	h = s.canonicalDomain(h)
	h = s.securityHeaders(h)
	h = s.instrument(h)
	h = s.proxyHeaders(h)
	h = s.recoverPanics(h)
	return h, nil
}

// Shutdown releases resources held by the server's dependencies.
func (s *Server) Shutdown(context.Context) error {
	if s.geo != nil {
		return s.geo.Close()
	}
	return nil
}

// cacheStatic adds a long cache lifetime to the embedded, versioned assets.
func cacheStatic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		next.ServeHTTP(w, r)
	})
}

// metrics holds the Prometheus collectors, mirroring the counter names the
// previous deployment exported so existing dashboards keep working.
type metrics struct {
	shortened   prometheus.Counter
	redirects   prometheus.Counter
	rateLimited *prometheus.CounterVec
	requests    *prometheus.CounterVec
	duration    *prometheus.HistogramVec
}

func newMetrics(reg *prometheus.Registry) *metrics {
	m := &metrics{
		shortened: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "redrx_shortened_links_total",
			Help: "Total number of shortened links created",
		}),
		redirects: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "redrx_redirections_total",
			Help: "Total number of link redirections",
		}),
		rateLimited: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "redrx_ratelimit_hits_total",
			Help: "Requests rejected by the rate limiter, labelled by scope",
		}, []string{"scope"}),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "redrx_http_requests_total",
			Help: "HTTP requests by method, route and status",
		}, []string{"method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "redrx_http_request_duration_seconds",
			Help:    "HTTP request latency by method and route",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route"}),
	}
	reg.MustRegister(m.shortened, m.redirects, m.rateLimited, m.requests, m.duration)
	return m
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	promhttp.HandlerFor(s.registry, promhttp.HandlerOpts{}).ServeHTTP(w, r)
}

// handleHealth reports database and rate-limit backend health.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := map[string]any{"status": "healthy"}
	checks := map[string]string{}

	// Each dependency gets its own budget. Sharing one deadline made a slow
	// database ping exhaust it, so the rate-limit ping then failed against a
	// perfectly healthy Redis and reported a false error.
	dbCtx, cancelDB := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancelDB()
	if err := s.db.PingContext(dbCtx); err != nil {
		// The message is logged, not returned: driver errors routinely name the
		// host, port, database and user, and this endpoint is unauthenticated.
		s.log.Error("health check: database unreachable", "error", err)
		health["status"] = "unhealthy"
		checks["database"] = "error"
	} else {
		checks["database"] = "ok"
	}

	if backend := s.limiter.Backend(); backend != nil {
		rlCtx, cancelRL := context.WithTimeout(r.Context(), 1*time.Second)
		defer cancelRL()
		if err := backend.Ping(rlCtx); err != nil {
			s.log.Warn("health check: rate limit backend unreachable", "error", err)
			checks["ratelimit"] = "error"
			if health["status"] == "healthy" {
				health["status"] = "degraded"
			}
		} else {
			checks["ratelimit"] = "ok"
		}
	}

	health["checks"] = checks

	// Only a hard failure (the database) is Unavailable. A "degraded" status —
	// the rate-limit backend being down, which the limiter fails open on — must
	// still return 200, or a Redis blip would pull every replica out of the load
	// balancer over a dependency the request path tolerates.
	status := http.StatusOK
	if health["status"] == "unhealthy" {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, health)
}

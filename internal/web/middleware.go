package web

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/arumes31/redrx/internal/ratelimit"
	"github.com/arumes31/redrx/internal/session"
	"github.com/arumes31/redrx/internal/store"
)

type ctxKey int

const (
	ctxSession ctxKey = iota
	ctxUser
	ctxRoute
)

// reqContext bundles the per-request values handlers need.
type reqContext struct {
	Session *session.Session
	User    *store.User
}

// handlerFunc is a handler that receives the resolved session and user.
type handlerFunc func(http.ResponseWriter, *http.Request)

// wrap attaches session loading and CSRF verification to a handler.
func (s *Server) wrap(h handlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := s.sessions.Load(r)

		var user *store.User
		if sess.UserID != 0 {
			u, err := s.db.UserByID(r.Context(), sess.UserID)
			switch {
			case err == nil:
				user = u
			case errors.Is(err, store.ErrNotFound):
				// The account was deleted out from under the cookie.
				sess.Logout()
			default:
				s.log.Error("load session user", "error", err)
			}
		}

		ctx := context.WithValue(r.Context(), ctxSession, sess)
		ctx = context.WithValue(ctx, ctxUser, user)
		r = r.WithContext(ctx)

		if isStateChanging(r.Method) && !s.checkCSRF(r, sess) {
			s.sessions.Save(w, sess)
			s.renderError(w, r, http.StatusForbidden)
			return
		}

		// The session must be written before the handler sends a body, since
		// headers cannot be set afterwards.
		sw := &sessionWriter{ResponseWriter: w, save: func(w http.ResponseWriter) {
			s.sessions.Save(w, sess)
		}}
		h(sw, r)
		sw.flushSession()
	})
}

// limit applies a rate-limit rule set on top of wrap.
func (s *Server) limit(scope string, rules ratelimit.Rules, h handlerFunc) http.Handler {
	inner := s.wrap(h)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := s.geo.ClientIP(r)
		if res := s.limiter.Allow(r.Context(), scope, ip, rules); !res.Allowed {
			s.metrics.rateLimited.Inc()
			s.renderRateLimited(w, r, res.RetryAfter)
			return
		}
		inner.ServeHTTP(w, r)
	})
}

// requireLogin rejects anonymous callers, redirecting browsers to the login
// page with a `next` parameter.
func (s *Server) requireLogin(h handlerFunc) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if userFrom(r) == nil {
			sess := sessionFrom(r)
			sess.AddFlash("info", "Please log in to access this page.")
			next := url.QueryEscape(r.URL.RequestURI())
			http.Redirect(w, r, "/login?next="+next, http.StatusSeeOther)
			return
		}
		h(w, r)
	}
}

func sessionFrom(r *http.Request) *session.Session {
	if v, ok := r.Context().Value(ctxSession).(*session.Session); ok {
		return v
	}
	return &session.Session{}
}

func userFrom(r *http.Request) *store.User {
	if v, ok := r.Context().Value(ctxUser).(*store.User); ok {
		return v
	}
	return nil
}

func isStateChanging(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// checkCSRF validates the token on state-changing requests. The API is exempt
// because it authenticates with a header key rather than a cookie, so it is not
// reachable by a cross-site form post.
func (s *Server) checkCSRF(r *http.Request, sess *session.Session) bool {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		return true
	}
	token := r.Header.Get("X-CSRFToken")
	if token == "" {
		token = r.Header.Get("X-CSRF-Token")
	}
	if token == "" {
		// Parsing here is safe: handlers call ParseForm again and it is
		// idempotent, and multipart bodies are handled by the handler itself.
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			_ = r.ParseForm()
			token = r.PostFormValue("csrf_token")
		} else if err := r.ParseMultipartForm(s.cfg.MaxUploadSize); err == nil {
			token = r.PostFormValue("csrf_token")
		}
	}
	return sess.ValidCSRF(token)
}

// sessionWriter writes the session cookie exactly once, immediately before the
// first byte of the response body.
type sessionWriter struct {
	http.ResponseWriter
	save    func(http.ResponseWriter)
	written bool
}

func (w *sessionWriter) flushSession() {
	if !w.written {
		w.written = true
		w.save(w.ResponseWriter)
	}
}

func (w *sessionWriter) WriteHeader(status int) {
	w.flushSession()
	w.ResponseWriter.WriteHeader(status)
}

func (w *sessionWriter) Write(b []byte) (int, error) {
	w.flushSession()
	return w.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *sessionWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// proxyHeaders applies X-Forwarded-* from a trusted reverse proxy, replacing
// the ProxyFix middleware the previous deployment used.
func (s *Server) proxyHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// The rightmost entry is the one the trusted proxy appended; the
			// leftmost is client-controlled and must not be trusted blindly.
			parts := strings.Split(xff, ",")
			if ip := strings.TrimSpace(parts[len(parts)-1]); ip != "" {
				if _, port, err := net.SplitHostPort(r.RemoteAddr); err == nil {
					r.RemoteAddr = net.JoinHostPort(ip, port)
				} else {
					r.RemoteAddr = ip
				}
			}
		}
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			r.URL.Scheme = strings.TrimSpace(strings.Split(proto, ",")[0])
		}
		if host := r.Header.Get("X-Forwarded-Host"); host != "" {
			r.Host = strings.TrimSpace(strings.Split(host, ",")[0])
		}
		next.ServeHTTP(w, r)
	})
}

// securityHeaders sets the same headers the previous after_request hook did.
func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy",
			"default-src 'self' 'unsafe-inline' 'unsafe-eval' https://fonts.googleapis.com https://fonts.gstatic.com data:;")
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// canonicalDomain redirects requests that arrive on a non-canonical host, so
// links always resolve under BASE_DOMAIN.
func (s *Server) canonicalDomain(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := s.cfg.CanonicalHost()
		host := r.Host

		if s.cfg.Debug || base == "" || host == base ||
			strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1") {
			next.ServeHTTP(w, r)
			return
		}

		// Only the path and query carry over; anything that could inject a host
		// is dropped so this cannot become an open redirect.
		target := &url.URL{Scheme: "https", Host: base, Path: r.URL.Path, RawQuery: r.URL.RawQuery}
		http.Redirect(w, r, target.String(), http.StatusMovedPermanently)
	})
}

// instrument records request counts and latency.
func (s *Server) instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		route := routeLabel(r)
		s.metrics.requests.WithLabelValues(r.Method, route, statusClass(rec.status)).Inc()
		s.metrics.duration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
	})
}

// recoverPanics turns a panic into a 500 instead of a dropped connection.
func (s *Server) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("panic serving request", "path", r.URL.Path, "panic", rec)
				s.renderError(w, r, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if !r.written {
		r.status = status
		r.written = true
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.written = true
	return r.ResponseWriter.Write(b)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// routeLabel keeps metric cardinality bounded by reporting the matched pattern
// rather than the raw path, which contains user-chosen short codes.
func routeLabel(r *http.Request) string {
	// The short-code router sets its own label, since its requests all match
	// the catch-all pattern.
	if v, ok := r.Context().Value(ctxRoute).(string); ok && v != "" {
		return v
	}
	if p := r.Pattern; p != "" {
		return p
	}
	return "other"
}

func statusClass(status int) string {
	switch {
	case status < 200:
		return "1xx"
	case status < 300:
		return "2xx"
	case status < 400:
		return "3xx"
	case status < 500:
		return "4xx"
	default:
		return "5xx"
	}
}

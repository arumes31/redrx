package web

import (
	"context"
	"net/http"
	"strings"
)

// shortCodeRouter dispatches the short-code URL space: `/{code}`,
// `/{code}/stats` and `/{code}/qr`.
//
// These cannot be ServeMux patterns. `/{code}/stats` and `/edit/{code}` both
// match "/edit/stats" with neither being more specific, and ServeMux panics on
// such a pair. Routing them here keeps the public URLs unchanged — existing
// links, QR codes and shared stats pages all still resolve — while letting
// every literal route above win by being registered as a real pattern.
func (s *Server) shortCodeRouter() http.Handler {
	redirect := s.limit("redirect", s.limits.Redirect, s.handleRedirect)
	stats := s.limit("stats", s.limits.Stats, s.handleStats)
	qr := s.limit("qr", s.limits.QR, s.handleQR)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			s.renderError(w, r, http.StatusNotFound)
			return
		}

		segments := splitPath(r.URL.Path)

		switch {
		case len(segments) == 1:
			s.dispatch(w, r, redirect, segments[0], "GET /{code}")
		case len(segments) == 2 && segments[1] == "stats":
			s.dispatch(w, r, stats, segments[0], "GET /{code}/stats")
		case len(segments) == 2 && segments[1] == "qr":
			s.dispatch(w, r, qr, segments[0], "GET /{code}/qr")
		default:
			s.renderError(w, r, http.StatusNotFound)
		}
	})
}

// dispatch runs a short-code handler with the code and a metrics label
// attached, standing in for the PathValue and Pattern a ServeMux match sets.
func (s *Server) dispatch(w http.ResponseWriter, r *http.Request, h http.Handler, code, label string) {
	r.SetPathValue("code", code)
	r = r.WithContext(context.WithValue(r.Context(), ctxRoute, label))
	h.ServeHTTP(w, r)
}

// splitPath returns the non-empty path segments, with percent-escapes already
// decoded by net/http.
func splitPath(p string) []string {
	trimmed := strings.Trim(p, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// newPageData assembles the common template context for a request.
func (s *Server) newPageData(r *http.Request) *PageData {
	sess := sessionFrom(r)
	scheme := "https"
	if s.cfg.Debug {
		scheme = "http"
	}
	data := &PageData{
		Config:      s.cfg,
		User:        userFrom(r),
		Flashes:     sess.TakeFlashes(),
		CSRFToken:   sess.CSRFToken(),
		RequestPath: r.URL.Path,
		RequestURL:  scheme + "://" + r.Host + r.URL.Path,
		Data:        map[string]any{},
	}
	data.Data["show_consent_banner"] = s.cfg.EnableConsentBanner && s.consentChoice(r) == ""
	return data
}

// render writes a page, falling back to a plain 500 if the template fails.
func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, name string, data *PageData) {
	if err := s.renderer.Render(w, status, name, data); err != nil {
		s.log.Error("render template", "template", name, "path", r.URL.Path, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// renderError shows one of the styled error pages.
func (s *Server) renderError(w http.ResponseWriter, r *http.Request, status int) {
	// API callers get JSON, not an HTML page.
	if wantsJSON(r) {
		writeJSON(w, status, map[string]string{"error": http.StatusText(status)})
		return
	}

	template := "500.html"
	switch status {
	case http.StatusNotFound:
		template = "404.html"
	case http.StatusForbidden:
		template = "403.html"
	case http.StatusGone:
		template = "410.html"
	case http.StatusTooManyRequests:
		template = "429.html"
	}

	data := s.newPageData(r)
	if err := s.renderer.Render(w, status, template, data); err != nil {
		s.log.Error("render error page", "template", template, "error", err)
		http.Error(w, http.StatusText(status), status)
	}
}

// renderRateLimited serves the 429 page with the client's IP and country, as
// the previous handler did.
func (s *Server) renderRateLimited(w http.ResponseWriter, r *http.Request, retryAfter time.Duration) {
	if retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
	}

	if wantsJSON(r) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "Rate limit exceeded"})
		return
	}

	ip := s.geo.ClientIP(r)
	data := &PageData{
		Config:      s.cfg,
		RequestPath: r.URL.Path,
		Data: map[string]any{
			"client_ip":      ip,
			"client_country": s.geo.Country(r.Context(), ip, r),
		},
	}
	if err := s.renderer.Render(w, http.StatusTooManyRequests, "429.html", data); err != nil {
		s.log.Error("render 429 page", "error", err)
		http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
	}
}

// wantsJSON reports whether the caller expects a JSON error body.
func wantsJSON(r *http.Request) bool {
	if hasPrefixFold(r.URL.Path, "/api/") {
		return true
	}
	accept := r.Header.Get("Accept")
	return accept == "application/json" ||
		(hasSubstringFold(accept, "application/json") && !hasSubstringFold(accept, "text/html"))
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		// The status line is already sent, so there is nothing left to do but
		// stop writing.
		return
	}
}

func apiError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && equalFold(s[:len(prefix)], prefix)
}

func hasSubstringFold(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if equalFold(s[i:i+len(sub)], sub) {
			return true
		}
	}
	return false
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

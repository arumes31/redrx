package web

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mileusna/useragent"

	"github.com/arumes31/redrx/internal/geo"
	"github.com/arumes31/redrx/internal/qr"
	"github.com/arumes31/redrx/internal/security"
	"github.com/arumes31/redrx/internal/shortcode"
	"github.com/arumes31/redrx/internal/store"
)

// lookupTimeout bounds the short-code lookup on the visitor-facing paths. A
// point lookup on the unique short_code index is sub-millisecond, so this only
// ever fires when the database is unreachable — where connect_timeout bounds a
// single connection attempt but database/sql retries it two or three times,
// stacking into ~8s. The deadline caps the whole thing so a redirect fails fast
// during a DB outage instead of holding the connection and the visitor.
const lookupTimeout = 3 * time.Second

func (s *Server) handleRedirect(w http.ResponseWriter, r *http.Request) {
	// Codes are stored uppercase, so the lookup normalises exactly as the stats
	// and QR handlers do; without it "/abc123" would 404 while "/abc123/stats"
	// resolved.
	code := shortcode.Normalize(r.PathValue("code"))

	ctx, cancel := context.WithTimeout(r.Context(), lookupTimeout)
	defer cancel()
	link, err := s.db.URLByShortCode(ctx, code)
	if errors.Is(err, store.ErrNotFound) {
		s.renderError(w, r, http.StatusNotFound)
		return
	}
	if err != nil {
		s.log.Error("load link for redirect", "code", code, "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}

	if !link.IsActive() {
		s.renderError(w, r, http.StatusGone)
		return
	}

	if link.IsPasswordProtected() && !sessionFrom(r).IsLinkAuthorized(link.ShortCode) {
		http.Redirect(w, r, "/link-auth/"+url.PathEscape(link.ShortCode), http.StatusSeeOther)
		return
	}

	ua := useragent.Parse(r.Header.Get("User-Agent"))
	target := s.selectTarget(link, ua)

	// Re-check at redirect time: the blocklist may have grown since the link
	// was created.
	if !s.safety.IsSafeURL(target) {
		s.renderError(w, r, http.StatusForbidden)
		return
	}

	if err := s.db.TouchLastAccessed(r.Context(), link.ID, time.Now().UTC()); err != nil {
		s.log.Warn("update last accessed", "code", link.ShortCode, "error", err)
	}
	s.metrics.redirects.Inc()

	if link.StatsEnabled {
		s.recordClick(r, link, ua)
	}

	if link.PreviewMode {
		data := s.newPageData(r)
		data.Data["target_url"] = target
		data.Data["short_code"] = link.ShortCode
		s.render(w, r, http.StatusOK, "preview.html", data)
		return
	}

	// The interstitial matches the previous behaviour: a countdown page rather
	// than an HTTP redirect, so the destination is always shown first.
	if err := s.renderer.Render(w, http.StatusOK, "redirect.html", struct{ TargetURL string }{target}); err != nil {
		s.log.Error("render redirect page", "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
	}
}

// selectTarget picks the destination for this visitor: a device-specific URL
// when one matches, otherwise a rotation target, otherwise the main URL.
func (s *Server) selectTarget(link *store.URL, ua useragent.UserAgent) string {
	if link.IOSTargetURL != "" && ua.IsIOS() {
		return link.IOSTargetURL
	}
	if link.AndroidTargetURL != "" && ua.IsAndroid() {
		return link.AndroidTargetURL
	}

	if len(link.RotateTargets) > 0 {
		safe := make([]string, 0, len(link.RotateTargets))
		for _, t := range link.RotateTargets {
			if s.safety.IsSafeURL(t) {
				safe = append(safe, t)
			}
		}
		if len(safe) > 0 {
			// Traffic splitting, not a security decision — an observer gaining
			// nothing from predicting which of the operator's own targets is
			// served next. A CSPRNG here would only cost entropy per redirect.
			return safe[rand.IntN(len(safe))] // #nosec G404 -- load distribution, not secrecy
		}
	}
	return link.LongURL
}

// recordClick stores an anonymised analytics row for the redirect.
func (s *Server) recordClick(r *http.Request, link *store.URL, ua useragent.UserAgent) {
	ip := s.geo.ClientIP(r)
	country := s.geo.Country(r.Context(), ip, r)

	referrer := r.Referer()
	if referrer == "" {
		referrer = "Direct"
	}

	// Truncate to the column widths the schema declares. A crafted or unusual
	// User-Agent can yield a browser/platform string over 50 chars, which
	// Postgres rejects outright (VARCHAR(50)) and drops the click entirely.
	click := &store.Click{
		URLID:     link.ID,
		Timestamp: time.Now().UTC(),
		IPAddress: truncate(geo.AnonymizeIP(ip), 45),
		Country:   truncate(country, 100),
		Browser:   truncate(browserName(ua), 50),
		Platform:  truncate(firstNonEmpty(ua.OS, "Unknown"), 50),
		Referrer:  truncate(referrer, 255),
	}
	if err := s.db.RecordClick(r.Context(), click); err != nil {
		s.log.Error("record click", "code", link.ShortCode, "error", err)
	}
}

func (s *Server) handleLinkAuthForm(w http.ResponseWriter, r *http.Request) {
	code := shortcode.Normalize(r.PathValue("code"))
	if _, err := s.db.URLByShortCode(r.Context(), code); err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			s.log.Error("load link for auth form", "code", code, "error", err)
			s.renderError(w, r, http.StatusInternalServerError)
			return
		}
		s.renderError(w, r, http.StatusNotFound)
		return
	}

	data := s.newPageData(r)
	data.Data["short_code"] = code
	s.render(w, r, http.StatusOK, "login.html", data)
}

func (s *Server) handleLinkAuth(w http.ResponseWriter, r *http.Request) {
	code := shortcode.Normalize(r.PathValue("code"))
	sess := sessionFrom(r)

	link, err := s.db.URLByShortCode(r.Context(), code)
	if errors.Is(err, store.ErrNotFound) {
		s.renderError(w, r, http.StatusNotFound)
		return
	}
	if err != nil {
		s.log.Error("load link for auth", "code", code, "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}

	if link.IsPasswordProtected() && security.CheckPasswordHash(link.PasswordHash, r.PostFormValue("password")) {
		sess.AuthorizeLink(link.ShortCode)
		http.Redirect(w, r, "/"+url.PathEscape(link.ShortCode), http.StatusSeeOther)
		return
	}

	sess.AddFlash("danger", "Invalid Password")
	data := s.newPageData(r)
	data.Data["short_code"] = code
	s.render(w, r, http.StatusOK, "login.html", data)
}

func (s *Server) handleQR(w http.ResponseWriter, r *http.Request) {
	code := shortcode.Normalize(r.PathValue("code"))

	link, err := s.db.URLByShortCode(r.Context(), code)
	if errors.Is(err, store.ErrNotFound) {
		s.renderError(w, r, http.StatusNotFound)
		return
	}
	if err != nil {
		s.log.Error("load link for qr", "code", code, "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}

	// Fall back to the instance defaults for links created before the colours
	// were stored, and for anyone who left them alone.
	png, err := qr.PNG(s.cfg.ShortURL(link.ShortCode), qr.Options{
		Foreground: firstNonEmpty(link.QRColor, s.cfg.DefaultQRColor),
		Background: firstNonEmpty(link.QRBackground, s.cfg.DefaultQRBG),
	})
	if err != nil {
		s.log.Error("render qr", "code", code, "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Disposition", `inline; filename="`+link.ShortCode+`.png"`)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(png)
}

// browserName normalises the parser's browser label. For Safari on iOS the
// library reports the device ("iPhone", "iPad") rather than the browser, which
// would put a device name in the analytics browser column and stop the stats
// page from showing the Safari icon.
func browserName(ua useragent.UserAgent) string {
	switch ua.Name {
	case "iPhone", "iPad", "iPod":
		return "Safari"
	case "":
		return "Unknown"
	default:
		return ua.Name
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// truncate limits s to n bytes, backing up to a rune boundary so a Referrer cut
// mid-character is never stored as invalid UTF-8.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func nowUTC() time.Time { return time.Now().UTC() }

// optionalTime renders a nullable timestamp for CSV export.
func optionalTime(t *time.Time) string {
	if t == nil {
		return "Never"
	}
	return t.Format("2006-01-02T15:04:05")
}

// io_ReadFull is io.ReadFull, named locally to keep the import list of the
// handlers file small.
func io_ReadFull(r io.Reader, buf []byte) (int, error) { return io.ReadFull(r, buf) }

// hostOf returns the hostname of a referrer URL for grouping in analytics.
func hostOf(raw string) string {
	if !strings.Contains(raw, "://") {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return u.Host
}

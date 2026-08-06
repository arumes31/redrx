package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/arumes31/redrx/internal/config"
	"github.com/arumes31/redrx/internal/session"
	"github.com/arumes31/redrx/internal/store"
)

//go:embed templates/*.html templates/*.txt templates/*.xml
var templateFS embed.FS

//go:embed all:static
var staticFS embed.FS

// pages lists every template rendered on top of base.html.
var pages = []string{
	"index.html", "login.html", "login_user.html", "register.html",
	"dashboard.html", "edit_url.html", "stats.html", "preview.html",
	"login_totp.html", "security_settings.html",
	"api_docs.html", "data_usage.html", "terms.html",
	"403.html", "404.html", "410.html", "429.html", "500.html",
}

// standalone lists templates rendered on their own, without the site chrome.
var standalone = []string{"redirect.html", "robots.txt", "sitemap.xml"}

type renderer struct {
	templates map[string]*template.Template
}

func newRenderer() (*renderer, error) {
	r := &renderer{templates: map[string]*template.Template{}}

	for _, page := range pages {
		t, err := template.New("base.html").Funcs(templateFuncs()).ParseFS(templateFS,
			"templates/base.html", "templates/"+page)
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", page, err)
		}
		r.templates[page] = t
	}

	for _, name := range standalone {
		t, err := template.New(name).Funcs(templateFuncs()).ParseFS(templateFS, "templates/"+name)
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", name, err)
		}
		r.templates[name] = t
	}
	return r, nil
}

// Render writes a page. It buffers first so a template error surfaces as a 500
// rather than as a truncated page with a 200 already committed.
func (r *renderer) Render(w http.ResponseWriter, status int, name string, data any) error {
	t, ok := r.templates[name]
	if !ok {
		return fmt.Errorf("unknown template %q", name)
	}

	root := "base.html"
	for _, s := range standalone {
		if s == name {
			root = name
			break
		}
	}

	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, root, data); err != nil {
		return fmt.Errorf("render %s: %w", name, err)
	}

	if ct := w.Header().Get("Content-Type"); ct == "" {
		switch {
		case strings.HasSuffix(name, ".txt"):
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		case strings.HasSuffix(name, ".xml"):
			w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		default:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		}
	}
	w.WriteHeader(status)
	_, err := buf.WriteTo(w)
	return err
}

// staticFiles returns the embedded static assets rooted at static/.
func staticFiles() (fs.FS, error) { return fs.Sub(staticFS, "static") }

// PageData is the value every page template receives.
type PageData struct {
	Title       string
	Config      *config.Config
	User        *store.User
	Flashes     []session.Flash
	CSRFToken   string
	RequestPath string
	RequestURL  string
	// Data carries page-specific values.
	Data map[string]any
}

// IsAuthenticated reports whether a user is logged in, for template use.
func (p *PageData) IsAuthenticated() bool { return p.User != nil }

// Get reads a page-specific value, returning nil when absent so templates can
// test for optional values without erroring.
func (p *PageData) Get(key string) any {
	if p.Data == nil {
		return nil
	}
	return p.Data[key]
}

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"timeAgo":   timeAgo,
		"timeUntil": timeUntil,
		"formatUTC": func(t time.Time, layout string) string {
			if t.IsZero() {
				return ""
			}
			return t.UTC().Format(layout)
		},
		"add":      func(a, b int) int { return a + b },
		"sub":      func(a, b int) int { return a - b },
		"contains": strings.Contains,
		"lower":    strings.ToLower,
		"upper":    strings.ToUpper,
		"unixSeconds": func(t time.Time) int64 {
			if t.IsZero() {
				return 0
			}
			return t.Unix()
		},
		"dict": dict,
		"seq":  seq,
		// deref unwraps an optional timestamp for formatting; the caller has
		// already checked it is non-nil.
		"deref": func(t *time.Time) time.Time {
			if t == nil {
				return time.Time{}
			}
			return *t
		},
	}
}

// timeUntil renders a future timestamp as a coarse "in 3d" style label.
func timeUntil(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "Never"
	}
	d := time.Until(*t)
	if d <= 0 {
		return "Expired"
	}
	switch {
	case d >= 365*24*time.Hour:
		return fmt.Sprintf("in %dy", int(d.Hours())/(365*24))
	case d >= 30*24*time.Hour:
		return fmt.Sprintf("in %dmo", int(d.Hours())/(30*24))
	case d >= 24*time.Hour:
		return fmt.Sprintf("in %dd", int(d.Hours())/24)
	case d >= time.Hour:
		return fmt.Sprintf("in %dh", int(d.Hours()))
	case d >= time.Minute:
		return fmt.Sprintf("in %dm", int(d.Minutes()))
	default:
		return "Soon"
	}
}

// timeAgo renders a past timestamp as a coarse "3d ago" style label.
func timeAgo(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "Never"
	}
	d := time.Since(*t)
	switch {
	case d >= 365*24*time.Hour:
		return fmt.Sprintf("%dy ago", int(d.Hours())/(365*24))
	case d >= 30*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d.Hours())/(30*24))
	case d >= 24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours())/24)
	case d >= time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d >= time.Minute:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return "Just now"
	}
}

// dict builds a map inline, for passing several values to a sub-template.
func dict(pairs ...any) (map[string]any, error) {
	if len(pairs)%2 != 0 {
		return nil, fmt.Errorf("dict requires an even number of arguments")
	}
	m := make(map[string]any, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict keys must be strings")
		}
		m[key] = pairs[i+1]
	}
	return m, nil
}

// seq returns the integers from lo to hi inclusive, for pagination loops.
func seq(lo, hi int) []int {
	if hi < lo {
		return nil
	}
	out := make([]int, 0, hi-lo+1)
	for i := lo; i <= hi; i++ {
		out = append(out, i)
	}
	return out
}

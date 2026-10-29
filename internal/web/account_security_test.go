package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/arumes31/redrx/internal/config"
	"github.com/arumes31/redrx/internal/security"
	"github.com/arumes31/redrx/internal/store"
)

const legacyAPIKey = "11111111-2222-3333-4444-555555555555"

// getWithCookie fetches a page as the holder of a session cookie.
func getWithCookie(t *testing.T, srv *Server, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = "short.example.com"
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

// postForm submits a form as the holder of a session cookie.
func postForm(t *testing.T, srv *Server, path string, cookie *http.Cookie, form url.Values, remoteAddr string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Host = "short.example.com"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func apiGet(t *testing.T, srv *Server, path, key string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = "short.example.com"
	req.Header.Set("X-API-KEY", key)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec.Code
}

// sessionCookieOr returns the session cookie set on resp, or fallback when the
// response did not rewrite it.
func sessionCookieOr(resp *http.Response, fallback *http.Cookie) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == "redrx_session" {
			return c
		}
	}
	return fallback
}

func TestLogoutEverywhereRevokesOtherSessions(t *testing.T) {
	srv, _ := newTestServer(t)
	laptop := login(t, srv, "alice", "alice-password")
	phone := login(t, srv, "alice", "alice-password")

	page := getWithCookie(t, srv, "/settings/security", laptop)
	if page.Code != http.StatusOK {
		t.Fatalf("security settings = %d, want 200", page.Code)
	}
	token := extractCSRF(t, page.Body.String())
	laptop = sessionCookieOr(page.Result(), laptop)

	rec := postForm(t, srv, "/settings/logout-all", laptop, url.Values{"csrf_token": {token}}, "")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("logout-all = %d, want 303", rec.Code)
	}

	for name, cookie := range map[string]*http.Cookie{"phone": phone, "laptop": laptop} {
		if code := getWithCookie(t, srv, "/dashboard", cookie).Code; code != http.StatusSeeOther {
			t.Errorf("%s session still reaches the dashboard (%d) after logging out everywhere", name, code)
		}
	}

	// A fresh login still works.
	fresh := login(t, srv, "alice", "alice-password")
	if code := getWithCookie(t, srv, "/dashboard", fresh).Code; code != http.StatusOK {
		t.Errorf("a new login after logout-all reaches the dashboard with %d, want 200", code)
	}
}

func TestTOTPCodeCannotBeReplayed(t *testing.T) {
	srv, db := newTestServer(t)
	ctx := context.Background()
	user, err := db.UserByLogin(ctx, "alice")
	if err != nil {
		t.Fatalf("load fixture user: %v", err)
	}
	const secret = "JBSWY3DPEHPK3PXP"
	sealed, err := security.SealAccountSecret(srv.cfg.SecretKey, secret)
	if err != nil {
		t.Fatalf("encrypt TOTP secret: %v", err)
	}
	if err := db.SetTOTPPending(ctx, user.ID, user.SessionEpoch, sealed); err != nil {
		t.Fatalf("SetTOTPPending: %v", err)
	}
	if err := db.EnableTOTP(ctx, user.ID, sealed, 0, nil); err != nil {
		t.Fatalf("EnableTOTP: %v", err)
	}
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}

	attempt := func() *httptest.ResponseRecorder {
		page := get(t, srv, "/login")
		cookie := sessionCookie(t, page.Result())
		form := url.Values{"username": {"alice"}, "password": {"alice-password"},
			"csrf_token": {extractCSRF(t, page.Body.String())}}
		rec := postForm(t, srv, "/login", cookie, form, "")
		if loc := rec.Header().Get("Location"); loc != "/login/totp" {
			t.Fatalf("password step redirected to %q, want /login/totp", loc)
		}
		pending := sessionCookie(t, rec.Result())
		page = getWithCookie(t, srv, "/login/totp", pending)
		form = url.Values{"code": {code}, "csrf_token": {extractCSRF(t, page.Body.String())}}
		return postForm(t, srv, "/login/totp", sessionCookieOr(page.Result(), pending), form, "")
	}

	if rec := attempt(); rec.Code != http.StatusSeeOther {
		t.Fatalf("first use of a valid TOTP code = %d, want 303", rec.Code)
	}
	if rec := attempt(); rec.Code == http.StatusSeeOther {
		t.Error("the same TOTP code completed a second login")
	}
}

func TestLoginIsLimitedPerAccount(t *testing.T) {
	srv, _ := newTestServer(t, func(c *config.Config) {
		c.RateLimitLogin = "1000 per hour"
		c.RateLimitLoginAccount = "2 per hour"
	})

	page := get(t, srv, "/login")
	cookie := sessionCookie(t, page.Result())
	token := extractCSRF(t, page.Body.String())

	// Each attempt comes from a different address, so only the per-account
	// budget can stop the third.
	var codes []int
	for _, ip := range []string{"192.0.2.1:1000", "192.0.2.2:1000", "192.0.2.3:1000"} {
		form := url.Values{"username": {"alice"}, "password": {"wrong"}, "csrf_token": {token}}
		codes = append(codes, postForm(t, srv, "/login", cookie, form, ip).Code)
	}
	if codes[0] != http.StatusOK || codes[1] != http.StatusOK || codes[2] != http.StatusTooManyRequests {
		t.Errorf("login attempts from three IPs = %v, want [200 200 429]", codes)
	}

	// Another account is unaffected.
	form := url.Values{"username": {"bob@example.com"}, "password": {"wrong"}, "csrf_token": {token}}
	if code := postForm(t, srv, "/login", cookie, form, "192.0.2.4:1000").Code; code != http.StatusOK {
		t.Errorf("a different account was throttled (%d)", code)
	}
}

func TestAPIKeysAreStoredHashedAndShownOnce(t *testing.T) {
	srv, db := newTestServer(t)
	ctx := context.Background()

	user, err := db.UserByLogin(ctx, "alice")
	if err != nil {
		t.Fatalf("load fixture user: %v", err)
	}
	if user.APIKey != store.HashAPIKey(legacyAPIKey) {
		t.Fatalf("legacy API key was not hashed by the migration: stored %q", user.APIKey)
	}

	cookie := login(t, srv, "alice", "alice-password")
	page := getWithCookie(t, srv, "/dashboard", cookie)
	cookie = sessionCookieOr(page.Result(), cookie)
	rec := postForm(t, srv, "/regenerate-api-key", cookie,
		url.Values{"csrf_token": {extractCSRF(t, page.Body.String())}}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("regenerate = %d, want 200 with the new key", rec.Code)
	}
	body := rec.Body.String()
	const marker = `id="apiKeyValue"`
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatal("the regenerated key was not displayed")
	}
	_, rest, _ := strings.Cut(body[i:], `value="`)
	newKey, _, _ := strings.Cut(rest, `"`)
	if len(newKey) != len(legacyAPIKey) {
		t.Fatalf("displayed key %q does not look like a key", newKey)
	}

	if code := apiGet(t, srv, "/api/v1/ABC123", newKey); code != http.StatusOK {
		t.Errorf("the new key was rejected (%d)", code)
	}
	if code := apiGet(t, srv, "/api/v1/ABC123", legacyAPIKey); code != http.StatusUnauthorized {
		t.Errorf("the replaced key still works (%d)", code)
	}
	if code := apiGet(t, srv, "/api/v1/ABC123", store.HashAPIKey(newKey)); code != http.StatusUnauthorized {
		t.Errorf("the stored hash works as a key (%d)", code)
	}
	later := getWithCookie(t, srv, "/dashboard", sessionCookieOr(rec.Result(), cookie))
	if strings.Contains(later.Body.String(), newKey) {
		t.Error("the key was displayed again on a later dashboard view")
	}
}

func TestMetricsTokenIsEnforcedWhenSet(t *testing.T) {
	srv, _ := newTestServer(t, func(c *config.Config) { c.MetricsToken = "scrape-token" })

	for header, want := range map[string]int{
		"":                    http.StatusUnauthorized,
		"Bearer wrong":        http.StatusUnauthorized,
		"Bearer scrape-token": http.StatusOK,
	} {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("GET /metrics with Authorization %q = %d, want %d", header, rec.Code, want)
		}
	}
}

// TestEnablingTOTPKeepsCurrentSessionOnly checks that the session epoch bump
// on a 2FA change revokes other sessions but not the one that made the change.
func TestEnablingTOTPKeepsCurrentSessionOnly(t *testing.T) {
	srv, _ := newTestServer(t)
	current := login(t, srv, "alice", "alice-password")
	other := login(t, srv, "alice", "alice-password")

	page := getWithCookie(t, srv, "/settings/security", current)
	current = sessionCookieOr(page.Result(), current)
	rec := postForm(t, srv, "/settings/totp/start", current,
		url.Values{"csrf_token": {extractCSRF(t, page.Body.String())}}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("start TOTP = %d, want 200", rec.Code)
	}
	current = sessionCookieOr(rec.Result(), current)
	body := rec.Body.String()
	const marker = `user-select-all">`
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatal("no manual setup code on the enrollment page")
	}
	secret, _, _ := strings.Cut(body[i+len(marker):], "<")
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}

	rec = postForm(t, srv, "/settings/totp/confirm", current,
		url.Values{"code": {code}, "csrf_token": {extractCSRF(t, body)}}, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "recoveryCodes") {
		t.Fatalf("confirm TOTP = %d, want 200 with recovery codes", rec.Code)
	}
	current = sessionCookieOr(rec.Result(), current)

	if code := getWithCookie(t, srv, "/dashboard", current).Code; code != http.StatusOK {
		t.Errorf("the session that enabled 2FA was logged out (%d)", code)
	}
	if code := getWithCookie(t, srv, "/dashboard", other).Code; code != http.StatusSeeOther {
		t.Errorf("another session survived enabling 2FA (%d)", code)
	}
}

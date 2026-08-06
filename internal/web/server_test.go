package web

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/arumes31/redrx/internal/config"
	"github.com/arumes31/redrx/internal/geo"
	"github.com/arumes31/redrx/internal/ratelimit"
	"github.com/arumes31/redrx/internal/safety"
	"github.com/arumes31/redrx/internal/security"
	"github.com/arumes31/redrx/internal/store"
)

// newTestServer builds a server backed by a copy of the legacy Python database,
// so the handlers are exercised against real pre-existing rows.
func newTestServer(t *testing.T, tweaks ...func(*config.Config)) (*Server, *store.DB) {
	t.Helper()

	src, err := os.ReadFile(filepath.Join("..", "store", "testdata", "legacy_python.db"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "web.db")
	if err := os.WriteFile(path, src, 0o600); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}

	ctx := context.Background()
	db, err := store.Open(ctx, "sqlite:///"+filepath.ToSlash(path))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cfg := &config.Config{
		Debug:               true,
		SecretKey:           []byte("test-secret-key"),
		BaseDomain:          "short.example.com",
		ExpiryHours:         24,
		ShortCodeLength:     6,
		DefaultQRColor:      "black",
		DefaultQRBG:         "white",
		MaxUploadSize:       1 << 20,
		EnableSEO:           true,
		SEODomain:           "redrx.eu",
		RateLimitStorageURI: "memory://",
	}
	for _, tweak := range tweaks {
		tweak(cfg)
	}

	backend := ratelimit.NewMemoryBackend()
	t.Cleanup(func() { backend.Close() })

	srv, err := NewServer(Options{
		Config:  cfg,
		DB:      db,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Limiter: ratelimit.New(backend),
		// Phishing checking off: the fixture has no blocklist file, and with it
		// enabled the checker would correctly fail closed.
		Safety: safety.New(safety.Options{Enabled: false}),
		Geo:    geo.New(geo.Options{}),
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv, db
}

// TestAllTemplatesParse is the guard for template conversion mistakes, which
// otherwise only surface when a specific page is first requested.
func TestAllTemplatesParse(t *testing.T) {
	r, err := newRenderer()
	if err != nil {
		t.Fatalf("templates do not parse: %v", err)
	}
	for _, name := range append(append([]string{}, pages...), standalone...) {
		if _, ok := r.templates[name]; !ok {
			t.Errorf("template %q was not registered", name)
		}
	}
}

func get(t *testing.T, srv *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = "short.example.com"
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

// TestPublicPagesRender walks every page a logged-out visitor can reach. A
// template that references a missing field fails here rather than in production.
func TestPublicPagesRender(t *testing.T) {
	srv, _ := newTestServer(t)

	cases := []struct {
		path string
		want int
	}{
		{"/", http.StatusOK},
		{"/login", http.StatusOK},
		{"/register", http.StatusOK},
		{"/api-docs", http.StatusOK},
		{"/data-usage", http.StatusOK},
		{"/terms", http.StatusOK},
		{"/robots.txt", http.StatusOK},
		{"/sitemap.xml", http.StatusOK},
		{"/health", http.StatusOK},
		{"/metrics", http.StatusOK},
		{"/static/css/style.css", http.StatusOK},
		{"/NOSUCHCODE", http.StatusNotFound},
		{"/dashboard", http.StatusSeeOther}, // redirected to login
	}

	for _, tc := range cases {
		rec := get(t, srv, tc.path)
		if rec.Code != tc.want {
			t.Errorf("GET %s = %d, want %d\n%s", tc.path, rec.Code, tc.want,
				truncateBody(rec.Body.String()))
		}
	}
}

func TestRedirectServesLegacyLink(t *testing.T) {
	srv, _ := newTestServer(t)

	// ABC123 is password protected in the fixture, so it should gate first.
	rec := get(t, srv, "/ABC123")
	if rec.Code != http.StatusSeeOther {
		t.Errorf("password-protected link returned %d, want a redirect to the gate", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/link-auth/ABC123" {
		t.Errorf("Location = %q, want /link-auth/ABC123", loc)
	}

	// ANON01 is disabled in the fixture.
	if rec := get(t, srv, "/ANON01"); rec.Code != http.StatusGone {
		t.Errorf("disabled link returned %d, want 410", rec.Code)
	}

	// NOEXPIRE is a plain, active, preview-enabled link.
	rec = get(t, srv, "/NOEXPIRE")
	if rec.Code != http.StatusOK {
		t.Fatalf("active link returned %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "https://forever.example.net/") {
		t.Error("preview page does not show the destination URL")
	}
}

func TestStatsAccessControl(t *testing.T) {
	srv, _ := newTestServer(t)

	// ABC123 belongs to alice, so an anonymous visitor must be refused.
	if rec := get(t, srv, "/ABC123/stats"); rec.Code != http.StatusForbidden {
		t.Errorf("anonymous access to an owned link's stats = %d, want 403", rec.Code)
	}

	// ANON01 has no owner, so its stats are public.
	rec := get(t, srv, "/ANON01/stats")
	if rec.Code != http.StatusOK {
		t.Errorf("public stats returned %d, want 200\n%s", rec.Code, truncateBody(rec.Body.String()))
	}
	if !strings.Contains(rec.Body.String(), "public link") {
		t.Error("public stats page is missing its public-link notice")
	}
}

func TestQRCodeIsAPNG(t *testing.T) {
	srv, _ := newTestServer(t)

	rec := get(t, srv, "/NOEXPIRE/qr")
	if rec.Code != http.StatusOK {
		t.Fatalf("QR endpoint returned %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
	if body := rec.Body.Bytes(); len(body) < 8 || string(body[1:4]) != "PNG" {
		t.Error("response body is not a PNG image")
	}
}

func TestDraftCreatedByAPINeverResolvesUntilPublished(t *testing.T) {
	srv, db := newTestServer(t)
	user, err := db.UserByLogin(context.Background(), "alice")
	if err != nil {
		t.Fatalf("load fixture user: %v", err)
	}
	body := `{"long_url":"https://draft.example.com/","custom_code":"DRAFT1","draft":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/shorten", strings.NewReader(body))
	req.Host = "short.example.com"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", user.APIKey)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create draft = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	link, err := db.URLByShortCode(context.Background(), "DRAFT1")
	if err != nil {
		t.Fatalf("load draft: %v", err)
	}
	if !link.IsDraft || link.IsActive() {
		t.Fatalf("draft state = draft:%v active:%v", link.IsDraft, link.IsActive())
	}
	if rec := get(t, srv, "/DRAFT1"); rec.Code != http.StatusGone {
		t.Errorf("draft redirect = %d, want 410", rec.Code)
	}
}

func TestPrivacySignalsSuppressClickAnalytics(t *testing.T) {
	srv, db := newTestServer(t, func(c *config.Config) {
		c.HonorDoNotTrack = true
		c.EnableConsentBanner = true
	})
	link := &store.URL{
		ShortCode: "TRACK1", LongURL: "https://tracking.example.com/",
		PreviewMode: true, StatsEnabled: true, IsEnabled: true,
	}
	if err := db.CreateURL(context.Background(), link); err != nil {
		t.Fatalf("CreateURL: %v", err)
	}

	request := func(dnt string, consent bool) {
		req := httptest.NewRequest(http.MethodGet, "/TRACK1", nil)
		req.Host = "short.example.com"
		if dnt != "" {
			req.Header.Set("DNT", dnt)
		}
		if consent {
			req.AddCookie(&http.Cookie{Name: consentCookieName, Value: "accepted"})
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("redirect page = %d, want 200", rec.Code)
		}
	}

	request("", false)
	request("1", true)
	stored, err := db.URLByShortCode(context.Background(), "TRACK1")
	if err != nil {
		t.Fatalf("load tracking link: %v", err)
	}
	if stored.ClicksCount != 0 {
		t.Errorf("privacy-suppressed requests recorded %d clicks, want 0", stored.ClicksCount)
	}
	request("", true)
	stored, err = db.URLByShortCode(context.Background(), "TRACK1")
	if err != nil {
		t.Fatalf("reload tracking link: %v", err)
	}
	if stored.ClicksCount != 1 {
		t.Errorf("consented request recorded %d clicks, want 1", stored.ClicksCount)
	}
}

func TestDataUsageMentionsDoNotTrackOnlyWhenEnabled(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(strconv.FormatBool(enabled), func(t *testing.T) {
			srv, _ := newTestServer(t, func(c *config.Config) {
				c.HonorDoNotTrack = enabled
			})
			body := get(t, srv, "/data-usage").Body.String()
			if !strings.Contains(body, "analytics are recorded only after you allow them") {
				t.Fatal("consent wording is missing")
			}
			if got := strings.Contains(body, "Redrx also honors the browser"); got != enabled {
				t.Errorf("DNT statement present = %v, want %v", got, enabled)
			}
		})
	}
}

func TestAnonymousPoWChallengeCannotBeReplayed(t *testing.T) {
	srv, db := newTestServer(t, func(c *config.Config) {
		c.AnonymousPoWDifficulty = 1
	})

	page := get(t, srv, "/")
	originalCookie := sessionCookie(t, page.Result())
	csrf := extractCSRF(t, page.Body.String())
	challenge := extractHiddenValue(t, page.Body.String(), "pow_challenge")
	solution := solveTestPoW(t, challenge)

	create := func(code string) *httptest.ResponseRecorder {
		form := url.Values{
			"long_url":      {"https://pow.example/" + strings.ToLower(code)},
			"custom_code":   {code},
			"csrf_token":    {csrf},
			"pow_challenge": {challenge},
			"pow_solution":  {solution},
		}
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
		req.Host = "short.example.com"
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(originalCookie)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec
	}

	first := create("POWFIRST")
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), "/POWFIRST") {
		t.Fatalf("first create = %d, want success: %s", first.Code, truncateBody(first.Body.String()))
	}
	replayed := create("POWREPLAY")
	if replayed.Code != http.StatusOK ||
		!strings.Contains(replayed.Body.String(), "Browser verification expired or failed") {
		t.Fatalf("replayed create was not rejected: %d %s", replayed.Code, truncateBody(replayed.Body.String()))
	}
	if _, err := db.URLByShortCode(context.Background(), "POWFIRST"); err != nil {
		t.Fatalf("first link was not stored: %v", err)
	}
	if _, err := db.URLByShortCode(context.Background(), "POWREPLAY"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("replayed link lookup = %v, want ErrNotFound", err)
	}

	var storedHash string
	if err := db.QueryRowContext(context.Background(),
		"SELECT challenge_hash FROM pow_challenge_claims").Scan(&storedHash); err != nil {
		t.Fatalf("load proof-of-work claim: %v", err)
	}
	if storedHash == challenge || len(storedHash) != sha256.Size*2 {
		t.Errorf("stored claim is not a SHA-256 digest: %q", storedHash)
	}
}

func TestValidateSecondFactorRejectsDisabledAccountWithoutConsumingRecoveryCode(t *testing.T) {
	srv, db := newTestServer(t)
	ctx := context.Background()
	user, err := db.UserByLogin(ctx, "alice")
	if err != nil {
		t.Fatalf("load fixture user: %v", err)
	}
	user.TOTPSecret = ""
	user.TOTPEnabled = false
	const recovery = "DISA-BLED-CODE"
	hash := security.RecoveryCodeHash(srv.cfg.SecretKey, recovery)
	if _, err := db.Exec(ctx,
		"INSERT INTO recovery_codes (user_id, code_hash) VALUES (?, ?)", user.ID, hash); err != nil {
		t.Fatalf("insert recovery code: %v", err)
	}

	valid, err := srv.validateSecondFactor(httptest.NewRequest(http.MethodPost, "/login/totp", nil), user, recovery)
	if err != nil || valid {
		t.Fatalf("validateSecondFactor = (%v, %v), want (false, nil)", valid, err)
	}
	unused, err := db.ConsumeRecoveryCode(ctx, user.ID, hash)
	if err != nil {
		t.Fatalf("consume untouched recovery code: %v", err)
	}
	if !unused {
		t.Error("disabled-account validation consumed the recovery code")
	}
}

func TestRecoveryCodeCopyCapturesButtonBeforeAwait(t *testing.T) {
	source, err := templateFS.ReadFile("templates/security_settings.html")
	if err != nil {
		t.Fatalf("read security settings template: %v", err)
	}
	js := string(source)
	capture := strings.Index(js, "const button = event.currentTarget;")
	await := strings.Index(js, "await navigator.clipboard.writeText(codes)")
	if capture < 0 || await < 0 || capture > await {
		t.Fatal("copy handler does not capture event.currentTarget before awaiting the clipboard")
	}
	if strings.Contains(js[await:], "event.currentTarget.textContent") {
		t.Error("copy handler reads event.currentTarget after the await")
	}
}

func TestRecoveryCodeCompletesTwoFactorLoginOnce(t *testing.T) {
	srv, db := newTestServer(t)
	ctx := context.Background()
	user, err := db.UserByLogin(ctx, "alice")
	if err != nil {
		t.Fatalf("load fixture user: %v", err)
	}
	secret, err := security.SealAccountSecret(srv.cfg.SecretKey, "JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatalf("encrypt TOTP secret: %v", err)
	}
	if err := db.SetTOTPPending(ctx, user.ID, secret); err != nil {
		t.Fatalf("SetTOTPPending: %v", err)
	}
	const recovery = "ABCD-EFGH-JKLM"
	hash := security.RecoveryCodeHash(srv.cfg.SecretKey, recovery)
	if err := db.EnableTOTP(ctx, user.ID, []string{hash}); err != nil {
		t.Fatalf("EnableTOTP: %v", err)
	}

	loginPage := get(t, srv, "/login")
	cookie := sessionCookie(t, loginPage.Result())
	token := extractCSRF(t, loginPage.Body.String())
	form := url.Values{"username": {"alice"}, "password": {"alice-password"}, "csrf_token": {token}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Host = "short.example.com"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if loc := rec.Header().Get("Location"); rec.Code != http.StatusSeeOther || loc != "/login/totp" {
		t.Fatalf("password step = %d Location %q, want 303 /login/totp", rec.Code, loc)
	}
	pendingCookie := sessionCookie(t, rec.Result())

	req = httptest.NewRequest(http.MethodGet, "/login/totp", nil)
	req.Host = "short.example.com"
	req.AddCookie(pendingCookie)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	token = extractCSRF(t, rec.Body.String())

	form = url.Values{"code": {recovery}, "csrf_token": {token}}
	req = httptest.NewRequest(http.MethodPost, "/login/totp", strings.NewReader(form.Encode()))
	req.Host = "short.example.com"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(pendingCookie)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("2FA step = %d Location %q, want 303 /", rec.Code, rec.Header().Get("Location"))
	}
	used, err := db.ConsumeRecoveryCode(ctx, user.ID, hash)
	if err != nil {
		t.Fatalf("check consumed recovery code: %v", err)
	}
	if used {
		t.Error("recovery code could be consumed a second time")
	}
}

// login performs a full login round-trip and returns the session cookie.
func login(t *testing.T, srv *Server, username, password string) *http.Cookie {
	t.Helper()

	// Fetch the login page first to obtain a CSRF token and its session.
	rec := get(t, srv, "/login")
	cookie := sessionCookie(t, rec.Result())
	token := extractCSRF(t, rec.Body.String())

	form := url.Values{"username": {username}, "password": {password}, "csrf_token": {token}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "short.example.com"
	req.AddCookie(cookie)

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("login returned %d, want 303\n%s", rec.Code, truncateBody(rec.Body.String()))
	}
	return sessionCookie(t, rec.Result())
}

// TestLegacyUserCanLogInAndSeeDashboard is the end-to-end proof that an account
// created by the Python application still works.
func TestLegacyUserCanLogInAndSeeDashboard(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := login(t, srv, "alice", "alice-password")

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.Host = "short.example.com"
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard returned %d, want 200\n%s", rec.Code, truncateBody(rec.Body.String()))
	}
	body := rec.Body.String()
	for _, want := range []string{"ABC123", "My Dashboard", "11111111-2222-3333-4444-555555555555"} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard is missing %q", want)
		}
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	srv, _ := newTestServer(t)

	rec := get(t, srv, "/login")
	cookie := sessionCookie(t, rec.Result())
	token := extractCSRF(t, rec.Body.String())

	form := url.Values{"username": {"alice"}, "password": {"wrong"}, "csrf_token": {token}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "short.example.com"
	req.AddCookie(cookie)

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code == http.StatusSeeOther {
		t.Fatal("login with a wrong password was accepted")
	}
	if !strings.Contains(rec.Body.String(), "Login Unsuccessful") {
		t.Error("the failed-login message was not shown")
	}
}

func TestPostWithoutCSRFTokenIsRejected(t *testing.T) {
	srv, _ := newTestServer(t)

	form := url.Values{"long_url": {"https://example.com/"}}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "short.example.com"

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("POST without a CSRF token = %d, want 403", rec.Code)
	}
}

func TestCreateLinkThroughTheForm(t *testing.T) {
	srv, db := newTestServer(t)

	rec := get(t, srv, "/")
	cookie := sessionCookie(t, rec.Result())
	token := extractCSRF(t, rec.Body.String())

	form := url.Values{
		"long_url":     {"https://created-via-form.example/page"},
		"custom_code":  {"FORMTEST"},
		"expiry_hours": {"48"},
		"csrf_token":   {token},
		"preview_mode": {"y"},
	}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = "short.example.com"
	req.AddCookie(cookie)

	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("create returned %d, want 200\n%s", rec.Code, truncateBody(rec.Body.String()))
	}
	if !strings.Contains(rec.Body.String(), "https://short.example.com/FORMTEST") {
		t.Error("the created short URL was not shown")
	}

	link, err := db.URLByShortCode(context.Background(), "FORMTEST")
	if err != nil {
		t.Fatalf("created link not found in the database: %v", err)
	}
	if link.LongURL != "https://created-via-form.example/page" {
		t.Errorf("LongURL = %q", link.LongURL)
	}
	if link.ExpiresAt == nil {
		t.Error("expiry was not applied")
	}
}

func TestAPIShortenAndFetch(t *testing.T) {
	srv, _ := newTestServer(t)
	const key = "11111111-2222-3333-4444-555555555555"

	body := strings.NewReader(`{"long_url":"https://api.example.com/target","custom_code":"APITEST","expiry_hours":0}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/shorten", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", key)
	req.Host = "short.example.com"

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/shorten = %d, want 201\n%s", rec.Code, rec.Body.String())
	}

	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created["short_code"] != "APITEST" {
		t.Errorf("short_code = %v, want APITEST", created["short_code"])
	}
	if created["short_url"] != "https://short.example.com/APITEST" {
		t.Errorf("short_url = %v", created["short_url"])
	}
	if created["expires_at"] != nil {
		t.Errorf("expires_at = %v, want null for expiry_hours 0", created["expires_at"])
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/APITEST", nil)
	req.Header.Set("X-API-KEY", key)
	req.Host = "short.example.com"
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/APITEST = %d, want 200\n%s", rec.Code, rec.Body.String())
	}
	var fetched map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if fetched["long_url"] != "https://api.example.com/target" {
		t.Errorf("long_url = %v", fetched["long_url"])
	}
	if fetched["active"] != true {
		t.Errorf("active = %v, want true", fetched["active"])
	}
}

// TestAPITimestampFormatsMatchPython pins the two shapes the previous API
// emitted: create returns timezone-aware strings, reads return naive ones.
// Integrations parse these directly, so the asymmetry is preserved.
func TestAPITimestampFormatsMatchPython(t *testing.T) {
	srv, _ := newTestServer(t)
	const key = "11111111-2222-3333-4444-555555555555"

	body := strings.NewReader(`{"long_url":"https://ts.example/","custom_code":"TSTEST","expiry_hours":24}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/shorten", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", key)
	req.Host = "short.example.com"
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	expires, _ := created["expires_at"].(string)
	if !strings.HasSuffix(expires, "+00:00") {
		t.Errorf("create expires_at = %q, want a timezone-aware value ending in +00:00", expires)
	}
	if _, err := time.Parse("2006-01-02T15:04:05.000000-07:00", expires); err != nil {
		if _, err2 := time.Parse("2006-01-02T15:04:05-07:00", expires); err2 != nil {
			t.Errorf("create expires_at %q is not in Python's isoformat shape", expires)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/TSTEST", nil)
	req.Header.Set("X-API-KEY", key)
	req.Host = "short.example.com"
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	var fetched map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("decode read response: %v", err)
	}
	for _, field := range []string{"created_at", "expires_at"} {
		v, _ := fetched[field].(string)
		if strings.HasSuffix(v, "+00:00") || strings.HasSuffix(v, "Z") {
			t.Errorf("read %s = %q, want a naive value with no offset", field, v)
		}
		if _, err := time.Parse("2006-01-02T15:04:05.000000", v); err != nil {
			if _, err2 := time.Parse("2006-01-02T15:04:05", v); err2 != nil {
				t.Errorf("read %s = %q is not in Python's isoformat shape", field, v)
			}
		}
	}
}

func TestAPIRequiresValidKey(t *testing.T) {
	srv, _ := newTestServer(t)

	for _, key := range []string{"", "not-a-real-key"} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/shorten",
			strings.NewReader(`{"long_url":"https://example.com/"}`))
		req.Header.Set("Content-Type", "application/json")
		if key != "" {
			req.Header.Set("X-API-KEY", key)
		}
		req.Host = "short.example.com"

		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("API key %q = %d, want 401", key, rec.Code)
		}
	}
}

func TestAPIReadsLegacyLink(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ABC123", nil)
	req.Header.Set("X-API-KEY", "11111111-2222-3333-4444-555555555555")
	req.Host = "short.example.com"
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/ABC123 = %d, want 200\n%s", rec.Code, rec.Body.String())
	}

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["clicks"] != float64(42) {
		t.Errorf("clicks = %v, want 42 from the legacy row", got["clicks"])
	}
	targets, ok := got["rotate_targets"].([]any)
	if !ok || len(targets) != 2 {
		t.Errorf("rotate_targets = %v, want the two legacy targets", got["rotate_targets"])
	}
}

func TestSecurityHeadersArePresent(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := get(t, srv, "/")

	for header, want := range map[string]string{
		"X-Frame-Options":        "SAMEORIGIN",
		"X-Content-Type-Options": "nosniff",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if rec.Header().Get("Content-Security-Policy") == "" {
		t.Error("Content-Security-Policy header is missing")
	}
}

// TestRateLimitReturns429 also covers the RATELIMIT_* config plumbing, since
// the limit under test is the one parsed from configuration.
func TestRateLimitReturns429(t *testing.T) {
	srv, _ := newTestServer(t, func(c *config.Config) {
		c.RateLimitLogin = "2 per hour"
	})

	// POST /login carries the login limit; the GET form is on the shared pages
	// budget, so viewing the login page cannot lock a shared-IP visitor out.
	// The limiter runs ahead of CSRF, so a bare POST still consumes the budget.
	var last int
	for i := 0; i < 4; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=x&password=y"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		last = rec.Code
	}
	if last != http.StatusTooManyRequests {
		t.Errorf("after exceeding the limit the response was %d, want 429", last)
	}

	// A different route keeps its own budget.
	if code := get(t, srv, "/terms").Code; code != http.StatusOK {
		t.Errorf("an unrelated route returned %d, want 200", code)
	}
}

func sessionCookie(t *testing.T, resp *http.Response) *http.Cookie {
	t.Helper()
	for _, c := range resp.Cookies() {
		if c.Name == "redrx_session" {
			return c
		}
	}
	t.Fatal("no session cookie was set")
	return nil
}

// extractCSRF pulls the hidden token out of a rendered form.
func extractCSRF(t *testing.T, body string) string {
	t.Helper()
	const marker = `name="csrf_token" value="`
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatal("no csrf_token field in the rendered page")
	}
	rest := body[i+len(marker):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		t.Fatal("malformed csrf_token field")
	}
	return rest[:end]
}

func extractHiddenValue(t *testing.T, body, name string) string {
	t.Helper()
	marker := `name="` + name + `" value="`
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("no %s field in rendered page", name)
	}
	rest := body[i+len(marker):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		t.Fatalf("malformed %s field", name)
	}
	return rest[:end]
}

func solveTestPoW(t *testing.T, challenge string) string {
	t.Helper()
	for n := uint64(0); n < 1<<20; n++ {
		solution := strconv.FormatUint(n, 10)
		sum := sha256.Sum256([]byte(challenge + ":" + solution))
		if sum[0]&0x80 == 0 {
			return solution
		}
	}
	t.Fatal("failed to solve low-difficulty proof of work")
	return ""
}

func truncateBody(s string) string {
	if len(s) > 1500 {
		return s[:1500] + "\n...[truncated]"
	}
	return s
}

// TestTimeBucketsAreUnique guards the 24h range against emitting the current
// hour twice: the labels are hour-of-day only, so a duplicate would collapse
// onto one bucket and count its clicks twice in the total and daily average.
func TestTimeBucketsAreUnique(t *testing.T) {
	now := time.Date(2026, 5, 4, 15, 37, 0, 0, time.UTC)

	for _, tc := range []struct {
		rangeType string
		want      int
	}{
		{"24h", 24},
		{"7d", 8},
		{"30d", 31},
	} {
		labels, _, _ := timeBuckets(tc.rangeType, now)
		if len(labels) != tc.want {
			t.Errorf("%s produced %d labels, want %d", tc.rangeType, len(labels), tc.want)
		}
		seen := map[string]bool{}
		for _, l := range labels {
			if seen[l] {
				t.Errorf("%s repeats the label %q", tc.rangeType, l)
			}
			seen[l] = true
		}
	}
}

// TestTimeBucketsCutoffAlignsWithFirstLabel is the assertion that actually
// catches the double count. Unique labels are not enough: the labels can be
// distinct while the cutoff still admits an extra partial hour whose
// hour-of-day collides with the current hour, so that one bucket carries two
// hours of clicks and inflates both the total and the daily average.
func TestTimeBucketsCutoffAlignsWithFirstLabel(t *testing.T) {
	now := time.Date(2026, 5, 4, 15, 37, 0, 0, time.UTC)
	labels, cutoff, hourly := timeBuckets("24h", now)

	if !hourly {
		t.Fatal("24h range should use hourly buckets")
	}
	if got := cutoff.Format("15:00"); got != labels[0] {
		t.Errorf("cutoff is at %s but the first label is %s — clicks before the "+
			"first labelled hour land in a later bucket", got, labels[0])
	}
	if cutoff.Minute() != 0 || cutoff.Second() != 0 || cutoff.Nanosecond() != 0 {
		t.Errorf("cutoff %v is mid-hour; it must be the start of an hour", cutoff)
	}
	// The window must cover exactly the 24 labelled hours.
	if span := now.Sub(cutoff); span <= 23*time.Hour || span > 24*time.Hour {
		t.Errorf("window is %v, want more than 23h and at most 24h", span)
	}
}

// TestNormalizeHexColorExpandsShorthand checks that a valid "#rgb" is expanded
// to "#rrggbb": the <input type="color"> that renders the value accepts only
// the six-digit form and coerces "#f00" to black otherwise.
func TestNormalizeHexColorExpandsShorthand(t *testing.T) {
	cases := map[string]string{
		"#f00":    "#ff0000",
		"#ABC":    "#aabbcc",
		"#00ff00": "#00ff00",
		"red":     "#ff0000",
		"":        "#000000",
		"#12345":  "#000000", // invalid length falls back to the default
		"#gggggg": "#000000", // non-hex falls back to the default
	}
	for in, want := range cases {
		if got := normalizeHexColor(in, "#000000"); got != want {
			t.Errorf("normalizeHexColor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTruncateKeepsRunesIntact(t *testing.T) {
	// "€" is three bytes, so a byte-wise cut at 4 would split the second one.
	if got := truncate("a€€", 4); !utf8.ValidString(got) {
		t.Errorf("truncate produced invalid UTF-8: %q", got)
	}
	if got := truncate("a€€", 4); got != "a€" {
		t.Errorf("truncate = %q, want %q", got, "a€")
	}
	if got := truncate("short", 255); got != "short" {
		t.Errorf("a string within the limit was altered: %q", got)
	}
	if got := truncate("€€€", 2); got != "" {
		t.Errorf("truncate = %q, want empty when no whole rune fits", got)
	}
}

// TestShortCodeLookupIsCaseInsensitive checks that every short-code route
// normalises the same way. Codes are stored uppercase, so a lowercase link had
// to resolve on the redirect just as it already did on /stats and /qr.
func TestShortCodeLookupIsCaseInsensitive(t *testing.T) {
	srv, _ := newTestServer(t)

	for _, path := range []string{"/abc123", "/abc123/stats", "/abc123/qr"} {
		rec := get(t, srv, path)
		if rec.Code == http.StatusNotFound {
			t.Errorf("GET %s = 404; the lowercase form of an existing code did not resolve", path)
		}
	}
}

// TestAPIRejectsDuplicateCustomCodeWithConflict pins the 409 the API returns
// when a custom code is taken, including when the loss happens at insert time
// rather than at the preliminary check.
func TestAPIRejectsDuplicateCustomCodeWithConflict(t *testing.T) {
	srv, _ := newTestServer(t)

	body := `{"long_url":"https://example.com/","custom_code":"ABC123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/shorten", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "11111111-2222-3333-4444-555555555555")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409; body: %s", rec.Code, truncateBody(rec.Body.String()))
	}
}

// TestScheduledLinkNotBornExpired covers a future start_at with the default
// expiry: the window must count from the start, not from creation, or the link
// expires before it ever goes live and 410s on the first visit.
func TestScheduledLinkNotBornExpired(t *testing.T) {
	srv, db := newTestServer(t)
	key := "11111111-2222-3333-4444-555555555555" // alice, from the fixture

	start := time.Now().UTC().Add(7 * 24 * time.Hour)
	body := `{"long_url":"https://example.com/future","custom_code":"FUTURE1","start_at":"` +
		start.Format(time.RFC3339) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/shorten", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", key)
	req.Host = "short.example.com"
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create scheduled link: %d, body %s", rec.Code, truncateBody(rec.Body.String()))
	}

	// A not-yet-started link is correctly inactive now; the bug is an expiry
	// that precedes the start, which is what makes it permanently dead. Read the
	// row back and compare the two timestamps directly.
	link, err := db.URLByShortCode(context.Background(), "FUTURE1")
	if err != nil {
		t.Fatalf("load created link: %v", err)
	}
	if link.ExpiresAt == nil {
		return // permanent is fine; it certainly is not born expired
	}
	if link.StartAt == nil {
		t.Fatal("start_at was not stored")
	}
	if !link.ExpiresAt.After(*link.StartAt) {
		t.Errorf("expires_at %v is not after start_at %v — the link expires before it starts",
			link.ExpiresAt, link.StartAt)
	}
	// And it must become active once its window opens: expiry is well beyond the
	// 7-day start, so a link whose start had already passed would be active.
	if link.ExpiresAt.Sub(*link.StartAt) < 12*time.Hour {
		t.Errorf("expiry window is only %v after start; the default should apply from the start",
			link.ExpiresAt.Sub(*link.StartAt))
	}
}

// TestRateLimitScopesAreIndependent guards the separation: exhausting one
// route's limit must not spend another route's budget. Before the split, the
// content pages shared a single "page" counter.
func TestRateLimitScopesAreIndependent(t *testing.T) {
	srv, _ := newTestServer(t, func(c *config.Config) {
		c.RateLimitDefault = "1000 per hour" // keep the home page out of the way
	})

	// Hammer /terms until it 429s.
	var termsCode int
	for i := 0; i < 40; i++ {
		termsCode = get(t, srv, "/terms").Code
		if termsCode == http.StatusTooManyRequests {
			break
		}
	}
	if termsCode != http.StatusTooManyRequests {
		t.Fatalf("/terms never hit its own limit (last %d)", termsCode)
	}

	// A different content page must still be served — it has its own counter.
	if code := get(t, srv, "/api-docs").Code; code != http.StatusOK {
		t.Errorf("/api-docs returned %d after /terms was throttled; scopes are shared", code)
	}
	// And the login form, also formerly on the shared "page" scope.
	if code := get(t, srv, "/login").Code; code != http.StatusOK {
		t.Errorf("/login returned %d after /terms was throttled; scopes are shared", code)
	}
}

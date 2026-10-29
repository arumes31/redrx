package web

import (
	"context"
	"errors"
	"io"
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

func TestLogoutRevokesRetainedCookie(t *testing.T) {
	srv, _ := newTestServer(t)
	retained := login(t, srv, "alice", "alice-password")
	other := login(t, srv, "alice", "alice-password")

	page := getWithCookie(t, srv, "/settings/security", retained)
	if page.Code != http.StatusOK {
		t.Fatalf("security settings = %d, want 200", page.Code)
	}
	current := sessionCookieOr(page.Result(), retained)
	rec := postForm(t, srv, "/logout", current, url.Values{"csrf_token": {extractCSRF(t, page.Body.String())}}, "")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("logout = %d, want 303", rec.Code)
	}

	for name, cookie := range map[string]*http.Cookie{"retained": retained, "current": current} {
		if code := getWithCookie(t, srv, "/dashboard", cookie).Code; code != http.StatusSeeOther {
			t.Errorf("%s copy of the logged-out cookie still reaches the dashboard (%d)", name, code)
		}
	}
	if code := getWithCookie(t, srv, "/dashboard", other).Code; code != http.StatusOK {
		t.Errorf("an unrelated session was ended by an ordinary logout (%d)", code)
	}
}

// enableTestTOTP turns on 2FA for the fixture user with a known secret.
func enableTestTOTP(t *testing.T, srv *Server, db *store.DB, secret string) *store.User {
	t.Helper()
	ctx := context.Background()
	user, err := db.UserByLogin(ctx, "alice")
	if err != nil {
		t.Fatalf("load fixture user: %v", err)
	}
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
	return user
}

func TestLogoutEverywhereVoidsPendingSecondFactor(t *testing.T) {
	srv, db := newTestServer(t)
	const secret = "JBSWY3DPEHPK3PXP" // gitleaks:allow -- test fixture
	user := enableTestTOTP(t, srv, db, secret)

	loginPage := get(t, srv, "/login")
	cookie := sessionCookie(t, loginPage.Result())
	form := url.Values{"username": {"alice"}, "password": {"alice-password"},
		"csrf_token": {extractCSRF(t, loginPage.Body.String())}}
	rec := postForm(t, srv, "/login", cookie, form, "")
	if loc := rec.Header().Get("Location"); rec.Code != http.StatusSeeOther || loc != "/login/totp" {
		t.Fatalf("password step = %d Location %q, want 303 /login/totp", rec.Code, loc)
	}
	pending := sessionCookie(t, rec.Result())

	if _, err := db.BumpSessionEpoch(context.Background(), user.ID); err != nil {
		t.Fatalf("BumpSessionEpoch: %v", err)
	}

	page := getWithCookie(t, srv, "/login/totp", pending)
	pending = sessionCookieOr(page.Result(), pending)
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	rec = postForm(t, srv, "/login/totp", pending,
		url.Values{"code": {code}, "csrf_token": {extractCSRF(t, page.Body.String())}}, "")
	if loc := rec.Header().Get("Location"); rec.Code != http.StatusSeeOther || loc != "/login" {
		t.Fatalf("2FA step after revocation = %d Location %q, want 303 /login", rec.Code, loc)
	}
	after := sessionCookieOr(rec.Result(), pending)
	if code := getWithCookie(t, srv, "/dashboard", after).Code; code != http.StatusSeeOther {
		t.Errorf("a password check from before the revocation produced a session (%d)", code)
	}
}

func TestStaleTOTPStartCannotDisableActiveFactor(t *testing.T) {
	srv, db := newTestServer(t)
	ctx := context.Background()
	before, err := db.UserByLogin(ctx, "alice")
	if err != nil {
		t.Fatalf("load fixture user: %v", err)
	}
	enableTestTOTP(t, srv, db, "JBSWY3DPEHPK3PXP") // gitleaks:allow -- test fixture

	replacement, err := security.SealAccountSecret(srv.cfg.SecretKey, "KRSXG5CTMVRXEZLU") // gitleaks:allow -- test fixture
	if err != nil {
		t.Fatalf("encrypt TOTP secret: %v", err)
	}
	// A setup request admitted with the pre-activation snapshot.
	if err := db.SetTOTPPending(ctx, before.ID, before.SessionEpoch, replacement); !errors.Is(err, store.ErrStateChanged) {
		t.Fatalf("stale SetTOTPPending = %v, want ErrStateChanged", err)
	}
	after, err := db.UserByID(ctx, before.ID)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	// Even with a current epoch, an active factor is never overwritten.
	if err := db.SetTOTPPending(ctx, after.ID, after.SessionEpoch, replacement); !errors.Is(err, store.ErrStateChanged) {
		t.Fatalf("SetTOTPPending on an enabled factor = %v, want ErrStateChanged", err)
	}
	if after, err = db.UserByID(ctx, before.ID); err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if !after.TOTPEnabled || after.TOTPSecret == replacement {
		t.Error("a stale enrollment request disabled or replaced the active factor")
	}
}

func TestTOTPActivationIsBoundToVerifiedSecret(t *testing.T) {
	srv, db := newTestServer(t)
	ctx := context.Background()
	user, err := db.UserByLogin(ctx, "alice")
	if err != nil {
		t.Fatalf("load fixture user: %v", err)
	}
	verified, err := security.SealAccountSecret(srv.cfg.SecretKey, "JBSWY3DPEHPK3PXP") // gitleaks:allow -- test fixture
	if err != nil {
		t.Fatalf("encrypt TOTP secret: %v", err)
	}
	swapped, err := security.SealAccountSecret(srv.cfg.SecretKey, "KRSXG5CTMVRXEZLU") // gitleaks:allow -- test fixture
	if err != nil {
		t.Fatalf("encrypt TOTP secret: %v", err)
	}
	if err := db.SetTOTPPending(ctx, user.ID, user.SessionEpoch, verified); err != nil {
		t.Fatalf("SetTOTPPending: %v", err)
	}
	// A concurrent setup request replaces the pending secret.
	if err := db.SetTOTPPending(ctx, user.ID, user.SessionEpoch, swapped); err != nil {
		t.Fatalf("SetTOTPPending: %v", err)
	}
	step := time.Now().Unix() / totpPeriod
	if err := db.EnableTOTP(ctx, user.ID, verified, step, nil); !errors.Is(err, store.ErrStateChanged) {
		t.Fatalf("EnableTOTP with a superseded secret = %v, want ErrStateChanged", err)
	}
	after, err := db.UserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if after.TOTPEnabled {
		t.Error("proof for one authenticator enabled a different one")
	}
}

func TestLinkPasswordGrantDoesNotCarryToReusedCode(t *testing.T) {
	srv, db := newTestServer(t)
	ctx := context.Background()
	firstHash, err := security.GeneratePasswordHash("first-password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	secondHash, err := security.GeneratePasswordHash("second-password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	create := func(hash string) *store.URL {
		t.Helper()
		link := &store.URL{ShortCode: "GUARD1", LongURL: "https://protected.example.com/",
			PasswordHash: hash, IsEnabled: true}
		if err := db.CreateURL(ctx, link); err != nil {
			t.Fatalf("CreateURL: %v", err)
		}
		return link
	}
	gated := func(name string, cookie *http.Cookie) {
		t.Helper()
		rec := getWithCookie(t, srv, "/GUARD1", cookie)
		if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/link-auth/") {
			t.Errorf("%s = %d Location %q, want the password gate", name, rec.Code, rec.Header().Get("Location"))
		}
	}
	first := create(firstHash)

	page := get(t, srv, "/link-auth/GUARD1")
	cookie := sessionCookie(t, page.Result())
	rec := postForm(t, srv, "/link-auth/GUARD1", cookie,
		url.Values{"password": {"first-password"}, "csrf_token": {extractCSRF(t, page.Body.String())}}, "")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("link auth = %d, want 303", rec.Code)
	}
	cookie = sessionCookieOr(rec.Result(), cookie)
	if code := getWithCookie(t, srv, "/GUARD1", cookie).Code; code != http.StatusOK {
		t.Fatalf("unlocked link = %d, want 200", code)
	}

	if err := db.DeleteURL(ctx, first.ID); err != nil {
		t.Fatalf("DeleteURL: %v", err)
	}
	second := create(secondHash)
	gated("replacement link with a different password", cookie)

	// Same password hash, new link: the grant is bound to the link id too.
	if err := db.DeleteURL(ctx, second.ID); err != nil {
		t.Fatalf("DeleteURL: %v", err)
	}
	create(firstHash)
	gated("replacement link with the same password hash", cookie)
}

func TestMetricsMethodLabelIsBounded(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest("BREWCOFFEE", "/ABC123", nil)
	req.Host = "short.example.com"
	srv.ServeHTTP(httptest.NewRecorder(), req)

	rec := get(t, srv, "/metrics")
	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("read metrics: %v", err)
	}
	if strings.Contains(string(body), "BREWCOFFEE") {
		t.Error("an arbitrary request method became a metric label")
	}
	if !strings.Contains(string(body), `method="OTHER"`) {
		t.Error("unknown methods are not reported under the shared OTHER label")
	}
}

func TestLastAccessedRespectsPrivacySignals(t *testing.T) {
	srv, db := newTestServer(t, func(c *config.Config) { c.HonorDoNotTrack = true })
	ctx := context.Background()
	link := &store.URL{ShortCode: "QUIET1", LongURL: "https://quiet.example.com/",
		StatsEnabled: true, IsEnabled: true}
	if err := db.CreateURL(ctx, link); err != nil {
		t.Fatalf("CreateURL: %v", err)
	}

	visit := func(dnt bool) {
		req := httptest.NewRequest(http.MethodGet, "/QUIET1", nil)
		req.Host = "short.example.com"
		if dnt {
			req.Header.Set("DNT", "1")
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("redirect page = %d, want 200", rec.Code)
		}
	}

	visit(true)
	stored, err := db.URLByShortCode(ctx, "QUIET1")
	if err != nil {
		t.Fatalf("load link: %v", err)
	}
	if stored.LastAccessedAt != nil {
		t.Error("a Do-Not-Track visit updated the owner-visible last access time")
	}
	visit(false)
	if stored, err = db.URLByShortCode(ctx, "QUIET1"); err != nil {
		t.Fatalf("reload link: %v", err)
	}
	if stored.LastAccessedAt == nil {
		t.Error("a tracked visit did not update the last access time")
	}
}

func TestLastAccessedFollowsConsentAndStatsSetting(t *testing.T) {
	srv, db := newTestServer(t, func(c *config.Config) { c.EnableConsentBanner = true })
	ctx := context.Background()
	for _, l := range []*store.URL{
		{ShortCode: "NOSTAT", LongURL: "https://quiet.example.com/", StatsEnabled: false, IsEnabled: true},
		{ShortCode: "CONSNT", LongURL: "https://quiet.example.com/", StatsEnabled: true, IsEnabled: true},
	} {
		if err := db.CreateURL(ctx, l); err != nil {
			t.Fatalf("CreateURL: %v", err)
		}
	}
	visit := func(code string, consent bool) *store.URL {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/"+code, nil)
		req.Host = "short.example.com"
		if consent {
			req.AddCookie(&http.Cookie{Name: consentCookieName, Value: "accepted"})
		}
		srv.ServeHTTP(httptest.NewRecorder(), req)
		stored, err := db.URLByShortCode(ctx, code)
		if err != nil {
			t.Fatalf("load link: %v", err)
		}
		return stored
	}

	if visit("NOSTAT", true).LastAccessedAt != nil {
		t.Error("a link with statistics disabled recorded its last access time")
	}
	if visit("CONSNT", false).LastAccessedAt != nil {
		t.Error("a visit without consent recorded the last access time")
	}
	if visit("CONSNT", true).LastAccessedAt == nil {
		t.Error("a consented visit did not record the last access time")
	}
}

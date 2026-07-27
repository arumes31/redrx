package web

import (
	"encoding/csv"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/arumes31/redrx/internal/qr"
	"github.com/arumes31/redrx/internal/security"
	"github.com/arumes31/redrx/internal/shortcode"
	"github.com/arumes31/redrx/internal/store"
	"github.com/google/uuid"
)

const dashboardPageSize = 10

// dummyPasswordHash is verified against when a login names an account that does
// not exist, so the response takes the same time either way. It is a real
// scrypt hash of a random string; no password matches it.
const dummyPasswordHash = "scrypt:32768:8:1$XmQ2yTnBd7pKwLr4$" +
	"7c1a3f2b9d8e4c6a5b0f2d1e8a7c4b3d6e9f0a2c5b8d1e4f7a0c3b6d9e2f5a8c" +
	"1b4d7e0a3c6f9b2e5d8a1c4f7b0e3d6a9c2f5b8e1d4a7c0f3b6e9d2a5c8f1b4e"

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data := s.newPageData(r)
	data.Data["form"] = newShortenForm(s.cfg.ShortCodeLength, s.cfg.ExpiryHours, s.cfg.DefaultQRColor, s.cfg.DefaultQRBG)
	s.render(w, r, http.StatusOK, "index.html", data)
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	user := userFrom(r)

	if s.cfg.DisableAnonymousCreate && user == nil {
		sess.AddFlash("warning", "Please log in to shorten URLs.")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseMultipartForm(s.cfg.MaxUploadSize); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		sess.AddFlash("danger", "Could not read the submitted form.")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	form := bindShortenForm(r)
	in, ok := form.Validate(s.cfg.ShortCodeLength, user != nil)

	data := s.newPageData(r)
	data.Data["form"] = form

	if !ok {
		s.render(w, r, http.StatusOK, "index.html", data)
		return
	}

	if !s.safety.IsSafeURL(in.LongURL) {
		sess.AddFlash("danger", "That destination URL is blocked for safety reasons.")
		data.Flashes = sess.TakeFlashes()
		s.render(w, r, http.StatusOK, "index.html", data)
		return
	}
	for _, t := range in.RotateTargets {
		if !s.safety.IsSafeURL(t) {
			form.Errors.add("rotate_targets", "One or more rotate target URLs are blocked or invalid.")
			s.render(w, r, http.StatusOK, "index.html", data)
			return
		}
	}
	if in.IOSTargetURL != "" && !s.safety.IsSafeURL(in.IOSTargetURL) {
		form.Errors.add("ios_target_url", "iOS target URL is blocked or invalid.")
		s.render(w, r, http.StatusOK, "index.html", data)
		return
	}
	if in.AndroidTargetURL != "" && !s.safety.IsSafeURL(in.AndroidTargetURL) {
		form.Errors.add("android_target_url", "Android target URL is blocked or invalid.")
		s.render(w, r, http.StatusOK, "index.html", data)
		return
	}

	code, err := s.resolveShortCode(r, in.CustomCode, in.CodeLength)
	if err != nil {
		if errors.Is(err, errCodeTaken) {
			form.Errors.add("custom_code", "Code '"+in.CustomCode+"' is already taken.")
			s.render(w, r, http.StatusOK, "index.html", data)
			return
		}
		s.log.Error("allocate short code", "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}

	link := &store.URL{
		ShortCode:        code,
		LongURL:          in.LongURL,
		RotateTargets:    in.RotateTargets,
		IOSTargetURL:     in.IOSTargetURL,
		AndroidTargetURL: in.AndroidTargetURL,
		PreviewMode:      in.PreviewMode,
		StatsEnabled:     in.StatsEnabled,
		IsEnabled:        true,
		ExpiresAt:        in.ExpiresAt,
		StartAt:          in.StartAt,
		EndAt:            in.EndAt,
	}
	if user != nil {
		link.UserID = &user.ID
	}
	if in.Password != "" {
		hash, err := security.GeneratePasswordHash(in.Password)
		if err != nil {
			s.log.Error("hash link password", "error", err)
			s.renderError(w, r, http.StatusInternalServerError)
			return
		}
		link.PasswordHash = hash
	}

	if err := s.db.CreateURL(r.Context(), link); err != nil {
		// The availability check above can be raced by a concurrent request;
		// the unique index decides, and the loser sees the ordinary "taken"
		// field error rather than a 500.
		if store.IsUniqueViolation(err) {
			form.Errors.add("custom_code", "Code '"+code+"' is already taken.")
			s.render(w, r, http.StatusOK, "index.html", data)
			return
		}
		s.log.Error("create link", "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}
	s.metrics.shortened.Inc()

	shortURL := s.cfg.ShortURL(code)
	qrPayload, err := s.renderQRForForm(r, shortURL, form)
	if err != nil {
		s.log.Warn("render qr code", "error", err)
	}

	sess.AddFlash("success", "URL Shortened Successfully!")

	data = s.newPageData(r)
	data.Data["form"] = newShortenForm(s.cfg.ShortCodeLength, s.cfg.ExpiryHours, s.cfg.DefaultQRColor, s.cfg.DefaultQRBG)
	data.Data["short_url"] = shortURL
	data.Data["short_code"] = code
	data.Data["qr_data"] = qrPayload
	data.Data["stats_url"] = shortURL + "/stats"
	s.render(w, r, http.StatusOK, "index.html", data)
}

// renderQRForForm builds the inline QR preview, applying the submitted colours
// and optional logo upload.
func (s *Server) renderQRForForm(r *http.Request, shortURL string, form *ShortenForm) (string, error) {
	opts := qr.Options{Foreground: form.QRColor, Background: form.QRBg}

	if file, header, err := r.FormFile("logo_file"); err == nil {
		defer file.Close()
		if header.Size <= s.cfg.MaxUploadSize {
			buf := make([]byte, header.Size)
			if _, err := io_ReadFull(file, buf); err == nil {
				if img, err := qr.DecodeLogo(buf); err == nil {
					opts.Logo = img
				} else {
					s.log.Debug("uploaded logo could not be decoded", "error", err)
				}
			}
		}
	}

	return qr.DataURLPayload(shortURL, opts)
}

var errCodeTaken = errors.New("short code already taken")

// resolveShortCode returns the custom code when free, otherwise generates one.
func (s *Server) resolveShortCode(r *http.Request, custom string, length int) (string, error) {
	if custom != "" {
		taken, err := s.db.ShortCodeTaken(r.Context(), custom)
		if err != nil {
			return "", err
		}
		if taken {
			return "", errCodeTaken
		}
		return custom, nil
	}
	return shortcode.GenerateUnique(length, func(code string) (bool, error) {
		return s.db.ShortCodeTaken(r.Context(), code)
	})
}

func (s *Server) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	if userFrom(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	data := s.newPageData(r)
	data.Data["form"] = &LoginForm{Errors: errorMap{}}
	s.render(w, r, http.StatusOK, "login_user.html", data)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	if userFrom(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	login := strings.TrimSpace(r.PostFormValue("username"))
	password := r.PostFormValue("password")

	form := &LoginForm{Username: login, Errors: errorMap{}}

	user, err := s.db.UserByLogin(r.Context(), login)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.log.Error("look up user", "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}

	// Hash even when the account does not exist. scrypt costs ~100ms, so
	// returning early on an unknown user would time-distinguish it from a wrong
	// password and turn login into a username oracle.
	ok := user != nil && security.CheckPasswordHash(user.PasswordHash, password)
	if user == nil {
		security.CheckPasswordHash(dummyPasswordHash, password)
	}

	if !ok {
		sess.AddFlash("danger", "Login Unsuccessful. Please check username/email and password")
		// Also attach it to the field, so the message appears next to the input
		// rather than only in a banner that may be scrolled off a short screen.
		form.Errors.add("password", "Incorrect username/email or password.")
		data := s.newPageData(r)
		data.Data["form"] = form
		s.render(w, r, http.StatusOK, "login_user.html", data)
		return
	}

	// Transparently upgrade hashes left behind by the old deployment.
	if security.NeedsRehash(user.PasswordHash) {
		if hash, err := security.GeneratePasswordHash(password); err == nil {
			if err := s.db.SetUserPasswordHash(r.Context(), user.ID, hash); err != nil {
				s.log.Warn("upgrade password hash", "user", user.ID, "error", err)
			}
		}
	}

	sess.Login(user.ID)

	// isSafeRedirect admits only same-site relative paths, which gosec's taint
	// analysis cannot follow through the helper.
	if next := r.URL.Query().Get("next"); next != "" && isSafeRedirect(next) {
		http.Redirect(w, r, next, http.StatusSeeOther) // #nosec G710 -- validated by isSafeRedirect
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// isSafeRedirect permits only same-site relative targets, so `next` cannot be
// turned into an open redirect.
func isSafeRedirect(target string) bool {
	if target == "" || strings.HasPrefix(target, "//") || strings.HasPrefix(target, `/\`) {
		return false
	}
	u, err := url.Parse(target)
	if err != nil {
		return false
	}
	return u.Scheme == "" && u.Host == "" && strings.HasPrefix(u.Path, "/")
}

func (s *Server) handleRegisterForm(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	if s.cfg.DisableRegistration {
		sess.AddFlash("info", "Registration is currently disabled.")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if userFrom(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	data := s.newPageData(r)
	data.Data["form"] = &RegisterForm{Errors: errorMap{}}
	s.render(w, r, http.StatusOK, "register.html", data)
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	if s.cfg.DisableRegistration {
		sess.AddFlash("info", "Registration is currently disabled.")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	form := &RegisterForm{
		Username: strings.TrimSpace(r.PostFormValue("username")),
		Email:    strings.TrimSpace(r.PostFormValue("email")),
		Errors:   errorMap{},
	}
	password := r.PostFormValue("password")

	renderForm := func() {
		data := s.newPageData(r)
		data.Data["form"] = form
		s.render(w, r, http.StatusOK, "register.html", data)
	}

	if !form.Validate(password) {
		renderForm()
		return
	}

	if taken, err := s.db.UsernameTaken(r.Context(), form.Username); err != nil {
		s.log.Error("check username", "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	} else if taken {
		form.Errors.add("username", "That username is already taken.")
		renderForm()
		return
	}

	if taken, err := s.db.EmailTaken(r.Context(), form.Email); err != nil {
		s.log.Error("check email", "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	} else if taken {
		form.Errors.add("email", "That email is already registered.")
		renderForm()
		return
	}

	hash, err := security.GeneratePasswordHash(password)
	if err != nil {
		s.log.Error("hash password", "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}

	user := &store.User{
		Username:     form.Username,
		Email:        form.Email,
		PasswordHash: hash,
		APIKey:       uuid.NewString(),
	}
	if err := s.db.CreateUser(r.Context(), user); err != nil {
		s.log.Error("create user", "error", err)
		form.Errors.add("username", "Could not create the account. Please try again.")
		renderForm()
		return
	}

	sess.AddFlash("success", "Your account has been created! You can now log in.")
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	sessionFrom(r).Logout()
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// pagination describes the dashboard's page controls.
type pagination struct {
	Page    int
	Pages   int
	Total   int64
	HasPrev bool
	HasNext bool
	PrevNum int
	NextNum int
}

// Numbers returns the page numbers to render, with 0 marking an ellipsis.
func (p pagination) Numbers() []int {
	if p.Pages <= 0 {
		return nil
	}
	if p.Pages <= 9 {
		return seq(1, p.Pages)
	}

	var out []int
	add := func(n int) {
		if len(out) > 0 && out[len(out)-1] == n {
			return
		}
		out = append(out, n)
	}

	add(1)
	if p.Page-2 > 2 {
		add(0)
	}
	for n := max(2, p.Page-2); n <= min(p.Pages-1, p.Page+2); n++ {
		add(n)
	}
	if p.Page+2 < p.Pages-1 {
		add(0)
	}
	add(p.Pages)
	return out
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r)

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	stats, err := s.db.DashboardStats(r.Context(), user.ID)
	if err != nil {
		s.log.Error("dashboard stats", "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}

	pages := int((stats.TotalLinks + dashboardPageSize - 1) / dashboardPageSize)
	if pages < 1 {
		pages = 1
	}
	if page > pages {
		page = pages
	}

	urls, err := s.db.UserURLs(r.Context(), user.ID, dashboardPageSize, (page-1)*dashboardPageSize)
	if err != nil {
		s.log.Error("list user links", "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}

	data := s.newPageData(r)
	data.Data["stats"] = stats
	data.Data["urls"] = urls
	data.Data["pagination"] = pagination{
		Page: page, Pages: pages, Total: stats.TotalLinks,
		HasPrev: page > 1, HasNext: page < pages,
		PrevNum: page - 1, NextNum: page + 1,
	}
	s.render(w, r, http.StatusOK, "dashboard.html", data)
}

func (s *Server) handleRegenerateAPIKey(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r)
	sess := sessionFrom(r)

	if err := s.db.SetAPIKey(r.Context(), user.ID, uuid.NewString()); err != nil {
		s.log.Error("regenerate api key", "error", err)
		sess.AddFlash("danger", "Could not regenerate the API key.")
	} else {
		sess.AddFlash("success", "API Key regenerated successfully.")
	}
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// ownedLink loads a link and confirms the current user owns it.
func (s *Server) ownedLink(w http.ResponseWriter, r *http.Request) *store.URL {
	code := shortcode.Normalize(r.PathValue("code"))
	link, err := s.db.URLByShortCode(r.Context(), code)
	if errors.Is(err, store.ErrNotFound) {
		s.renderError(w, r, http.StatusNotFound)
		return nil
	}
	if err != nil {
		s.log.Error("load link", "code", code, "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return nil
	}

	user := userFrom(r)
	if user == nil || link.UserID == nil || *link.UserID != user.ID {
		s.renderError(w, r, http.StatusForbidden)
		return nil
	}
	return link
}

func (s *Server) handleToggleStatus(w http.ResponseWriter, r *http.Request) {
	link := s.ownedLink(w, r)
	if link == nil {
		return
	}
	if err := s.db.SetURLEnabled(r.Context(), link.ID, !link.IsEnabled); err != nil {
		s.log.Error("toggle link status", "error", err)
		apiError(w, http.StatusInternalServerError, "Could not update the link.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "success",
		"is_enabled": !link.IsEnabled,
	})
}

func (s *Server) handleExportLinks(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r)

	links, err := s.db.AllUserURLs(r.Context(), user.ID)
	if err != nil {
		s.log.Error("export links", "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="my_links.csv"`)
	w.WriteHeader(http.StatusOK)

	cw := csv.NewWriter(w)
	defer cw.Flush()

	_ = cw.Write([]string{"Short Code", "Long URL", "Clicks", "Created At", "Last Accessed", "Expires At"})
	for _, l := range links {
		_ = cw.Write([]string{
			sanitizeCSVField(l.ShortCode),
			sanitizeCSVField(l.LongURL),
			sanitizeCSVField(l.ClicksCount),
			sanitizeCSVField(l.CreatedAt.Format("2006-01-02T15:04:05")),
			sanitizeCSVField(optionalTime(l.LastAccessedAt)),
			sanitizeCSVField(optionalTime(l.ExpiresAt)),
		})
	}
}

func (s *Server) handleBulkDelete(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r)
	sess := sessionFrom(r)

	if err := r.ParseForm(); err != nil {
		sess.AddFlash("warning", "Could not read the submitted form.")
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	raw := r.PostForm["link_ids"]
	if len(raw) == 0 {
		sess.AddFlash("warning", "No links selected.")
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	ids := make([]int64, 0, len(raw))
	for _, v := range raw {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			ids = append(ids, id)
		}
	}

	deleted, err := s.db.DeleteUserURLs(r.Context(), user.ID, ids)
	if err != nil {
		s.log.Error("bulk delete", "error", err)
		sess.AddFlash("danger", "Could not delete the selected links.")
	} else {
		sess.AddFlash("info", "Successfully deleted "+strconv.FormatInt(deleted, 10)+" links.")
	}
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (s *Server) handleEditForm(w http.ResponseWriter, r *http.Request) {
	link := s.ownedLink(w, r)
	if link == nil {
		return
	}

	form := &EditForm{
		LongURL:          link.LongURL,
		IOSTargetURL:     link.IOSTargetURL,
		AndroidTargetURL: link.AndroidTargetURL,
		PreviewMode:      link.PreviewMode,
		StatsEnabled:     link.StatsEnabled,
		Errors:           errorMap{},
	}
	if link.ExpiresAt != nil {
		hours := int(link.ExpiresAt.Sub(nowUTC()).Hours())
		form.ExpiryHours = strconv.Itoa(max(0, hours))
	}

	data := s.newPageData(r)
	data.Data["form"] = form
	data.Data["short_code"] = link.ShortCode
	s.render(w, r, http.StatusOK, "edit_url.html", data)
}

func (s *Server) handleEdit(w http.ResponseWriter, r *http.Request) {
	link := s.ownedLink(w, r)
	if link == nil {
		return
	}
	sess := sessionFrom(r)

	form := bindEditForm(r)
	in, ok := form.Validate()

	renderForm := func() {
		data := s.newPageData(r)
		data.Data["form"] = form
		data.Data["short_code"] = link.ShortCode
		s.render(w, r, http.StatusOK, "edit_url.html", data)
	}

	if !ok {
		renderForm()
		return
	}

	if !s.safety.IsSafeURL(in.LongURL) {
		form.Errors.add("long_url", "That destination URL is blocked or invalid.")
		renderForm()
		return
	}
	if in.IOSTargetURL != "" && !s.safety.IsSafeURL(in.IOSTargetURL) {
		form.Errors.add("ios_target_url", "iOS target URL is blocked or invalid.")
		renderForm()
		return
	}
	if in.AndroidTargetURL != "" && !s.safety.IsSafeURL(in.AndroidTargetURL) {
		form.Errors.add("android_target_url", "Android target URL is blocked or invalid.")
		renderForm()
		return
	}

	link.LongURL = in.LongURL
	link.IOSTargetURL = in.IOSTargetURL
	link.AndroidTargetURL = in.AndroidTargetURL
	link.PreviewMode = in.PreviewMode
	link.StatsEnabled = in.StatsEnabled
	if in.ExpiryGiven {
		if in.ExpiryNever {
			link.ExpiresAt = nil
		} else {
			link.ExpiresAt = in.ExpiresAt
		}
	}

	if err := s.db.UpdateURL(r.Context(), link); err != nil {
		s.log.Error("update link", "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}

	sess.AddFlash("success", "Link updated successfully.")
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	link := s.ownedLink(w, r)
	if link == nil {
		return
	}
	sess := sessionFrom(r)

	if err := s.db.DeleteURL(r.Context(), link.ID); err != nil {
		s.log.Error("delete link", "error", err)
		sess.AddFlash("danger", "Could not delete the link.")
	} else {
		sess.AddFlash("info", "Link deleted successfully.")
	}
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (s *Server) handleAPIDocs(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "api_docs.html", s.newPageData(r))
}

func (s *Server) handleDataUsage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "data_usage.html", s.newPageData(r))
}

func (s *Server) handleTerms(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "terms.html", s.newPageData(r))
}

func (s *Server) handleRobots(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.EnableSEO {
		s.renderError(w, r, http.StatusNotFound)
		return
	}
	s.render(w, r, http.StatusOK, "robots.txt", s.newPageData(r))
}

func (s *Server) handleSitemap(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.EnableSEO {
		s.renderError(w, r, http.StatusNotFound)
		return
	}
	s.render(w, r, http.StatusOK, "sitemap.xml", s.newPageData(r))
}

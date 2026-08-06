package web

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"

	"github.com/arumes31/redrx/internal/qr"
	"github.com/arumes31/redrx/internal/security"
	"github.com/arumes31/redrx/internal/store"
)

func (s *Server) handleTOTPLoginForm(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if userFrom(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if _, _, ok := sessionFrom(r).PendingTwoFactor(); !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	data := s.newPageData(r)
	data.Data["errors"] = errorMap{}
	s.render(w, r, http.StatusOK, "login_totp.html", data)
}

func (s *Server) handleTOTPLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	sess := sessionFrom(r)
	userID, next, ok := sess.PendingTwoFactor()
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	user, err := s.db.UserByID(r.Context(), userID)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			s.log.Error("load pending 2FA user", "error", err)
		}
		sess.Logout()
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	valid, err := s.validateSecondFactor(r, user, r.PostFormValue("code"))
	if err != nil {
		s.log.Error("validate second factor", "user", user.ID, "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}
	if !valid {
		data := s.newPageData(r)
		errs := errorMap{}
		errs.add("code", "Enter a valid authenticator or unused recovery code.")
		data.Data["errors"] = errs
		s.render(w, r, http.StatusOK, "login_totp.html", data)
		return
	}

	sess.Login(user.ID)
	if isSafeRedirect(next) {
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleSecuritySettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	data := s.newPageData(r)
	s.render(w, r, http.StatusOK, "security_settings.html", data)
}

func (s *Server) handleTOTPStart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	user := userFrom(r)
	if user.TOTPEnabled {
		sessionFrom(r).AddFlash("info", "Two-factor authentication is already enabled.")
		http.Redirect(w, r, "/settings/security", http.StatusSeeOther)
		return
	}
	issuer := s.totpIssuer()
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer: issuer, AccountName: user.Email, Period: 30,
		SecretSize: 20, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		s.log.Error("generate TOTP enrollment", "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}
	encrypted, err := security.SealAccountSecret(s.cfg.SecretKey, key.Secret())
	if err != nil {
		s.log.Error("encrypt TOTP secret", "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}
	if err := s.db.SetTOTPPending(r.Context(), user.ID, encrypted); err != nil {
		s.log.Error("store TOTP enrollment", "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}
	qrData, err := qr.DataURLPayload(key.URL(), qr.Options{Foreground: "#000000", Background: "#ffffff"})
	if err != nil {
		s.log.Error("render TOTP QR", "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}
	data := s.newPageData(r)
	data.Data["totp_secret"] = key.Secret()
	data.Data["totp_qr"] = qrData
	s.render(w, r, http.StatusOK, "security_settings.html", data)
}

func (s *Server) handleTOTPConfirm(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	user, err := s.db.UserByID(r.Context(), userFrom(r).ID)
	if err != nil || user.TOTPSecret == "" || user.TOTPEnabled {
		sessionFrom(r).AddFlash("warning", "Start two-factor setup before confirming it.")
		http.Redirect(w, r, "/settings/security", http.StatusSeeOther)
		return
	}
	secret, err := security.OpenAccountSecret(s.cfg.SecretKey, user.TOTPSecret)
	if err != nil {
		s.log.Error("decrypt pending TOTP secret", "user", user.ID, "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}
	valid, err := validateTOTP(secret, r.PostFormValue("code"))
	if err != nil || !valid {
		sessionFrom(r).AddFlash("danger", "The authenticator code was invalid. Check the clock and try again.")
		qrData, qrErr := qr.DataURLPayload(s.totpURL(user, secret), qr.Options{Foreground: "#000000", Background: "#ffffff"})
		if qrErr != nil {
			s.log.Error("render pending TOTP QR", "error", qrErr)
			s.renderError(w, r, http.StatusInternalServerError)
			return
		}
		data := s.newPageData(r)
		data.Data["totp_secret"] = secret
		data.Data["totp_qr"] = qrData
		s.render(w, r, http.StatusOK, "security_settings.html", data)
		return
	}
	codes, err := security.GenerateRecoveryCodes(10)
	if err != nil {
		s.log.Error("generate recovery codes", "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}
	hashes := make([]string, len(codes))
	for i, code := range codes {
		hashes[i] = security.RecoveryCodeHash(s.cfg.SecretKey, code)
	}
	if err := s.db.EnableTOTP(r.Context(), user.ID, hashes); err != nil {
		s.log.Error("enable TOTP", "user", user.ID, "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}
	user.TOTPEnabled = true
	data := s.newPageData(r)
	data.User = user
	data.Data["recovery_codes"] = codes
	s.render(w, r, http.StatusOK, "security_settings.html", data)
}

func (s *Server) handleTOTPDisable(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	user, err := s.db.UserByID(r.Context(), userFrom(r).ID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}
	if !security.CheckPasswordHash(user.PasswordHash, r.PostFormValue("password")) {
		sessionFrom(r).AddFlash("danger", "The password was incorrect.")
		http.Redirect(w, r, "/settings/security", http.StatusSeeOther)
		return
	}
	valid, err := s.validateSecondFactor(r, user, r.PostFormValue("code"))
	if err != nil {
		s.log.Error("validate factor while disabling TOTP", "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}
	if !valid {
		sessionFrom(r).AddFlash("danger", "The authenticator or recovery code was invalid.")
		http.Redirect(w, r, "/settings/security", http.StatusSeeOther)
		return
	}
	if err := s.db.DisableTOTP(r.Context(), user.ID); err != nil {
		s.log.Error("disable TOTP", "error", err)
		s.renderError(w, r, http.StatusInternalServerError)
		return
	}
	sessionFrom(r).AddFlash("success", "Two-factor authentication disabled.")
	http.Redirect(w, r, "/settings/security", http.StatusSeeOther)
}

func (s *Server) validateSecondFactor(r *http.Request, user *store.User, code string) (bool, error) {
	if user.TOTPSecret == "" {
		return false, nil
	}
	secret, err := security.OpenAccountSecret(s.cfg.SecretKey, user.TOTPSecret)
	if err != nil {
		return false, err
	}
	trimmed := strings.TrimSpace(code)
	if len(trimmed) == 6 && strings.IndexFunc(trimmed, func(r rune) bool { return r < '0' || r > '9' }) == -1 {
		if valid, err := validateTOTP(secret, trimmed); err != nil || valid {
			return valid, err
		}
	}
	hash := security.RecoveryCodeHash(s.cfg.SecretKey, code)
	return s.db.ConsumeRecoveryCode(r.Context(), user.ID, hash)
}

func validateTOTP(secret, code string) (bool, error) {
	code = strings.TrimSpace(code)
	return totp.ValidateCustom(code, secret, time.Now(), totp.ValidateOpts{
		Period: 30, Skew: 1, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1,
	})
}

func (s *Server) totpIssuer() string {
	issuer := "Redrx"
	if host := s.cfg.CanonicalHost(); host != "" {
		issuer += " (" + host + ")"
	}
	return issuer
}

func (s *Server) totpURL(user *store.User, secret string) string {
	issuer := s.totpIssuer()
	query := url.Values{
		"secret":    {secret},
		"issuer":    {issuer},
		"period":    {"30"},
		"digits":    {"6"},
		"algorithm": {"SHA1"},
	}
	u := &url.URL{Scheme: "otpauth", Host: "totp", Path: "/" + issuer + ":" + user.Email, RawQuery: query.Encode()}
	return u.String()
}

package web

import (
	"net/http"
	"strings"
	"time"

	proof "github.com/arumes31/redrx/internal/pow"
)

const consentCookieName = "redrx_consent"

func (s *Server) addAnonymousProof(r *http.Request, data *PageData) {
	if userFrom(r) != nil || s.cfg.AnonymousPoWDifficulty == 0 {
		return
	}
	challenge, err := proof.Issue(s.cfg.SecretKey, 10*time.Minute)
	if err != nil {
		s.log.Error("create proof-of-work challenge", "error", err)
		return
	}
	sessionFrom(r).SetPoWChallenge(challenge)
	data.Data["pow_challenge"] = challenge
	data.Data["pow_difficulty"] = s.cfg.AnonymousPoWDifficulty
}

func (s *Server) verifyAnonymousProof(r *http.Request) bool {
	if userFrom(r) != nil || s.cfg.AnonymousPoWDifficulty == 0 {
		return true
	}
	challenge := r.PostFormValue("pow_challenge")
	expiresAt, err := proof.VerifyWithExpiry(s.cfg.SecretKey, challenge,
		r.PostFormValue("pow_solution"), s.cfg.AnonymousPoWDifficulty)
	if err != nil {
		return false
	}
	if !sessionFrom(r).ConsumePoWChallenge(challenge) {
		return false
	}
	claimed, err := s.db.ClaimPoWChallenge(r.Context(), challenge, expiresAt)
	if err != nil {
		s.log.Error("claim proof-of-work challenge", "error", err)
		return false
	}
	return claimed
}

func (s *Server) handleConsent(w http.ResponseWriter, r *http.Request) {
	choice := strings.ToLower(strings.TrimSpace(r.PostFormValue("choice")))
	if choice != "accepted" && choice != "declined" {
		s.renderError(w, r, http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{ // #nosec G124 -- Secure follows the server mode
		Name: consentCookieName, Value: choice, Path: "/",
		MaxAge: 365 * 24 * 60 * 60, HttpOnly: true,
		Secure: !s.cfg.Debug, SameSite: http.SameSiteLaxMode,
	})
	target := r.PostFormValue("return_to")
	if !isSafeRedirect(target) {
		target = "/"
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (s *Server) consentChoice(r *http.Request) string {
	c, err := r.Cookie(consentCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

func (s *Server) shouldTrack(r *http.Request) bool {
	if s.cfg.HonorDoNotTrack && strings.TrimSpace(r.Header.Get("DNT")) == "1" {
		return false
	}
	if s.cfg.EnableConsentBanner && s.consentChoice(r) != "accepted" {
		return false
	}
	return true
}

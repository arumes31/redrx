// Package session implements signed cookie sessions.
//
// The payload is JSON, base64url-encoded and authenticated with HMAC-SHA256
// keyed on SECRET_KEY. Nothing secret is stored in it — only a user and session id,
// flash messages, a CSRF token, a short-lived pending login, an anonymous-work
// challenge and the set of password-protected links this visitor has unlocked
// — so signing without encryption is sufficient.
package session

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	cookieName = "redrx_session"
	// MaxAge bounds both the cookie lifetime and the signature's validity, and
	// so how long a revoked session id must be remembered.
	MaxAge = 30 * 24 * time.Hour
	// maxCookieBytes leaves headroom under the 4 KiB browser limit.
	maxCookieBytes = 3800
)

// Flash is a one-shot message rendered on the next page load.
type Flash struct {
	Category string `json:"c"`
	Message  string `json:"m"`
}

// Data is the serialised session payload.
type Data struct {
	UserID int64 `json:"uid,omitempty"`
	// Epoch is the user's session epoch at login. Cookies are stateless, so
	// revocation works by bumping the stored epoch, which orphans every cookie
	// carrying an older one.
	Epoch int64 `json:"ep,omitempty"`
	// SID identifies this login, so an ordinary logout can revoke the cookie
	// it was issued as without touching the user's other sessions.
	SID           string `json:"sid,omitempty"`
	PendingUserID int64  `json:"puid,omitempty"`
	// PendingEpoch is the user's session epoch when the password was verified,
	// so revoking sessions also voids a login waiting on its second factor.
	PendingEpoch int64   `json:"pep,omitempty"`
	PendingNext  string  `json:"pn,omitempty"`
	PendingSince int64   `json:"ps,omitempty"`
	PoWChallenge string  `json:"pow,omitempty"`
	CSRF         string  `json:"csrf,omitempty"`
	Flashes      []Flash `json:"fl,omitempty"`
	// LinkAuth maps an unlocked short code to the grant for the link instance
	// and password it was unlocked with (see LinkGrant). The JSON key is new:
	// the earlier "la" set was keyed by code alone, so those grants are
	// deliberately ignored rather than carried over.
	LinkAuth map[string]string `json:"lg,omitempty"`
	Issued   int64             `json:"iat,omitempty"`
}

// Session is the per-request view of the cookie, plus a dirty flag so unchanged
// sessions do not rewrite the cookie on every response.
type Session struct {
	Data
	dirty  bool
	secure bool
}

// Manager signs and verifies session cookies.
type Manager struct {
	key []byte
	// secure marks cookies Secure; disabled in debug so plain-HTTP dev works.
	secure bool
}

func NewManager(secretKey []byte, secure bool) *Manager {
	// Derive a distinct key so the session MAC is not the raw configured
	// secret, which may be reused for other purposes.
	sum := sha256.Sum256(append([]byte("redrx.session.v1|"), secretKey...))
	return &Manager{key: sum[:], secure: secure}
}

// Load reads and verifies the session cookie, returning an empty session when
// the cookie is absent, tampered with or expired.
func (m *Manager) Load(r *http.Request) *Session {
	s := &Session{secure: m.secure}
	s.LinkAuth = map[string]string{}

	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return s
	}
	payload, ok := m.verify(c.Value)
	if !ok {
		return s
	}
	var d Data
	if err := json.Unmarshal(payload, &d); err != nil {
		return s
	}
	if d.Issued > 0 && time.Since(time.Unix(d.Issued, 0)) > MaxAge {
		return s
	}
	s.Data = d
	if s.LinkAuth == nil {
		s.LinkAuth = map[string]string{}
	}
	return s
}

// Save writes the cookie if the session changed. It must be called before any
// response body is written.
func (m *Manager) Save(w http.ResponseWriter, s *Session) {
	if s == nil || !s.dirty {
		return
	}
	s.dirty = false

	if s.isEmpty() {
		// Secure comes from the manager rather than a literal, which is why
		// gosec cannot see it; it is false only when Debug is on, where the dev
		// server is plain HTTP and a Secure cookie would never come back.
		http.SetCookie(w, &http.Cookie{ // #nosec G124 -- Secure is set from config, see NewManager
			Name: cookieName, Value: "", Path: "/", MaxAge: -1,
			HttpOnly: true, Secure: m.secure, SameSite: http.SameSiteLaxMode,
		})
		return
	}

	s.Issued = time.Now().Unix()
	value, ok := m.shrinkToFit(s)
	if !ok {
		return
	}

	http.SetCookie(w, &http.Cookie{ // #nosec G124 -- Secure is set from config, see NewManager
		Name: cookieName, Value: value, Path: "/",
		MaxAge: int(MaxAge.Seconds()), HttpOnly: true,
		Secure: m.secure, SameSite: http.SameSiteLaxMode,
	})
}

// shrinkToFit signs the session, dropping the least important state until the
// cookie fits. An oversized Set-Cookie is discarded by the browser in full, so
// emitting one would silently log the visitor out and lose every unlocked link
// — shedding some state is always better than shedding all of it.
func (m *Manager) shrinkToFit(s *Session) (string, bool) {
	value, ok := m.trySign(s)
	if !ok {
		return "", false
	}
	if len(value) <= maxCookieBytes {
		return value, true
	}

	// Flashes are one-shot and the largest single contributor, so they go first.
	if len(s.Flashes) > 0 {
		s.Flashes = nil
		if value, ok = m.trySign(s); !ok {
			return "", false
		}
		if len(value) <= maxCookieBytes {
			return value, true
		}
	}

	// Then the oldest link authorisations, which cost the visitor no more than
	// re-entering a link password. Map order is unspecified, so this evicts an
	// arbitrary entry each round rather than a strictly oldest one.
	for len(s.LinkAuth) > 0 {
		for code := range s.LinkAuth {
			delete(s.LinkAuth, code)
			break
		}
		if value, ok = m.trySign(s); !ok {
			return "", false
		}
		if len(value) <= maxCookieBytes {
			return value, true
		}
	}

	// Nothing left to shed: the identity alone is over the limit, which cannot
	// happen with the fields defined here.
	return value, len(value) <= maxCookieBytes
}

func (m *Manager) trySign(s *Session) (string, bool) {
	payload, err := json.Marshal(s.Data)
	if err != nil {
		return "", false
	}
	return m.sign(payload), true
}

func (m *Manager) sign(payload []byte) string {
	body := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, m.key)
	mac.Write([]byte(body))
	return body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (m *Manager) verify(value string) ([]byte, bool) {
	body, sig, ok := strings.Cut(value, ".")
	if !ok {
		return nil, false
	}
	want, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return nil, false
	}
	mac := hmac.New(sha256.New, m.key)
	mac.Write([]byte(body))
	if subtle.ConstantTimeCompare(want, mac.Sum(nil)) != 1 {
		return nil, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return nil, false
	}
	return payload, true
}

func (s *Session) isEmpty() bool {
	return s.UserID == 0 && s.PendingUserID == 0 && s.PoWChallenge == "" &&
		s.CSRF == "" && len(s.Flashes) == 0 && len(s.LinkAuth) == 0
}

// Login records the authenticated user and their current session epoch, mints
// a fresh session id, and rotates the CSRF token, so a token captured before
// login cannot be replayed afterwards. It fails only if the system random
// source does, in which case the session is left logged out.
func (s *Session) Login(userID, epoch int64) error {
	sid, err := randomToken(16)
	if err != nil {
		s.Logout()
		return err
	}
	s.UserID = userID
	s.Epoch = epoch
	s.SID = sid
	s.clearPendingLogin()
	s.CSRF = ""
	s.dirty = true
	return nil
}

// BeginTwoFactor records a short-lived, pre-authentication login step, pinned
// to the user's session epoch at the time the password was verified. Any
// identity the session still carried is dropped, so the pending login is the
// only one and can be completed.
func (s *Session) BeginTwoFactor(userID, epoch int64, next string) {
	s.UserID = 0
	s.Epoch = 0
	s.SID = ""
	s.PendingUserID = userID
	s.PendingEpoch = epoch
	s.PendingNext = next
	s.PendingSince = time.Now().Unix()
	s.dirty = true
}

// PendingLogin is a password-verified login awaiting its second factor.
type PendingLogin struct {
	UserID int64
	Epoch  int64
	Next   string
}

// PendingTwoFactor returns the pending login. Pending challenges expire after
// ten minutes.
func (s *Session) PendingTwoFactor() (PendingLogin, bool) {
	if s.PendingUserID == 0 || s.PendingSince == 0 ||
		time.Since(time.Unix(s.PendingSince, 0)) > 10*time.Minute {
		if s.PendingUserID != 0 {
			s.clearPendingLogin()
			s.dirty = true
		}
		return PendingLogin{}, false
	}
	return PendingLogin{UserID: s.PendingUserID, Epoch: s.PendingEpoch, Next: s.PendingNext}, true
}

func (s *Session) clearPendingLogin() {
	s.PendingUserID = 0
	s.PendingEpoch = 0
	s.PendingNext = ""
	s.PendingSince = 0
}

// SetPoWChallenge replaces the anonymous creation challenge in this session.
func (s *Session) SetPoWChallenge(challenge string) {
	s.PoWChallenge = challenge
	s.dirty = true
}

// ConsumePoWChallenge accepts the current challenge exactly once.
func (s *Session) ConsumePoWChallenge(challenge string) bool {
	if challenge == "" || subtle.ConstantTimeCompare([]byte(challenge), []byte(s.PoWChallenge)) != 1 {
		return false
	}
	s.PoWChallenge = ""
	s.dirty = true
	return true
}

// Logout clears all session state.
func (s *Session) Logout() {
	s.Data = Data{}
	s.LinkAuth = map[string]string{}
	s.dirty = true
}

// AddFlash queues a message for the next rendered page.
func (s *Session) AddFlash(category, message string) {
	s.Flashes = append(s.Flashes, Flash{Category: category, Message: message})
	s.dirty = true
}

// TakeFlashes returns and clears the queued messages.
func (s *Session) TakeFlashes() []Flash {
	if len(s.Flashes) == 0 {
		return nil
	}
	out := s.Flashes
	s.Flashes = nil
	s.dirty = true
	return out
}

// AuthorizeLink records that the visitor entered the correct link password.
// grant identifies the link instance and password that were unlocked (see
// LinkGrant), so the record cannot unlock a different link that later reuses
// the short code, or the same link after its password changes.
func (s *Session) AuthorizeLink(code, grant string) {
	if s.LinkAuth == nil {
		s.LinkAuth = map[string]string{}
	}
	s.LinkAuth[code] = grant
	s.dirty = true
}

// IsLinkAuthorized reports whether this visitor already unlocked the link
// instance identified by grant.
func (s *Session) IsLinkAuthorized(code, grant string) bool {
	have, ok := s.LinkAuth[code]
	return ok && grant != "" && subtle.ConstantTimeCompare([]byte(have), []byte(grant)) == 1
}

// LinkGrant derives the value AuthorizeLink stores for a link: a short digest
// of its id, creation time and current password hash. The id alone is not
// enough — SQLite tables created by the legacy deployment lack AUTOINCREMENT
// and hand a deleted row's id to the next insert — but together with the
// creation time it identifies one link instance. The digest keeps the hash
// itself out of the cookie.
func LinkGrant(linkID int64, createdAt time.Time, passwordHash string) string {
	sum := sha256.Sum256([]byte("redrx.linkauth.v2|" + strconv.FormatInt(linkID, 10) + "|" +
		strconv.FormatInt(createdAt.UnixNano(), 10) + "|" + passwordHash))
	return base64.RawURLEncoding.EncodeToString(sum[:9])
}

// CSRFToken returns the session's CSRF token, minting one on first use.
func (s *Session) CSRFToken() string {
	if s.CSRF == "" {
		token, err := randomToken(32)
		if err != nil {
			return ""
		}
		s.CSRF = token
		s.dirty = true
	}
	return s.CSRF
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ValidCSRF compares a submitted token against the session's in constant time.
func (s *Session) ValidCSRF(token string) bool {
	if s.CSRF == "" || token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(s.CSRF), []byte(token)) == 1
}

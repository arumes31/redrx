package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/arumes31/redrx/internal/security"
	"github.com/arumes31/redrx/internal/shortcode"
	"github.com/arumes31/redrx/internal/store"
)

// shortenRequest is the /api/v1/shorten body. Pointer and json.RawMessage
// fields distinguish "absent" from "explicitly null or false".
type shortenRequest struct {
	LongURL          *string         `json:"long_url"`
	CustomCode       *string         `json:"custom_code"`
	CodeLength       json.RawMessage `json:"code_length"`
	ExpiryHours      json.RawMessage `json:"expiry_hours"`
	StartAt          *string         `json:"start_at"`
	EndAt            *string         `json:"end_at"`
	RotateTargets    json.RawMessage `json:"rotate_targets"`
	IOSTargetURL     *string         `json:"ios_target_url"`
	AndroidTargetURL *string         `json:"android_target_url"`
	Password         *string         `json:"password"`
	PreviewMode      json.RawMessage `json:"preview_mode"`
	StatsEnabled     json.RawMessage `json:"stats_enabled"`
}

// authenticateAPI resolves the X-API-KEY header to a user.
func (s *Server) authenticateAPI(r *http.Request) (*store.User, bool) {
	key := strings.TrimSpace(r.Header.Get("X-API-KEY"))
	if key == "" {
		return nil, false
	}
	user, err := s.db.UserByAPIKey(r.Context(), key)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			s.log.Error("api key lookup", "error", err)
		}
		return nil, false
	}
	return user, true
}

func (s *Server) handleAPIShorten(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticateAPI(r)
	if !ok {
		apiError(w, http.StatusUnauthorized, "Valid API Key required. Access denied.")
		return
	}

	var req shortenRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadSize))
	if err := dec.Decode(&req); err != nil {
		apiError(w, http.StatusBadRequest, "Request payload must be a JSON object")
		return
	}

	if req.LongURL == nil {
		apiError(w, http.StatusBadRequest, "Missing long_url")
		return
	}
	longURL := strings.TrimSpace(*req.LongURL)
	if longURL == "" {
		apiError(w, http.StatusBadRequest, "long_url must be a non-empty string")
		return
	}
	if !s.safety.IsSafeURL(longURL) {
		apiError(w, http.StatusForbidden, "Destination URL is blocked")
		return
	}

	codeLength := s.cfg.ShortCodeLength
	if len(req.CodeLength) > 0 {
		n, err := decodeInt(req.CodeLength)
		if err != nil {
			apiError(w, http.StatusBadRequest, "code_length must be an integer")
			return
		}
		if n < shortcode.MinLength || n > shortcode.MaxLength {
			apiError(w, http.StatusBadRequest, "code_length must be between 3 and 20")
			return
		}
		codeLength = n
	}

	custom := ""
	if req.CustomCode != nil {
		// An empty or whitespace-only custom_code means "no custom code", the
		// same as omitting it, so fall through to a generated one rather than
		// rejecting it. A non-empty value still has to validate.
		if trimmed := strings.TrimSpace(*req.CustomCode); trimmed != "" {
			validated, err := shortcode.ValidateCustom(trimmed)
			if err != nil {
				apiError(w, http.StatusBadRequest, err.Error())
				return
			}
			custom = validated
		}
	}

	code, err := s.resolveShortCode(r, custom, codeLength)
	if err != nil {
		if errors.Is(err, errCodeTaken) {
			apiError(w, http.StatusConflict, "Custom code already taken")
			return
		}
		s.log.Error("allocate short code", "error", err)
		apiError(w, http.StatusInternalServerError, "Could not allocate a short code")
		return
	}

	startAt, err := parseOptionalISO(req.StartAt, "start_at")
	if err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}
	endAt, err := parseOptionalISO(req.EndAt, "end_at")
	if err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}
	if startAt != nil && endAt != nil && !endAt.After(*startAt) {
		apiError(w, http.StatusBadRequest, "Invalid scheduling window: end_at must be after start_at")
		return
	}

	// Anchor the expiry to the start, not to now. Otherwise a link scheduled to
	// begin next week still gets a 24h expiry from creation time and is dead on
	// arrival — expired before it ever starts.
	expiresAt, err := s.apiExpiry(req.ExpiryHours, startAt)
	if err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}
	if expiresAt != nil && startAt != nil && !expiresAt.After(*startAt) {
		apiError(w, http.StatusBadRequest, "Invalid scheduling window: the link would expire before it starts")
		return
	}

	rotateTargets, err := s.apiRotateTargets(req.RotateTargets)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "blocked") {
			status = http.StatusForbidden
		}
		apiError(w, status, err.Error())
		return
	}

	iosURL, err := s.apiTargetURL(req.IOSTargetURL, "ios_target_url")
	if err != nil {
		apiError(w, statusForTargetErr(err), err.Error())
		return
	}
	androidURL, err := s.apiTargetURL(req.AndroidTargetURL, "android_target_url")
	if err != nil {
		apiError(w, statusForTargetErr(err), err.Error())
		return
	}

	previewMode, err := decodeBoolDefault(req.PreviewMode, true)
	if err != nil {
		apiError(w, http.StatusBadRequest, "preview_mode must be a boolean")
		return
	}
	statsEnabled, err := decodeBoolDefault(req.StatsEnabled, true)
	if err != nil {
		apiError(w, http.StatusBadRequest, "stats_enabled must be a boolean")
		return
	}

	// Trim so a whitespace-only password does not create a "protected" link that
	// no one — including the creator — can ever unlock with a meaningful value.
	password := ""
	if req.Password != nil {
		password = strings.TrimSpace(*req.Password)
	}

	link := &store.URL{
		UserID:           &user.ID,
		ShortCode:        code,
		LongURL:          longURL,
		RotateTargets:    rotateTargets,
		IOSTargetURL:     iosURL,
		AndroidTargetURL: androidURL,
		PreviewMode:      previewMode,
		StatsEnabled:     statsEnabled,
		IsEnabled:        true,
		ExpiresAt:        expiresAt,
		StartAt:          startAt,
		EndAt:            endAt,
	}
	if password != "" {
		hash, err := security.GeneratePasswordHash(password)
		if err != nil {
			s.log.Error("hash link password", "error", err)
			apiError(w, http.StatusInternalServerError, "Could not create the link")
			return
		}
		link.PasswordHash = hash
	}

	if err := s.db.CreateURL(r.Context(), link); err != nil {
		// resolveShortCode checked availability a moment ago, but a concurrent
		// request can claim the same code in between. The unique index on
		// urls.short_code settles it, and the loser gets the same conflict it
		// would have got had it checked second.
		if store.IsUniqueViolation(err) {
			apiError(w, http.StatusConflict, "Custom code already taken")
			return
		}
		s.log.Error("create link via api", "error", err)
		apiError(w, http.StatusInternalServerError, "Could not create the link")
		return
	}
	s.metrics.shortened.Inc()

	writeJSON(w, http.StatusCreated, map[string]any{
		"short_code":         code,
		"short_url":          s.cfg.ShortURL(code),
		"long_url":           longURL,
		"rotate_targets":     rotateTargets,
		"ios_target_url":     nullableString(iosURL),
		"android_target_url": nullableString(androidURL),
		"expires_at":         isoAware(expiresAt),
		"start_at":           isoAware(startAt),
		"end_at":             isoAware(endAt),
		"password_protected": password != "",
		"preview_mode":       previewMode,
		"stats_enabled":      statsEnabled,
	})
}

func (s *Server) handleAPIGetURL(w http.ResponseWriter, r *http.Request) {
	user, ok := s.authenticateAPI(r)
	if !ok {
		apiError(w, http.StatusUnauthorized, "Valid API Key required. Access denied.")
		return
	}

	code := shortcode.Normalize(r.PathValue("code"))
	link, err := s.db.URLByShortCode(r.Context(), code)
	if errors.Is(err, store.ErrNotFound) {
		apiError(w, http.StatusNotFound, "URL not found")
		return
	}
	if err != nil {
		s.log.Error("api load link", "code", code, "error", err)
		apiError(w, http.StatusInternalServerError, "Could not load the link")
		return
	}

	// Only the owner may read a link's details. This is a deliberate change from
	// the previous release, which returned any link to any valid key: since the
	// response carries long_url, that let any registered user read the
	// destination of someone else's password-protected link without the
	// password, routing around the gate on the redirect path entirely.
	//
	// 404 rather than 403, so the endpoint does not become an oracle for which
	// short codes exist.
	if link.UserID == nil || *link.UserID != user.ID {
		apiError(w, http.StatusNotFound, "URL not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"short_code":         link.ShortCode,
		"short_url":          s.cfg.ShortURL(link.ShortCode),
		"long_url":           link.LongURL,
		"rotate_targets":     link.RotateTargets,
		"ios_target_url":     nullableString(link.IOSTargetURL),
		"android_target_url": nullableString(link.AndroidTargetURL),
		"preview_mode":       link.PreviewMode,
		"stats_enabled":      link.StatsEnabled,
		"clicks_count":       link.ClicksCount,
		"clicks":             link.ClicksCount,
		"created_at":         pyISOFormat(link.CreatedAt),
		"expires_at":         isoNaive(link.ExpiresAt),
		"start_at":           isoNaive(link.StartAt),
		"end_at":             isoNaive(link.EndAt),
		"active":             link.IsActive(),
	})
}

func (s *Server) apiExpiry(raw json.RawMessage, startAt *time.Time) (*time.Time, error) {
	hours := s.cfg.ExpiryHours
	if len(raw) > 0 {
		n, err := decodeInt(raw)
		if err != nil {
			return nil, errors.New("expiry_hours must be an integer")
		}
		hours = n
	}
	if hours < 0 || hours > 876000 {
		return nil, errors.New("expiry_hours must be between 0 and 876,000 (100 years)")
	}
	if hours == 0 {
		return nil, nil
	}
	// "expires N hours after it becomes live": count from a future start rather
	// than from creation, so a scheduled link is not born already expired.
	base := time.Now().UTC()
	if startAt != nil && startAt.After(base) {
		base = *startAt
	}
	t := base.Add(time.Duration(hours) * time.Hour)
	return &t, nil
}

func (s *Server) apiRotateTargets(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var targets []string
	if err := json.Unmarshal(raw, &targets); err != nil {
		return nil, errors.New("rotate_targets must be a list of strings")
	}
	// The text of these errors is the API response body, reproduced verbatim
	// from the previous release, so it keeps the capitalisation and trailing
	// punctuation that ST1005 would otherwise have us strip.
	if len(targets) > 50 {
		return nil, errors.New("Maximum 50 rotate targets allowed") //nolint:staticcheck // verbatim API message
	}
	for i, t := range targets {
		targets[i] = strings.TrimSpace(t)
		if !s.safety.IsSafeURL(targets[i]) {
			return nil, errors.New("One or more rotate target URLs are blocked or invalid.") //nolint:staticcheck // verbatim API message
		}
	}
	return targets, nil
}

// targetURLError distinguishes a blocked destination (403) from a malformed
// one (400).
type targetURLError struct {
	msg     string
	blocked bool
}

func (e *targetURLError) Error() string { return e.msg }

func statusForTargetErr(err error) int {
	var t *targetURLError
	if errors.As(err, &t) && t.blocked {
		return http.StatusForbidden
	}
	return http.StatusBadRequest
}

func (s *Server) apiTargetURL(value *string, field string) (string, error) {
	if value == nil {
		return "", nil
	}
	v := strings.TrimSpace(*value)
	if v == "" {
		return "", nil
	}
	if !s.safety.IsSafeURL(v) {
		label := "iOS"
		if field == "android_target_url" {
			label = "Android"
		}
		return "", &targetURLError{msg: label + " target URL is blocked or invalid.", blocked: true}
	}
	return v, nil
}

// decodeInt accepts a JSON number or a numeric string, as the previous API did
// via Python's int() coercion.
func decodeInt(raw json.RawMessage) (int, error) {
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		// Atoi, not Sscanf: Sscanf stops at the first non-digit and reports
		// success, so "12abc" became 12 and "3.9" became 3, where the previous
		// API's int() raised and returned 400.
		if parsed, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
			return parsed, nil
		}
	}
	return 0, errors.New("not an integer")
}

// decodeBoolDefault accepts a JSON boolean or the string forms the previous API
// coerced, falling back to def when the field is absent.
func decodeBoolDefault(raw json.RawMessage, def bool) (bool, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return def, nil
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return b, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "true", "1", "yes", "y":
			return true, nil
		case "false", "0", "no", "n":
			return false, nil
		}
	}
	var n float64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n != 0, nil
	}
	return false, errors.New("not a boolean")
}

// parseOptionalISO reads an ISO-8601 timestamp, accepting the trailing Z form.
func parseOptionalISO(value *string, field string) (*time.Time, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	raw := strings.TrimSpace(*value)
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			t = t.UTC()
			return &t, nil
		}
	}
	// Verbatim API message, as above.
	return nil, fmt.Errorf("Invalid %s format. Use ISO 8601", field) //nolint:staticcheck // verbatim API message
}

// isoNaive and isoAware reproduce the two timestamp shapes the previous API
// emitted. Python's datetime.isoformat() rendered the values freshly computed
// during a create as timezone-aware ("...+00:00"), and the values read back
// from the database as naive. Integrations are written against those exact
// strings, so each endpoint keeps the form it always had.
func isoNaive(t *time.Time) any {
	if t == nil {
		return nil
	}
	return pyISOFormat(t.UTC())
}

func isoAware(t *time.Time) any {
	if t == nil {
		return nil
	}
	return pyISOFormat(t.UTC()) + "+00:00"
}

// pyISOFormat matches datetime.isoformat(): six fractional digits when there
// are microseconds, and no fractional part at all when there are none.
func pyISOFormat(t time.Time) string {
	if t.Nanosecond() == 0 {
		return t.Format("2006-01-02T15:04:05")
	}
	return t.Format("2006-01-02T15:04:05.000000")
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

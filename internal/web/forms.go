package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/arumes31/redrx/internal/shortcode"
)

// maxExpiryHours is 100 years, the ceiling the previous forms enforced.
const maxExpiryHours = 876000

// errorMap collects per-field validation messages for redisplay.
type errorMap map[string]string

func (e errorMap) add(field, msg string) {
	if _, exists := e[field]; !exists {
		e[field] = msg
	}
}

func (e errorMap) any() bool { return len(e) > 0 }

// Get returns the message for a field, for template use.
func (e errorMap) Get(field string) string { return e[field] }

// Any reports whether the form failed validation, for templates.
func (e errorMap) Any() bool { return len(e) > 0 }

// advancedFields are the ones rendered inside the collapsed "Advanced Options"
// section on the create form.
var advancedFields = []string{
	"custom_code", "code_length", "rotate_targets",
	"ios_target_url", "android_target_url", "logo_file",
	"start_date", "end_date",
}

// AnyAdvanced reports whether an error landed in the collapsed section. The
// template uses it to expand that section on re-render — otherwise the only
// message is hidden and the form looks as though pressing submit did nothing.
func (e errorMap) AnyAdvanced() bool {
	for _, f := range advancedFields {
		if _, ok := e[f]; ok {
			return true
		}
	}
	return false
}

// ShortenForm backs the home page's create-link form. Fields hold the raw
// submitted strings so an invalid submission can be re-rendered as typed.
type ShortenForm struct {
	LongURL          string
	PreviewMode      bool
	StatsEnabled     bool
	CustomCode       string
	CodeLength       string
	RotateTargets    string
	IOSTargetURL     string
	AndroidTargetURL string
	Password         string
	ExpiryHours      string
	StartDate        string
	StartTime        string
	EndDate          string
	EndTime          string
	QRColor          string
	QRBg             string
	Errors           errorMap
}

// newShortenForm returns the form in its initial, unsubmitted state.
func newShortenForm(defaultLength int, expiryHours int, qrColor, qrBg string) *ShortenForm {
	return &ShortenForm{
		PreviewMode:  true,
		StatsEnabled: true,
		CodeLength:   strconv.Itoa(defaultLength),
		ExpiryHours:  strconv.Itoa(expiryHours),
		QRColor:      normalizeHexColor(qrColor, "#000000"),
		QRBg:         normalizeHexColor(qrBg, "#ffffff"),
		Errors:       errorMap{},
	}
}

func bindShortenForm(r *http.Request) *ShortenForm {
	return &ShortenForm{
		LongURL:          strings.TrimSpace(r.FormValue("long_url")),
		PreviewMode:      checkboxChecked(r, "preview_mode"),
		StatsEnabled:     checkboxChecked(r, "stats_enabled"),
		CustomCode:       strings.TrimSpace(r.FormValue("custom_code")),
		CodeLength:       strings.TrimSpace(r.FormValue("code_length")),
		RotateTargets:    strings.TrimSpace(r.FormValue("rotate_targets")),
		IOSTargetURL:     strings.TrimSpace(r.FormValue("ios_target_url")),
		AndroidTargetURL: strings.TrimSpace(r.FormValue("android_target_url")),
		Password:         r.FormValue("password"),
		ExpiryHours:      strings.TrimSpace(r.FormValue("expiry_hours")),
		StartDate:        strings.TrimSpace(r.FormValue("start_date")),
		StartTime:        strings.TrimSpace(r.FormValue("start_time")),
		EndDate:          strings.TrimSpace(r.FormValue("end_date")),
		EndTime:          strings.TrimSpace(r.FormValue("end_time")),
		QRColor:          normalizeHexColor(r.FormValue("qr_color"), "#000000"),
		QRBg:             normalizeHexColor(r.FormValue("qr_bg"), "#ffffff"),
		Errors:           errorMap{},
	}
}

// shortenInput is the validated result of a ShortenForm submission.
type shortenInput struct {
	LongURL          string
	CustomCode       string
	CodeLength       int
	RotateTargets    []string
	IOSTargetURL     string
	AndroidTargetURL string
	Password         string
	PreviewMode      bool
	StatsEnabled     bool
	ExpiresAt        *time.Time
	StartAt          *time.Time
	EndAt            *time.Time
	// ExpirySetByUser distinguishes "0 hours, meaning never" from "not given".
	ExpiryNever bool
}

// Validate checks the form. loggedIn relaxes the expiry ceiling, matching the
// previous rule that only authenticated users may create links lasting more
// than a year or never expiring.
func (f *ShortenForm) Validate(defaultLength int, loggedIn bool) (*shortenInput, bool) {
	f.Errors = errorMap{}
	in := &shortenInput{
		LongURL:      f.LongURL,
		PreviewMode:  f.PreviewMode,
		StatsEnabled: f.StatsEnabled,
	}

	if f.LongURL == "" {
		f.Errors.add("long_url", "This field is required.")
	} else if !isHTTPURL(f.LongURL) {
		f.Errors.add("long_url", "Invalid URL")
	}

	if f.CustomCode != "" {
		code, err := shortcode.ValidateCustom(f.CustomCode)
		if err != nil {
			// Use the validator's own message: it distinguishes a length problem
			// from an invalid character, where the fixed string claimed "3 and
			// 20 characters" even for a 5-character code containing a space.
			f.Errors.add("custom_code", err.Error())
		} else {
			in.CustomCode = code
		}
	}

	in.CodeLength = defaultLength
	if f.CodeLength != "" {
		n, err := strconv.Atoi(f.CodeLength)
		if err != nil || n < shortcode.MinLength || n > shortcode.MaxLength {
			f.Errors.add("code_length", "Number must be between 3 and 20.")
		} else {
			in.CodeLength = n
		}
	}

	in.RotateTargets = splitList(f.RotateTargets)
	if len(in.RotateTargets) > 50 {
		f.Errors.add("rotate_targets", "Maximum 50 rotate targets allowed.")
	}
	for _, t := range in.RotateTargets {
		if !isHTTPURL(t) {
			f.Errors.add("rotate_targets", "One or more rotate target URLs are invalid.")
			break
		}
	}

	if f.IOSTargetURL != "" {
		if !isHTTPURL(f.IOSTargetURL) {
			f.Errors.add("ios_target_url", "Invalid URL")
		} else {
			in.IOSTargetURL = f.IOSTargetURL
		}
	}
	if f.AndroidTargetURL != "" {
		if !isHTTPURL(f.AndroidTargetURL) {
			f.Errors.add("android_target_url", "Invalid URL")
		} else {
			in.AndroidTargetURL = f.AndroidTargetURL
		}
	}

	// A whitespace-only password would hash to a "protected" link nobody can
	// unlock; treat it as no password.
	in.Password = strings.TrimSpace(f.Password)

	start, startOK := parseDateTime(f.StartDate, f.StartTime)
	if !startOK {
		f.Errors.add("start_date", "Invalid start date or time.")
	}
	end, endOK := parseDateTime(f.EndDate, f.EndTime)
	if !endOK {
		f.Errors.add("end_date", "Invalid end date or time.")
	}
	if start != nil && end != nil && !end.After(*start) {
		f.Errors.add("end_date", "End time must be after start time")
	}
	in.StartAt, in.EndAt = start, end

	if f.ExpiryHours != "" {
		hours, err := strconv.Atoi(f.ExpiryHours)
		switch {
		case err != nil || hours < 0 || hours > maxExpiryHours:
			f.Errors.add("expiry_hours", "Maximum expiry is 100 years (876,000 hours).")
		case hours == 0:
			if !loggedIn {
				f.Errors.add("expiry_hours", "Please log in to create permanent links.")
			}
			in.ExpiryNever = true
		case hours > 8760 && !loggedIn:
			f.Errors.add("expiry_hours", "Please log in to create links longer than 1 year.")
		default:
			// Anchor to a future start, so "expires in N hours" counts from when
			// the link goes live, not from creation — otherwise a link scheduled
			// for next week is born already expired.
			base := time.Now().UTC()
			if in.StartAt != nil && in.StartAt.After(base) {
				base = *in.StartAt
			}
			t := base.Add(time.Duration(hours) * time.Hour)
			in.ExpiresAt = &t
		}
	}

	if in.ExpiresAt != nil && in.StartAt != nil && !in.ExpiresAt.After(*in.StartAt) {
		f.Errors.add("expiry_hours", "The link would expire before it starts. Increase the expiry or clear the start time.")
	}

	return in, !f.Errors.any()
}

// EditForm backs the per-link edit page.
type EditForm struct {
	LongURL          string
	IOSTargetURL     string
	AndroidTargetURL string
	ExpiryHours      string
	PreviewMode      bool
	StatsEnabled     bool
	Errors           errorMap
}

func bindEditForm(r *http.Request) *EditForm {
	return &EditForm{
		LongURL:          strings.TrimSpace(r.FormValue("long_url")),
		IOSTargetURL:     strings.TrimSpace(r.FormValue("ios_target_url")),
		AndroidTargetURL: strings.TrimSpace(r.FormValue("android_target_url")),
		ExpiryHours:      strings.TrimSpace(r.FormValue("expiry_hours")),
		PreviewMode:      checkboxChecked(r, "preview_mode"),
		StatsEnabled:     checkboxChecked(r, "stats_enabled"),
		Errors:           errorMap{},
	}
}

// editInput is the validated result of an EditForm submission.
type editInput struct {
	LongURL          string
	IOSTargetURL     string
	AndroidTargetURL string
	PreviewMode      bool
	StatsEnabled     bool
	ExpiresAt        *time.Time
	ExpiryGiven      bool
	ExpiryNever      bool
}

func (f *EditForm) Validate() (*editInput, bool) {
	f.Errors = errorMap{}
	in := &editInput{
		LongURL:      f.LongURL,
		PreviewMode:  f.PreviewMode,
		StatsEnabled: f.StatsEnabled,
	}

	if f.LongURL == "" {
		f.Errors.add("long_url", "This field is required.")
	} else if !isHTTPURL(f.LongURL) {
		f.Errors.add("long_url", "Invalid URL")
	}
	if f.IOSTargetURL != "" {
		if !isHTTPURL(f.IOSTargetURL) {
			f.Errors.add("ios_target_url", "Invalid URL")
		} else {
			in.IOSTargetURL = f.IOSTargetURL
		}
	}
	if f.AndroidTargetURL != "" {
		if !isHTTPURL(f.AndroidTargetURL) {
			f.Errors.add("android_target_url", "Invalid URL")
		} else {
			in.AndroidTargetURL = f.AndroidTargetURL
		}
	}

	if f.ExpiryHours != "" {
		hours, err := strconv.Atoi(f.ExpiryHours)
		switch {
		case err != nil || hours < 0 || hours > maxExpiryHours:
			f.Errors.add("expiry_hours", "Maximum expiry is 100 years (876,000 hours).")
		case hours == 0:
			in.ExpiryGiven, in.ExpiryNever = true, true
		default:
			t := time.Now().UTC().Add(time.Duration(hours) * time.Hour)
			in.ExpiresAt, in.ExpiryGiven = &t, true
		}
	}

	return in, !f.Errors.any()
}

// LoginForm backs the account login page.
type LoginForm struct {
	Username string
	Errors   errorMap
}

// RegisterForm backs the account registration page.
type RegisterForm struct {
	Username string
	Email    string
	Errors   errorMap
}

func (f *RegisterForm) Validate(password string) bool {
	f.Errors = errorMap{}
	if n := len([]rune(f.Username)); n < 4 || n > 20 {
		f.Errors.add("username", "Field must be between 4 and 20 characters long.")
	}
	if !looksLikeEmail(f.Email) {
		f.Errors.add("email", "Invalid email address.")
	}
	if len(password) < 6 {
		f.Errors.add("password", "Field must be at least 6 characters long.")
	}
	return !f.Errors.any()
}

// checkboxChecked reports whether an HTML checkbox was submitted. Unchecked
// boxes are omitted from the body entirely.
func checkboxChecked(r *http.Request, name string) bool {
	v := r.FormValue(name)
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

// isHTTPURL applies the same rule the old WTForms URL validator did: an
// absolute http(s) URL with a dotted host.
func isHTTPURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return false
	}
	host := u.Hostname()
	if host == "" {
		return false
	}
	// Require a TLD, except for localhost which is useful in development.
	return strings.Contains(host, ".") || strings.EqualFold(host, "localhost")
}

func looksLikeEmail(s string) bool {
	s = strings.TrimSpace(s)
	at := strings.LastIndex(s, "@")
	if at <= 0 || at == len(s)-1 || strings.ContainsAny(s, " \t\r\n") {
		return false
	}
	domain := s[at+1:]
	return strings.Contains(domain, ".") && !strings.HasPrefix(domain, ".") && !strings.HasSuffix(domain, ".")
}

func splitList(raw string) []string {
	var out []string
	for _, p := range strings.Split(raw, ",") {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// parseDateTime combines the separate date and time inputs. It reports false
// only when a value was supplied but could not be parsed; both empty yields
// (nil, true).
func parseDateTime(date, clock string) (*time.Time, bool) {
	if date == "" && clock == "" {
		return nil, true
	}
	if date == "" || clock == "" {
		// One half without the other was silently ignored before; keep that.
		return nil, true
	}
	if len(clock) == 5 {
		clock += ":00"
	}
	t, err := time.Parse("2006-01-02 15:04:05", date+" "+clock)
	if err != nil {
		return nil, false
	}
	t = t.UTC()
	return &t, true
}

// normalizeHexColor keeps colour inputs to the "#rrggbb" form the colour picker
// emits, falling back when a value is missing or malformed.
// namedColors maps the basic CSS colour names to hex, enough to cover any
// value an operator is likely to set for DEFAULT_QR_COLOR / DEFAULT_QR_BACKGROUND.
var namedColors = map[string]string{
	"black": "#000000", "white": "#ffffff", "red": "#ff0000", "green": "#008000",
	"blue": "#0000ff", "yellow": "#ffff00", "cyan": "#00ffff", "magenta": "#ff00ff",
	"gray": "#808080", "grey": "#808080", "silver": "#c0c0c0", "maroon": "#800000",
	"olive": "#808000", "lime": "#00ff00", "teal": "#008080", "navy": "#000080",
	"purple": "#800080", "orange": "#ffa500",
}

func normalizeHexColor(v, def string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return def
	}
	if !strings.HasPrefix(v, "#") {
		// Convert a named colour to hex. Passing the name through breaks the
		// HTML <input type="color"> that renders this value — it accepts only
		// #rrggbb and silently coerces anything else to #000000, so the shipped
		// "white" background became black and every default QR came out solid
		// black and unscannable.
		if hex, ok := namedColors[strings.ToLower(v)]; ok {
			return hex
		}
		return def
	}
	hex := v[1:]
	if len(hex) != 3 && len(hex) != 6 {
		return def
	}
	for _, c := range hex {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return def
		}
	}
	return strings.ToLower(v)
}

// sanitizeCSVField neutralises spreadsheet formula injection in exports.
func sanitizeCSVField(v any) string {
	s := fmt.Sprint(v)
	if s == "" {
		return ""
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}

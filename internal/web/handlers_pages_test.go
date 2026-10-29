package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSafeRedirect(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   bool
		canon  string
	}{
		{name: "root", target: "/", want: true, canon: "/"},
		{name: "same-site path", target: "/dashboard?tab=links#recent", want: true, canon: "/dashboard?tab=links#recent"},
		{name: "dot segments are cleaned", target: "/a/../dashboard/", want: true, canon: "/dashboard/"},
		{name: "empty", target: "", want: false},
		{name: "relative path", target: "dashboard", want: false},
		{name: "absolute URL", target: "https://attacker.example/", want: false},
		{name: "protocol-relative URL", target: "//attacker.example/", want: false},
		{name: "backslash authority", target: `/\attacker.example/`, want: false},
		{name: "backslash after dot segments", target: `/x/../\attacker.example/`, want: false},
		{name: "backslash anywhere", target: `/dashboard\x`, want: false},
		{name: "control character", target: "/\nattacker.example/", want: false},
		{name: "tab", target: "/\t/attacker.example/", want: false},
		{name: "doubled slash after cleaning", target: "/./attacker.example", want: true, canon: "/attacker.example"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := safeRedirect(tt.target)
			if ok != tt.want {
				t.Fatalf("safeRedirect(%q) ok = %t, want %t", tt.target, ok, tt.want)
			}
			if ok && got != tt.canon {
				t.Errorf("safeRedirect(%q) = %q, want %q", tt.target, got, tt.canon)
			}
		})
	}
}

// TestSafeRedirectMatchesHTTPRedirect checks that the value validated is the
// value http.Redirect emits, so its normalisation cannot turn an accepted path
// into an external one.
func TestSafeRedirectMatchesHTTPRedirect(t *testing.T) {
	for _, target := range []string{"/a/../b", "/a/./b/", "/x/..//y?q=1", "/dashboard?tab=links"} {
		canon, ok := safeRedirect(target)
		if !ok {
			t.Fatalf("safeRedirect(%q) rejected a same-site path", target)
		}
		rec := httptest.NewRecorder()
		http.Redirect(rec, httptest.NewRequest(http.MethodGet, "/login", nil), canon, http.StatusSeeOther)
		if loc := rec.Header().Get("Location"); loc != canon {
			t.Errorf("http.Redirect(%q) emitted %q", canon, loc)
		}
	}
}

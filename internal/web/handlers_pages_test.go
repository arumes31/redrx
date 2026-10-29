package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsSafeRedirect(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{name: "root", target: "/", want: true},
		{name: "same-site path", target: "/dashboard?tab=links#recent", want: true},
		{name: "trailing slash", target: "/dashboard/", want: true},
		{name: "empty", target: "", want: false},
		{name: "relative path", target: "dashboard", want: false},
		{name: "absolute URL", target: "https://attacker.example/", want: false},
		{name: "protocol-relative URL", target: "//attacker.example/", want: false},
		{name: "backslash authority", target: `/\attacker.example/`, want: false},
		{name: "backslash after dot segments", target: `/x/../\attacker.example/`, want: false},
		{name: "backslash anywhere", target: `/dashboard\x`, want: false},
		{name: "control character", target: "/\nattacker.example/", want: false},
		{name: "tab", target: "/\t/attacker.example/", want: false},
		// Non-canonical paths are rewritten by http.Redirect, so they are refused
		// rather than validated in one form and sent in another.
		{name: "dot segments", target: "/a/../dashboard", want: false},
		{name: "doubled slash inside", target: "/./attacker.example", want: false},
		{name: "collapsed slashes", target: "/a//b", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSafeRedirect(tt.target); got != tt.want {
				t.Errorf("isSafeRedirect(%q) = %t, want %t", tt.target, got, tt.want)
			}
		})
	}
}

// TestSafeRedirectIsSentUnchanged checks that every accepted target is emitted
// by http.Redirect exactly as validated, so its normalisation cannot turn an
// accepted path into an external one.
func TestSafeRedirectIsSentUnchanged(t *testing.T) {
	for _, target := range []string{"/", "/dashboard", "/a/b/", "/dashboard?tab=links", "/x?next=/../y"} {
		if !isSafeRedirect(target) {
			t.Fatalf("isSafeRedirect(%q) rejected a canonical same-site path", target)
		}
		rec := httptest.NewRecorder()
		http.Redirect(rec, httptest.NewRequest(http.MethodGet, "/login", nil), target, http.StatusSeeOther)
		if loc := rec.Header().Get("Location"); loc != target {
			t.Errorf("http.Redirect(%q) emitted %q", target, loc)
		}
	}
}

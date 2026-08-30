package web

import "testing"

func TestIsSafeRedirect(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{name: "root", target: "/", want: true},
		{name: "same-site path", target: "/dashboard?tab=links#recent", want: true},
		{name: "empty", target: "", want: false},
		{name: "relative path", target: "dashboard", want: false},
		{name: "absolute URL", target: "https://attacker.example/", want: false},
		{name: "protocol-relative URL", target: "//attacker.example/", want: false},
		{name: "backslash authority", target: `/\attacker.example/`, want: false},
		{name: "control character", target: "/\nattacker.example/", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSafeRedirect(tt.target); got != tt.want {
				t.Errorf("isSafeRedirect(%q) = %t, want %t", tt.target, got, tt.want)
			}
		})
	}
}

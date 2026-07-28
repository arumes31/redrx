package main

import "testing"

func TestMaskIPs(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// IPv4.
		{"from 203.0.113.7 ok", "from " + maskedIP + " ok"},
		{"dial tcp 10.0.0.1:5432: refused", "dial tcp " + maskedIP + ":5432: refused"},
		// IPv6, full and compressed, including the bracketed RemoteAddr form.
		{"peer 2001:db8:85a3:0:0:8a2e:370:7334", "peer " + maskedIP},
		{"peer 2001:db8::8a2e:370:7334", "peer " + maskedIP},
		{"[::1]:54321", "[" + maskedIP + "]:54321"},
		{"loopback ::1 seen", "loopback " + maskedIP + " seen"},
		// A wall-clock time is three groups with no "::", so it must be left alone.
		{"took until 15:04:05 today", "took until 15:04:05 today"},
		// No address at all.
		{"nothing to mask here", "nothing to mask here"},
	}
	for _, c := range cases {
		if got := maskIPs(c.in); got != c.want {
			t.Errorf("maskIPs(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

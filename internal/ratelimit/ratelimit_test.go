package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestParseFlaskLimiterSyntax(t *testing.T) {
	cases := []struct {
		spec string
		want Rules
	}{
		{"10 per minute", Rules{{10, time.Minute}}},
		{"200 per day;50 per hour", Rules{{200, 24 * time.Hour}, {50, time.Hour}}},
		{"5/hour", Rules{{5, time.Hour}}},
		{"1 per second", Rules{{1, time.Second}}},
		{"100 per 5 minutes", Rules{{100, 5 * time.Minute}}},
		{"60 per minute, 1000 per day", Rules{{60, time.Minute}, {1000, 24 * time.Hour}}},
		{"", nil},
	}
	for _, tc := range cases {
		got, err := Parse(tc.spec)
		if err != nil {
			t.Errorf("Parse(%q): %v", tc.spec, err)
			continue
		}
		if len(got) != len(tc.want) {
			t.Errorf("Parse(%q) = %v, want %v", tc.spec, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("Parse(%q)[%d] = %v, want %v", tc.spec, i, got[i], tc.want[i])
			}
		}
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	for _, spec := range []string{"nonsense", "10 per fortnight", "0 per minute", "-3 per hour", "per minute"} {
		if _, err := Parse(spec); err == nil {
			t.Errorf("Parse(%q) accepted an invalid spec", spec)
		}
	}
}

func TestAllowEnforcesLimit(t *testing.T) {
	backend := NewMemoryBackend()
	defer backend.Close()
	l := New(backend)
	rules := Rules{{Limit: 3, Window: time.Hour}}
	ctx := context.Background()

	for i := 1; i <= 3; i++ {
		if res := l.Allow(ctx, "create", "1.2.3.4", rules); !res.Allowed {
			t.Fatalf("request %d was blocked but is within the limit of 3", i)
		}
	}
	res := l.Allow(ctx, "create", "1.2.3.4", rules)
	if res.Allowed {
		t.Error("the 4th request was allowed but exceeds the limit of 3")
	}
	if res.RetryAfter <= 0 {
		t.Errorf("RetryAfter = %v, want a positive duration", res.RetryAfter)
	}
}

func TestAllowIsolatesIdentitiesAndScopes(t *testing.T) {
	backend := NewMemoryBackend()
	defer backend.Close()
	l := New(backend)
	rules := Rules{{Limit: 1, Window: time.Hour}}
	ctx := context.Background()

	l.Allow(ctx, "create", "1.1.1.1", rules)
	if res := l.Allow(ctx, "create", "1.1.1.1", rules); res.Allowed {
		t.Error("same identity and scope should be limited")
	}
	if res := l.Allow(ctx, "create", "2.2.2.2", rules); !res.Allowed {
		t.Error("a different client IP must have its own budget")
	}
	if res := l.Allow(ctx, "login", "1.1.1.1", rules); !res.Allowed {
		t.Error("a different scope must have its own budget")
	}
}

func TestNoRulesMeansUnlimited(t *testing.T) {
	backend := NewMemoryBackend()
	defer backend.Close()
	l := New(backend)
	for i := 0; i < 50; i++ {
		if res := l.Allow(context.Background(), "any", "ip", nil); !res.Allowed {
			t.Fatal("an empty rule set must not limit anything")
		}
	}
}

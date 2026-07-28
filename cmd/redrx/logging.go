package main

import (
	"context"
	"log/slog"
	"net"
	"regexp"
)

// ipv4Pattern matches dotted-quad addresses in log output.
var ipv4Pattern = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)

// ipv6Candidate loosely matches colon-separated hex runs (optionally with a
// trailing embedded IPv4). It over-matches on purpose — hand-ordering a precise
// IPv6 alternation is fragile under Go's leftmost-first matching — so every hit
// is confirmed with net.ParseIP before masking. That validation is also what
// keeps a timestamp like "15:04:05", which fits the shape but is not an address,
// untouched.
var ipv6Candidate = regexp.MustCompile(`(?i)[0-9a-f]{0,4}(?::[0-9a-f]{0,4}){2,7}(?:\.[0-9]{1,3}){0,3}`)

// maskedIP is what a matched address is replaced with.
const maskedIP = "xxx.xxx.xxx.xxx"

// maskIPs redacts any IPv6 or IPv4 address in s, so both logging paths anonymise
// either format the same way. IPv6 runs first so an embedded-IPv4 form like
// "::ffff:203.0.113.7" is masked whole instead of leaving the wrapper behind.
func maskIPs(s string) string {
	s = ipv6Candidate.ReplaceAllStringFunc(s, func(m string) string {
		if net.ParseIP(m) != nil {
			return maskedIP
		}
		return m
	})
	return ipv4Pattern.ReplaceAllString(s, maskedIP)
}

// anonymizingHandler masks IPv4 addresses in log messages and string
// attributes, honouring ANONYMIZE_LOGS as the previous logging filter did.
type anonymizingHandler struct {
	inner slog.Handler
}

func (h anonymizingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h anonymizingHandler) Handle(ctx context.Context, rec slog.Record) error {
	masked := slog.NewRecord(rec.Time, rec.Level, maskIPs(rec.Message), rec.PC)
	rec.Attrs(func(a slog.Attr) bool {
		masked.AddAttrs(maskAttr(a))
		return true
	})
	return h.inner.Handle(ctx, masked)
}

func (h anonymizingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		out[i] = maskAttr(a)
	}
	return anonymizingHandler{h.inner.WithAttrs(out)}
}

func (h anonymizingHandler) WithGroup(name string) slog.Handler {
	return anonymizingHandler{h.inner.WithGroup(name)}
}

func maskAttr(a slog.Attr) slog.Attr {
	switch a.Value.Kind() {
	case slog.KindString:
		return slog.String(a.Key, maskIPs(a.Value.String()))
	case slog.KindAny:
		// The most common way an IP reaches a log is inside an error value —
		// "dial tcp 10.0.0.1:5432: connection refused" — which is KindAny, not
		// KindString. Mask its rendered text. Other non-string values are left
		// as they are so their type survives to the handler.
		if _, ok := a.Value.Any().(error); ok {
			return slog.String(a.Key, maskIPs(a.Value.String()))
		}
		return a
	case slog.KindGroup:
		// Recurse so a nested string member is masked as a top-level one is.
		members := a.Value.Group()
		masked := make([]slog.Attr, len(members))
		for i, m := range members {
			masked[i] = maskAttr(m)
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(masked...)}
	default:
		return a
	}
}

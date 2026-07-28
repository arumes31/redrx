package main

import (
	"context"
	"log/slog"
	"regexp"
)

// ipv4Pattern matches dotted-quad addresses in log output.
var ipv4Pattern = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)

// maskedIP is what a matched address is replaced with.
const maskedIP = "xxx.xxx.xxx.xxx"

// anonymizingHandler masks IPv4 addresses in log messages and string
// attributes, honouring ANONYMIZE_LOGS as the previous logging filter did.
type anonymizingHandler struct {
	inner slog.Handler
}

func (h anonymizingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h anonymizingHandler) Handle(ctx context.Context, rec slog.Record) error {
	masked := slog.NewRecord(rec.Time, rec.Level, ipv4Pattern.ReplaceAllString(rec.Message, maskedIP), rec.PC)
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
		return slog.String(a.Key, ipv4Pattern.ReplaceAllString(a.Value.String(), maskedIP))
	case slog.KindAny:
		// The most common way an IP reaches a log is inside an error value —
		// "dial tcp 10.0.0.1:5432: connection refused" — which is KindAny, not
		// KindString. Mask its rendered text. Other non-string values are left
		// as they are so their type survives to the handler.
		if _, ok := a.Value.Any().(error); ok {
			return slog.String(a.Key, ipv4Pattern.ReplaceAllString(a.Value.String(), maskedIP))
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

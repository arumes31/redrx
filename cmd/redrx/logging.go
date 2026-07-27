package main

import (
	"context"
	"log/slog"
	"regexp"
)

// ipv4Pattern matches dotted-quad addresses in log output.
var ipv4Pattern = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)

// anonymizingHandler masks IPv4 addresses in log messages and string
// attributes, honouring ANONYMIZE_LOGS as the previous logging filter did.
type anonymizingHandler struct {
	inner slog.Handler
}

func (h anonymizingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h anonymizingHandler) Handle(ctx context.Context, rec slog.Record) error {
	masked := slog.NewRecord(rec.Time, rec.Level, ipv4Pattern.ReplaceAllString(rec.Message, "xxx.xxx.xxx.xxx"), rec.PC)
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
	if a.Value.Kind() == slog.KindString {
		return slog.String(a.Key, ipv4Pattern.ReplaceAllString(a.Value.String(), "xxx.xxx.xxx.xxx"))
	}
	return a
}

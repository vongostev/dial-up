/*
[2026-07-02] :: 🚀 :: Initial slog adapter
*/

package logger

import (
	"context"
	"log/slog"
	"os"
	"time"
)

type slogLogger struct {
	inner *slog.Logger
}

// New creates a Logger backed by slog, writing JSON to stderr.
func New(debug bool) Logger {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: level}
	handler := slog.NewJSONHandler(os.Stderr, opts)
	return &slogLogger{inner: slog.New(handler)}
}

func (l *slogLogger) Debug(pkg, msg string, fields ...Field) {
	l.inner.LogAttrs(context.Background(), slog.LevelDebug, msg, toAttrs(pkg, fields)...)
}

func (l *slogLogger) Info(pkg, msg string, fields ...Field) {
	l.inner.LogAttrs(context.Background(), slog.LevelInfo, msg, toAttrs(pkg, fields)...)
}

func (l *slogLogger) Warn(pkg, msg string, fields ...Field) {
	l.inner.LogAttrs(context.Background(), slog.LevelWarn, msg, toAttrs(pkg, fields)...)
}

func (l *slogLogger) Error(pkg, msg string, fields ...Field) {
	l.inner.LogAttrs(context.Background(), slog.LevelError, msg, toAttrs(pkg, fields)...)
}

func (l *slogLogger) With(fields ...Field) Logger {
	return &slogLogger{inner: l.inner.With(toArgs(toAttrs("", fields))...)}
}

func toAttrs(pkg string, fields []Field) []slog.Attr {
	attrs := make([]slog.Attr, 0, len(fields)+1)
	if pkg != "" {
		attrs = append(attrs, slog.String("pkg", pkg))
	}
	for _, f := range fields {
		switch v := f.Value.(type) {
		case string:
			attrs = append(attrs, slog.String(f.Key, v))
		case int:
			attrs = append(attrs, slog.Int(f.Key, v))
		case bool:
			attrs = append(attrs, slog.Bool(f.Key, v))
		case error:
			attrs = append(attrs, slog.String(f.Key, v.Error()))
		case time.Time:
			attrs = append(attrs, slog.Time(f.Key, v))
		default:
			attrs = append(attrs, slog.Any(f.Key, v))
		}
	}
	return attrs
}

func toArgs(attrs []slog.Attr) []any {
	args := make([]any, 0, len(attrs)*2)
	for _, a := range attrs {
		args = append(args, a.Key, a.Value)
	}
	return args
}

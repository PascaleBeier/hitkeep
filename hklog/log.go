package hklog

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"log/slog"
	"strings"
)

// Writer is an io.Writer that forwards lines to slog at a fixed level.
type Writer struct {
	Logger *slog.Logger
	Level  slog.Level
}

type loggerContextKey struct{}

// WithLogger attaches a logger to ctx for request or job-scoped logging.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if logger == nil {
		logger = slog.Default()
	}
	return context.WithValue(ctx, loggerContextKey{}, logger)
}

// WithLoggerIfAbsent preserves an existing context logger and otherwise adds logger.
func WithLoggerIfAbsent(ctx context.Context, logger *slog.Logger) context.Context {
	if ctx != nil {
		if existing, ok := ctx.Value(loggerContextKey{}).(*slog.Logger); ok && existing != nil {
			return ctx
		}
	}
	return WithLogger(ctx, logger)
}

// WithLoggerAttrs derives the context logger with additional structured fields.
func WithLoggerAttrs(ctx context.Context, args ...any) context.Context {
	return WithLogger(ctx, LoggerFromContext(ctx).With(args...))
}

// LoggerFromContext returns the context logger or the default logger when none is attached.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	return LoggerFromContextOr(ctx, nil)
}

// LoggerFromContextOr returns the context logger, the supplied fallback, or
// the default logger when neither is available. Receivers with an injected
// logger should use this instead of silently losing that dependency when a
// caller supplies a plain context.
func LoggerFromContextOr(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if ctx != nil {
		if logger, ok := ctx.Value(loggerContextKey{}).(*slog.Logger); ok && logger != nil {
			return logger
		}
	}
	if fallback != nil {
		return fallback
	}
	return slog.Default()
}

func (w Writer) Write(p []byte) (int, error) {
	msg := string(bytes.TrimRight(p, "\n"))
	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Log(context.Background(), w.Level, msg)
	return len(p), nil
}

// LevelParsingWriter forwards log lines to slog, inferring level from a prefix.
type LevelParsingWriter struct {
	Logger        *slog.Logger
	DefaultLevel  slog.Level
	ComponentName string
}

func (w LevelParsingWriter) Write(p []byte) (int, error) {
	msg := string(bytes.TrimRight(p, "\n"))
	level := w.DefaultLevel

	trimmed := strings.TrimSpace(msg)
	level, trimmed = parseLevelPrefix(level, trimmed)
	if after, ok := strings.CutPrefix(trimmed, "memberlist:"); ok {
		trimmed = strings.TrimSpace(after)
	}

	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if w.ComponentName != "" {
		logger = logger.With("component", w.ComponentName)
	}
	logger.Log(context.Background(), level, trimmed)
	return len(p), nil
}

// StdLogger returns a *log.Logger that writes to the provided slog.Logger at the given level.
func StdLogger(l *slog.Logger, level slog.Level) *log.Logger {
	if l == nil {
		l = slog.Default()
	}
	return log.New(Writer{Logger: l, Level: level}, "", 0)
}

// MemberlistLogger returns a *log.Logger that maps memberlist log levels to slog.
func MemberlistLogger(l *slog.Logger) *log.Logger {
	if l == nil {
		l = slog.Default()
	}
	return log.New(LevelParsingWriter{
		Logger:        l,
		DefaultLevel:  slog.LevelInfo,
		ComponentName: "memberlist",
	}, "", 0)
}

// ParseLevel parses a user-provided string into a slog.Level.
// Accepts: debug, info, warn, warning, error. Case-insensitive.
func ParseLevel(s string) (slog.Level, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown log level: %q", s)
	}
}

func parseLevelPrefix(defaultLevel slog.Level, msg string) (slog.Level, string) {
	switch {
	case strings.HasPrefix(msg, "[DEBUG]"):
		return slog.LevelDebug, strings.TrimSpace(strings.TrimPrefix(msg, "[DEBUG]"))
	case strings.HasPrefix(msg, "[INFO]"):
		return slog.LevelInfo, strings.TrimSpace(strings.TrimPrefix(msg, "[INFO]"))
	case strings.HasPrefix(msg, "[WARN]"):
		return slog.LevelWarn, strings.TrimSpace(strings.TrimPrefix(msg, "[WARN]"))
	case strings.HasPrefix(msg, "[WARNING]"):
		return slog.LevelWarn, strings.TrimSpace(strings.TrimPrefix(msg, "[WARNING]"))
	case strings.HasPrefix(msg, "[ERR]"):
		return slog.LevelError, strings.TrimSpace(strings.TrimPrefix(msg, "[ERR]"))
	case strings.HasPrefix(msg, "[ERROR]"):
		return slog.LevelError, strings.TrimSpace(strings.TrimPrefix(msg, "[ERROR]"))
	default:
		return defaultLevel, msg
	}
}

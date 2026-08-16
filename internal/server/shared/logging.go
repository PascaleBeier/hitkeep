package shared

import (
	"context"
	"log/slog"

	"hitkeep/internal/hklog"
)

// WithLogger attaches a request-scoped logger to ctx.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return hklog.WithLogger(ctx, logger)
}

// WithLoggerAttrs derives the request logger with additional structured fields.
func WithLoggerAttrs(ctx context.Context, args ...any) context.Context {
	return hklog.WithLoggerAttrs(ctx, args...)
}

// LoggerFromContext returns the request logger, falling back to the default
// logger for library callers that do not have a request context.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	return hklog.LoggerFromContext(ctx)
}

func withLoggerIfAbsent(ctx context.Context, logger *slog.Logger) context.Context {
	return hklog.WithLoggerIfAbsent(ctx, logger)
}

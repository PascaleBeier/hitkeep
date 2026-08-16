package hklog

import (
	"context"
	"log/slog"
	"testing"
)

func TestWithLoggerIfAbsentReplacesTypedNilLogger(t *testing.T) {
	expected := slog.New(slog.NewTextHandler(nilWriter{}, nil))
	ctx := context.WithValue(context.Background(), loggerContextKey{}, (*slog.Logger)(nil))

	got := LoggerFromContext(WithLoggerIfAbsent(ctx, expected))
	if got != expected {
		t.Fatalf("expected typed nil logger to be replaced")
	}
}

func TestLoggerFromContextOrUsesInjectedFallback(t *testing.T) {
	expected := slog.New(slog.NewTextHandler(nilWriter{}, nil))
	if got := LoggerFromContextOr(context.Background(), expected); got != expected {
		t.Fatalf("fallback logger = %p, want %p", got, expected)
	}

	contextLogger := slog.New(slog.NewTextHandler(nilWriter{}, nil))
	ctx := WithLogger(context.Background(), contextLogger)
	if got := LoggerFromContextOr(ctx, expected); got != contextLogger {
		t.Fatalf("context logger = %p, want %p", got, contextLogger)
	}
}

func TestWritersUseDefaultLoggerWhenLoggerIsNil(t *testing.T) {
	message := []byte("message\n")

	if written, err := (Writer{Level: slog.LevelInfo}).Write(message); err != nil || written != len(message) {
		t.Fatalf("Writer.Write() = (%d, %v), want (%d, nil)", written, err, len(message))
	}
	if written, err := (LevelParsingWriter{DefaultLevel: slog.LevelInfo}).Write(message); err != nil || written != len(message) {
		t.Fatalf("LevelParsingWriter.Write() = (%d, %v), want (%d, nil)", written, err, len(message))
	}
	if err := (GoNSQLogger{}).Output(1, "INFO: message"); err != nil {
		t.Fatalf("GoNSQLogger.Output() = %v, want nil", err)
	}
	if err := (NSQDLogger{}).Output(1, "INFO: message"); err != nil {
		t.Fatalf("NSQDLogger.Output() = %v, want nil", err)
	}
}

type nilWriter struct{}

func (nilWriter) Write(p []byte) (int, error) { return len(p), nil }

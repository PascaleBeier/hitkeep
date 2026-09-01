package hklog

import (
	"log/slog"
	"strings"
)

func parseNSQLevel(s string) slog.Level {
	switch {
	case strings.HasPrefix(s, "DBG"):
		return slog.LevelDebug
	case strings.HasPrefix(s, "DEBUG:"):
		return slog.LevelDebug
	case strings.HasPrefix(s, "INF"):
		return slog.LevelInfo
	case strings.HasPrefix(s, "INFO:"):
		return slog.LevelInfo
	case strings.HasPrefix(s, "WRN"):
		return slog.LevelWarn
	case strings.HasPrefix(s, "WARN:"):
		return slog.LevelWarn
	case strings.HasPrefix(s, "WARNING:"):
		return slog.LevelWarn
	case strings.HasPrefix(s, "ERR"):
		return slog.LevelError
	case strings.HasPrefix(s, "ERROR:"):
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

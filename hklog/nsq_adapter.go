package hklog

import (
	"context"
	"log/slog"

	"github.com/nsqio/go-nsq"
)

// GoNSQLogger satisfies the go-nsq logger interface.
// It forwards log lines from a go-nsq Producer or Consumer to slog.
type GoNSQLogger struct {
	Logger *slog.Logger
}

func (l GoNSQLogger) Output(calldepth int, s string) error {
	lvl := parseNSQLevel(s)
	logger := l.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Log(context.Background(), lvl, s, "calldepth", calldepth)
	return nil
}

// NSQGoLevel maps slog.Level to go-nsq LogLevel for SetLogger thresholding.
func NSQGoLevel(lvl slog.Level) nsq.LogLevel {
	switch {
	case lvl <= slog.LevelDebug:
		return nsq.LogLevelDebug
	case lvl <= slog.LevelInfo:
		return nsq.LogLevelInfo
	case lvl <= slog.LevelWarn:
		return nsq.LogLevelWarning
	default:
		return nsq.LogLevelError
	}
}

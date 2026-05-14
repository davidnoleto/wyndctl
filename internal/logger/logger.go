// Package logger provides structured logging using slog.
package logger

import (
	"log/slog"
	"os"
	"strings"

	"github.com/hellowynd/wyndctl/internal/config"
)

// New creates a configured slog.Logger from the log config.
func New(cfg config.LogConfig) *slog.Logger {
	level := parseLevel(cfg.Level)
	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	switch strings.ToLower(cfg.Format) {
	case "json":
		handler = slog.NewJSONHandler(os.Stderr, opts)
	default:
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	return slog.New(handler)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

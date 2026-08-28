// Package logging centralizes slog configuration for the hub.
package logging

import (
	"log/slog"
	"os"
	"strings"
)

// New constructs a *slog.Logger emitting JSON to stdout at the given level.
func New(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	return slog.New(h)
}

// WithRequest enriches the logger with a request id.
func WithRequest(logger *slog.Logger, requestID string) *slog.Logger {
	return logger.With("request_id", requestID)
}

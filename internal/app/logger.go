package app

import (
	"log/slog"
	"os"
	"strings"

	"github.com/deeptrols/api/internal/config"
)

// NewSlogLogger builds the process logger from configuration using only the
// standard library. LOG_FORMAT=json|text (default json), LOG_LEVEL=
// debug|info|warn|error (default info). Unknown values fall back to defaults
// instead of failing startup.
func NewSlogLogger(cfg *config.Config) *slog.Logger {
	format := strings.ToLower(cfg.Server.LogFormat)
	level := parseLogLevel(cfg.Server.LogLevel)

	var handler slog.Handler
	if format == "text" {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	} else {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	}
	return slog.New(handler)
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

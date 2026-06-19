package logging

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

func New(level string, format string) (*slog.Logger, error) {
	parsedLevel, err := parseLevel(level)
	if err != nil {
		return nil, err
	}

	handlerOptions := &slog.HandlerOptions{
		AddSource: true,
		Level:     parsedLevel,
	}

	switch normalize(format) {
	case "", "json":
		return slog.New(slog.NewJSONHandler(os.Stdout, handlerOptions)), nil
	case "text":
		return slog.New(slog.NewTextHandler(os.Stdout, handlerOptions)), nil
	default:
		return nil, fmt.Errorf("unsupported log format %q", format)
	}
}

func parseLevel(level string) (slog.Level, error) {
	switch normalize(level) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unsupported log level %q", level)
	}
}

func normalize(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}

package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

const (
	FieldService    = "service"
	FieldComponent  = "component"
	FieldRunID      = "run_id"
	FieldToolCallID = "tool_call_id"
)

type Options struct {
	Service   string
	Component string
	Level     slog.Leveler
	Writer    io.Writer
}

func New(opts Options) *slog.Logger {
	handler := slog.NewJSONHandler(resolveWriter(opts.Writer), &slog.HandlerOptions{
		Level: resolveLevel(opts.Level),
	})

	logger := slog.New(handler)
	if opts.Service != "" {
		logger = logger.With(slog.String(FieldService, opts.Service))
	}
	if opts.Component != "" {
		logger = logger.With(slog.String(FieldComponent, opts.Component))
	}

	return logger
}

func WithComponent(logger *slog.Logger, component string) *slog.Logger {
	if component == "" {
		return logger
	}
	return logger.With(slog.String(FieldComponent, component))
}

func WithRunID(logger *slog.Logger, runID string) *slog.Logger {
	if runID == "" {
		return logger
	}
	return logger.With(slog.String(FieldRunID, runID))
}

func WithToolCallID(logger *slog.Logger, toolCallID string) *slog.Logger {
	if toolCallID == "" {
		return logger
	}
	return logger.With(slog.String(FieldToolCallID, toolCallID))
}

func MaskSecret(value string) string {
	if value == "" {
		return ""
	}

	runes := []rune(value)
	if len(runes) <= 4 {
		return strings.Repeat("*", 4)
	}

	maskedMiddle := strings.Repeat("*", len(runes)-4)
	return string(runes[:2]) + maskedMiddle + string(runes[len(runes)-2:])
}

func resolveWriter(writer io.Writer) io.Writer {
	if writer != nil {
		return writer
	}
	return os.Stdout
}

func resolveLevel(level slog.Leveler) slog.Leveler {
	if level != nil {
		return level
	}
	return slog.LevelInfo
}

package logging

import (
	"context"
	"log/slog"
)

type contextKey struct{}

// Associates a logger with an execution context without modifying global logging state.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, logger)
}

// Returns the execution logger, falling back to the default logger when none is provided.
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(contextKey{}).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return slog.Default()
}

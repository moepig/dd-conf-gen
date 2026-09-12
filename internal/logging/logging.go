package logging

import (
	"context"
	"log/slog"
)

// Identifies the logger value within a context.
type contextKey struct{}

// Returns a child of ctx containing logger without modifying global logging state.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, logger)
}

// Returns the non-nil logger stored in ctx, or the default logger when no non-nil logger is stored.
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(contextKey{}).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return slog.Default()
}

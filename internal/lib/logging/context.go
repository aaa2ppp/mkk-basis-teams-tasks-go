package logging

import (
	"context"
	"log/slog"
)

type loggerKey struct{}

// contextWithLogger returns context with logger.
func contextWithLogger(ctx context.Context, log *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, log)
}

// GetLogger returns the logger from the context. If there is no logger
// in the context, it returns the default logger.
func GetLogger(ctx context.Context) *slog.Logger {
	log := ctx.Value(loggerKey{})
	if log != nil {
		return log.(*slog.Logger)
	}
	return slog.Default()
}

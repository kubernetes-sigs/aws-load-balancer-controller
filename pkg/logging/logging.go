package logging

import (
	"context"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const contextKeyLogger = "Logger"

func WithLogger(ctx context.Context, logger logr.Logger) context.Context {
	return context.WithValue(ctx, contextKeyLogger, logger)
}

func FromContext(ctx context.Context) logr.Logger {
	logger, ok := ctx.Value(contextKeyLogger).(logr.Logger)
	if !ok {
		return log.Log
	}
	return logger
}

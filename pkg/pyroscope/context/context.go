package context

import (
	"context"
	"os"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
)

type contextKey int

const (
	loggerKey contextKey = iota
	registryKey
	localBucketClient
)

var (
	defaultLogger = log.NewLogfmtLogger(os.Stderr)
)

func WithLogger(ctx context.Context, logger log.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

func Logger(ctx context.Context) log.Logger {
	if logger, ok := ctx.Value(loggerKey).(log.Logger); ok {
		return logger
	}
	return defaultLogger
}

func WithRegistry(ctx context.Context, registry prometheus.Registerer) context.Context {
	return context.WithValue(ctx, registryKey, registry)
}

func Registry(ctx context.Context) prometheus.Registerer {
	if registry, ok := ctx.Value(registryKey).(prometheus.Registerer); ok {
		return registry
	}
	return prometheus.NewRegistry()
}

func WrapTenant(ctx context.Context, tenantID string) context.Context {
	// wrap registry
	reg := Registry(ctx)
	ctx = WithRegistry(ctx, prometheus.WrapRegistererWith(
		prometheus.Labels{"tenant": tenantID},
		reg,
	))

	// add field to logger
	logger := Logger(ctx)
	ctx = WithLogger(ctx,
		log.With(logger, "tenant", tenantID),
	)

	return ctx
}

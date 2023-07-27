package context

import (
	"context"
	"os"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"

	phlareobj "github.com/grafana/pyroscope/pkg/objstore"
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

func WithLocalBucketClient(ctx context.Context, client phlareobj.Bucket) context.Context {
	return context.WithValue(ctx, localBucketClient, client)
}

func LocalBucketClient(ctx context.Context) phlareobj.Bucket {
	if client, ok := ctx.Value(localBucketClient).(phlareobj.Bucket); ok {
		return client
	}
	return nil
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

	// wrap local bucket client
	localBucket := LocalBucketClient(ctx)
	if localBucket != nil {
		ctx = WithLocalBucketClient(ctx, phlareobj.NewPrefixedBucket(localBucket, tenantID))
	}

	return ctx
}

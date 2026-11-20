package async

import (
	"context"
	"time"
)

type contextKey struct{}

func WithQueryTimeout(ctx context.Context, timeout time.Duration) context.Context {
	return context.WithValue(ctx, contextKey{}, timeout)
}

func QueryTimeout(ctx context.Context) time.Duration {
	timeout, _ := ctx.Value(contextKey{}).(time.Duration)
	return timeout
}

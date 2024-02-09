package util

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/tracing"

	"github.com/grafana/pyroscope/pkg/tenant"
)

type timeoutInterceptor struct {
	timeout time.Duration
}

// WithTimeout returns a new timeout interceptor.
func WithTimeout(timeout time.Duration) connect.Interceptor {
	return timeoutInterceptor{timeout: timeout}
}

func (s timeoutInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, ar connect.AnyRequest) (connect.AnyResponse, error) {
		ctx, cancel := context.WithTimeout(ctx, s.timeout)
		defer cancel()
		return next(ctx, ar)
	}
}

func (s timeoutInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		ctx, cancel := context.WithTimeout(ctx, s.timeout)
		defer cancel()
		return next(ctx, spec)
	}
}

func (s timeoutInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, shc connect.StreamingHandlerConn) error {
		ctx, cancel := context.WithTimeout(ctx, s.timeout)
		defer cancel()
		return next(ctx, shc)
	}
}

// NewLogInterceptor logs the request parameters.
// It logs all kinds of requests.
func NewLogInterceptor(logger log.Logger) connect.UnaryInterceptorFunc {
	interceptor := func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(
			ctx context.Context,
			req connect.AnyRequest,
		) (connect.AnyResponse, error) {
			begin := time.Now()
			tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
			if err != nil {
				tenantID = "anonymous"
			}
			traceID, ok := tracing.ExtractTraceID(ctx)
			if !ok {
				traceID = "unknown"
			}
			defer func() {
				level.Info(logger).Log(
					"msg", "request parameters",
					"route", req.Spec().Procedure,
					"tenant", tenantID,
					"traceID", traceID,
					"parameters", req.Any(),
					"duration", time.Since(begin),
				)
			}()

			return next(ctx, req)
		}
	}
	return interceptor
}

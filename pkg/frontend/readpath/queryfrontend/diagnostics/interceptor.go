package diagnostics

import (
	"context"
	"strings"
	"time"

	"connectrpc.com/connect"
)

const (
	RequestHeader = "X-Pyroscope-Collect-Diagnostics"
	IdHeader      = "X-Pyroscope-Diagnostics-Id"
)

// Context key for diagnostics context.
type contextKey struct{}

// Context holds diagnostics collection state injected into the context.
type Context struct {
	Collect   bool
	startTime time.Time
	ID        string
}

// InjectCollectDiagnostics injects the diagnostics collection flag and start time into the context.
func InjectCollectDiagnostics(ctx context.Context, collect bool) context.Context {
	id := ""
	if collect {
		id = generateUUID()
	}
	return context.WithValue(ctx, contextKey{}, &Context{
		Collect:   collect,
		startTime: time.Now(),
		ID:        id,
	})
}

// From reads the diagnostics context from the context.
func From(ctx context.Context) *Context {
	if v, ok := ctx.Value(contextKey{}).(*Context); ok {
		return v
	}
	return nil
}

// InjectCollectDiagnosticsFromHeader checks the request header and injects the flag into context.
func InjectCollectDiagnosticsFromHeader(ctx context.Context, headers map[string][]string) context.Context {
	return InjectCollectDiagnostics(ctx, shouldCollectDiagnostics(headers))
}

// shouldCollectDiagnostics checks if a diagnostics collection was requested.
func shouldCollectDiagnostics(headers map[string][]string) bool {
	values := headers[RequestHeader]
	if len(values) == 0 {
		values = headers[strings.ToLower(RequestHeader)]
	}
	for _, v := range values {
		if v == "true" || v == "1" {
			return true
		}
	}
	return false
}

// Interceptor is a connect interceptor that extracts the diagnostics
// collection flag from request headers and injects it into the context.
var Interceptor = &interceptor{}

type interceptor struct{}

func (i *interceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		if !req.Spec().IsClient {
			ctx = InjectCollectDiagnosticsFromHeader(ctx, req.Header())
		}
		return next(ctx, req)
	}
}

func (i *interceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *interceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		ctx = InjectCollectDiagnosticsFromHeader(ctx, conn.RequestHeader())
		return next(ctx, conn)
	}
}

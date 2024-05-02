package util

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/httpgrpc"
	"github.com/grafana/dskit/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	httputil "github.com/grafana/pyroscope/pkg/util/http"
)

const maxStacksize = 8 * 1024

var (
	panicTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "pyroscope",
		Name:      "panic_total",
		Help:      "The total number of panic triggered",
	})

	RecoveryHTTPMiddleware = middleware.Func(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			defer func() {
				if p := recover(); p != nil {
					httputil.Error(w, httpgrpc.Errorf(http.StatusInternalServerError, "error while processing request: %v", panicError(p)))
				}
			}()
			next.ServeHTTP(w, req)
		})
	})

	RecoveryInterceptor recoveryInterceptor
)

func panicError(p interface{}) error {
	stack := make([]byte, maxStacksize)
	stack = stack[:runtime.Stack(stack, true)]
	// keep a multiline stack
	fmt.Fprintf(os.Stderr, "panic: %v\n%s", p, stack)
	panicTotal.Inc()
	return fmt.Errorf("%v", p)
}

// RecoverPanic is a helper function to recover from panic and return an error.
func RecoverPanic(f func() error) func() error {
	return func() (err error) {
		defer func() {
			if p := recover(); p != nil {
				err = panicError(p)
			}
		}()
		return f()
	}
}

type recoveryInterceptor struct{}

func (recoveryInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (resp connect.AnyResponse, err error) {
		defer func() {
			if p := recover(); p != nil {
				err = connect.NewError(connect.CodeInternal, panicError(p))
			}
		}()
		return next(ctx, req)
	}
}

func (recoveryInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) (err error) {
		defer func() {
			if p := recover(); p != nil {
				err = connect.NewError(connect.CodeInternal, panicError(p))
			}
		}()
		return next(ctx, conn)
	}
}

func (recoveryInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

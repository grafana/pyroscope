package circuitbreaker

import (
	"context"
	"fmt"

	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func UnaryClientInterceptor(cb *gobreaker.CircuitBreaker[any]) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		_, err := cb.Execute(func() (interface{}, error) {
			return nil, invoker(ctx, method, req, reply, cc, opts...)
		})
		// gobreaker returns unwrapped errors.
		switch err {
		case nil:
		case gobreaker.ErrOpenState,
			gobreaker.ErrTooManyRequests:
			return status.Error(codes.Unavailable, fmt.Sprintf("circuit breaker: %s", err.Error()))
		}
		return err
	}
}

package delayhandler

import (
	"context"

	"connectrpc.com/connect"
)

type delayInterceptor struct {
	limits Limits
}

func NewConnect(limits Limits) connect.Interceptor {
	return &delayInterceptor{limits: limits}
}

func (i *delayInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (resp connect.AnyResponse, err error) {
		start := timeNow()

		// now run the chain after me
		resp, err = next(ctx, req)

		// if there is an error, return it immediately
		if err != nil {
			return resp, err
		}

		// get delay from the context
		delay := getDelay(ctx, i.limits)

		// no delay, return immediately
		if delay <= 0 {
			return resp, err
		}

		delayLeft := delay - timeNow().Sub(start)

		// if the delay is already expired, return immediately
		if delayLeft <= 0 {
			return resp, err
		}

		// add delay header
		addDelayHeader(resp.Header(), delayLeft)

		// if the delay is not expired, sleep for the remaining time
		<-timeAfter(delayLeft)

		return resp, nil
	}
}

// do nothing for streaming handlers
func (delayInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) (err error) {
		panic("delayInterceptor not implemented")
	}
}

// do nothing for streaming clients
func (delayInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	panic("delayInterceptor not implemented")
}

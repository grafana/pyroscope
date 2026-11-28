package util

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
)

type fakeClientConn struct {
	connect.StreamingClientConn
	ctx context.Context
}

func (c *fakeClientConn) CloseResponse() error { return nil }

func TestTimeoutInterceptor_StreamingClientContextOutlivesWrapper(t *testing.T) {
	var streamCtx context.Context
	next := func(ctx context.Context, _ connect.Spec) connect.StreamingClientConn {
		streamCtx = ctx
		return &fakeClientConn{ctx: ctx}
	}

	conn := WithTimeout(time.Minute).(timeoutInterceptor).WrapStreamingClient(next)(context.Background(), connect.Spec{})

	require.NoError(t, streamCtx.Err(), "stream context must stay alive after the stream is created")
	_, hasDeadline := streamCtx.Deadline()
	require.True(t, hasDeadline)

	require.NoError(t, conn.CloseResponse())
	require.ErrorIs(t, streamCtx.Err(), context.Canceled, "stream context must be released on close")
}

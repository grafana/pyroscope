package querybackendclient

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/querybackend"
)

func TestClient_QueryDeadline(t *testing.T) {
	for _, tc := range []struct {
		name           string
		callerTimeout  time.Duration
		processingTime time.Duration
		wantElapsed    time.Duration
		wantCode       codes.Code
	}{
		{name: "no deadline", processingTime: 31 * time.Second, wantElapsed: 31 * time.Second},
		{name: "short caller deadline", callerTimeout: 500 * time.Millisecond, processingTime: 2 * time.Second, wantElapsed: 500 * time.Millisecond, wantCode: codes.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				reader := &testQueryBackendServer{invoke: func(ctx context.Context, _ int32) (*queryv1.InvokeResponse, error) {
					select {
					case <-time.After(tc.processingTime):
						return &queryv1.InvokeResponse{}, nil
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}}
				client := newTestClient(t, reader)
				ctx := t.Context()
				if tc.callerTimeout > 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, tc.callerTimeout)
					defer cancel()
				}
				start := time.Now()
				_, err := client.Invoke(ctx, &queryv1.InvokeRequest{})
				require.Equal(t, tc.wantCode, status.Code(err))
				require.Equal(t, tc.wantElapsed, time.Since(start))
			})
		})
	}
}

func TestClient_QueryCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		reader := &testQueryBackendServer{invoke: func(ctx context.Context, _ int32) (*queryv1.InvokeResponse, error) {
			cancel()
			<-ctx.Done()
			return nil, ctx.Err()
		}}
		client := newTestClient(t, reader)
		_, err := client.Invoke(ctx, &queryv1.InvokeRequest{})
		require.Equal(t, codes.Canceled, status.Code(err))
	})
}

func TestClient_QueryDeadlineIncludesRetries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reader := &testQueryBackendServer{invoke: func(ctx context.Context, _ int32) (*queryv1.InvokeResponse, error) {
			if err := grpc.SetTrailer(ctx, metadata.Pairs("grpc-retry-pushback-ms", "1000")); err != nil {
				return nil, err
			}
			return nil, status.Error(codes.Unavailable, "retry")
		}}
		client := newTestClient(t, reader)
		ctx, cancel := context.WithTimeout(t.Context(), 2500*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, err := client.Invoke(ctx, &queryv1.InvokeRequest{})
		require.Equal(t, codes.DeadlineExceeded, status.Code(err))
		require.Equal(t, 2500*time.Millisecond, time.Since(start))
		require.Equal(t, int32(3), reader.calls.Load())
	})
}

func TestClient_ChildInheritsQueryDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		remaining := make(chan time.Duration, 1)
		reader := &testQueryBackendServer{invoke: func(ctx context.Context, _ int32) (*queryv1.InvokeResponse, error) {
			deadline, _ := ctx.Deadline()
			remaining <- time.Until(deadline)
			<-ctx.Done()
			return nil, ctx.Err()
		}}
		leafClient := newTestClient(t, reader)
		root, err := querybackend.New(querybackend.Config{}, log.NewNopLogger(), nil, timingHandler(func(ctx context.Context, req *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
			time.Sleep(time.Second)
			return leafClient.Invoke(ctx, req)
		}), nil)
		require.NoError(t, err)
		client := newTestClient(t, root)
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()
		_, err = client.Invoke(ctx, &queryv1.InvokeRequest{
			QueryPlan: &queryv1.QueryPlan{Root: &queryv1.QueryNode{
				Type:     queryv1.QueryNode_MERGE,
				Children: []*queryv1.QueryNode{{Type: queryv1.QueryNode_READ}},
			}},
		})
		require.Equal(t, codes.DeadlineExceeded, status.Code(err))
		select {
		case got := <-remaining:
			require.Equal(t, 2*time.Second, got)
		default:
			t.Fatal("leaf was not invoked")
		}
	})
}

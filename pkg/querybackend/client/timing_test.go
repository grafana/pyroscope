package querybackendclient

import (
	"context"
	"fmt"
	"testing"
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

func TestClient_DurationAccounting(t *testing.T) {
	for _, failures := range []int32{0, 1, 3} {
		for _, collectDiagnostics := range []bool{false, true} {
			t.Run(fmt.Sprintf("failures=%d/diagnostics=%t", failures, collectDiagnostics), func(t *testing.T) {
				reader := &testQueryBackendServer{
					invoke: func(ctx context.Context, call int32) (*queryv1.InvokeResponse, error) {
						if call <= failures {
							// Failed-attempt work must remain unaccounted, not be
							// attributed to successful backend processing.
							time.Sleep(30 * time.Millisecond)
							if err := grpc.SetTrailer(ctx, metadata.Pairs("grpc-retry-pushback-ms", "20")); err != nil {
								return nil, err
							}
							return nil, status.Error(codes.Unavailable, "retry")
						}
						time.Sleep(10 * time.Millisecond)
						return &queryv1.InvokeResponse{Diagnostics: &queryv1.Diagnostics{
							ExecutionNode: &queryv1.ExecutionNode{Stats: &queryv1.ExecutionStats{
								ProcessingDurationNs: int64(10 * time.Millisecond),
							}},
						}}, nil
					},
				}
				client := newTestClient(t, reader)
				start := time.Now()
				resp, err := client.Invoke(t.Context(), &queryv1.InvokeRequest{
					Options: &queryv1.InvokeOptions{CollectDiagnostics: collectDiagnostics},
				})
				elapsed := time.Since(start)
				require.NoError(t, err)
				require.Equal(t, failures+1, reader.calls.Load())
				stats := resp.GetDiagnostics().GetExecutionNode().GetStats()
				require.Equal(t, int64(10*time.Millisecond), stats.GetProcessingDurationNs())
				require.GreaterOrEqual(t, stats.GetUnaccountedDurationNs(), int64(failures)*int64(30*time.Millisecond))
				if failures == 0 {
					require.Zero(t, stats.GetBackoffDurationNs())
				} else {
					require.GreaterOrEqual(t, stats.GetBackoffDurationNs(), int64(failures)*int64(20*time.Millisecond))
				}
				total := stats.GetProcessingDurationNs() + stats.GetBackoffDurationNs() + stats.GetUnaccountedDurationNs()
				require.InDelta(t, elapsed.Nanoseconds(), total, float64(20*time.Millisecond))
			})
		}
	}
}

func TestClient_NestedBackoffIsNotDoubleCounted(t *testing.T) {
	reader := &testQueryBackendServer{invoke: func(ctx context.Context, call int32) (*queryv1.InvokeResponse, error) {
		if call == 1 {
			if err := grpc.SetTrailer(ctx, metadata.Pairs("grpc-retry-pushback-ms", "20")); err != nil {
				return nil, err
			}
			return nil, status.Error(codes.ResourceExhausted, "busy")
		}
		time.Sleep(10 * time.Millisecond)
		return &queryv1.InvokeResponse{Diagnostics: &queryv1.Diagnostics{
			ExecutionNode: &queryv1.ExecutionNode{Stats: &queryv1.ExecutionStats{
				ProcessingDurationNs: int64(10 * time.Millisecond),
			}},
		}}, nil
	}}
	leafClient := newTestClient(t, reader)
	leafStatsCh := make(chan *queryv1.ExecutionStats, 1)
	root, err := querybackend.New(querybackend.Config{}, log.NewNopLogger(), nil, timingHandler(func(ctx context.Context, req *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
		resp, err := leafClient.Invoke(ctx, req)
		if err == nil {
			leafStatsCh <- resp.GetDiagnostics().GetExecutionNode().GetStats().CloneVT()
		}
		return resp, err
	}), nil)
	require.NoError(t, err)
	client := newTestClient(t, root)
	start := time.Now()
	resp, err := client.Invoke(t.Context(), &queryv1.InvokeRequest{QueryPlan: &queryv1.QueryPlan{
		Root: &queryv1.QueryNode{Type: queryv1.QueryNode_MERGE, Children: []*queryv1.QueryNode{{Type: queryv1.QueryNode_READ}}},
	}})
	elapsed := time.Since(start)
	require.NoError(t, err)
	leafStats := <-leafStatsCh
	require.NotNil(t, leafStats)
	stats := resp.GetDiagnostics().GetExecutionNode().GetStats()
	require.Equal(t, leafStats.BackoffDurationNs, stats.BackoffDurationNs)
	require.GreaterOrEqual(t, stats.UnaccountedDurationNs, leafStats.UnaccountedDurationNs)
	total := stats.ProcessingDurationNs + stats.BackoffDurationNs + stats.UnaccountedDurationNs
	require.InDelta(t, elapsed.Nanoseconds(), total, float64(20*time.Millisecond))
}

func TestClient_DurationAccountingWithoutBackendStats(t *testing.T) {
	reader := &testQueryBackendServer{invoke: func(context.Context, int32) (*queryv1.InvokeResponse, error) {
		time.Sleep(10 * time.Millisecond)
		return &queryv1.InvokeResponse{}, nil
	}}
	client := newTestClient(t, reader)
	resp, err := client.Invoke(t.Context(), &queryv1.InvokeRequest{})
	require.NoError(t, err)
	stats := resp.GetDiagnostics().GetExecutionNode().GetStats()
	require.NotNil(t, stats)
	require.Zero(t, stats.ProcessingDurationNs)
	require.Zero(t, stats.BackoffDurationNs)
	require.GreaterOrEqual(t, stats.UnaccountedDurationNs, int64(10*time.Millisecond))
}

type timingHandler func(context.Context, *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error)

func (h timingHandler) Invoke(ctx context.Context, req *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
	return h(ctx, req)
}

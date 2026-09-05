package querybackend

import (
	"context"
	"flag"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/querybackend/internal/pushback"
)

type testQueryHandler struct {
	invoke func(context.Context, *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error)
}

func (h *testQueryHandler) Invoke(ctx context.Context, req *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
	return h.invoke(ctx, req)
}

type testServerStream struct {
	trailer metadata.MD
}

func (*testServerStream) Method() string               { return "" }
func (*testServerStream) SetHeader(metadata.MD) error  { return nil }
func (*testServerStream) SendHeader(metadata.MD) error { return nil }

func (s *testServerStream) SetTrailer(md metadata.MD) error {
	s.trailer = metadata.Join(s.trailer, md)
	return nil
}

func TestQueryBackend_NoRetrySignal(t *testing.T) {
	readPlan := &queryv1.QueryPlan{Root: &queryv1.QueryNode{Type: queryv1.QueryNode_READ}}
	mergePlan := &queryv1.QueryPlan{Root: &queryv1.QueryNode{
		Type:     queryv1.QueryNode_MERGE,
		Children: []*queryv1.QueryNode{{Type: queryv1.QueryNode_READ}, {Type: queryv1.QueryNode_READ}},
	}}

	tests := []struct {
		name   string
		plan   *queryv1.QueryPlan
		invoke func(context.Context, *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error)
		merge  bool
		want   bool
	}{
		{
			name: "response built",
			plan: readPlan,
			invoke: func(context.Context, *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
				return &queryv1.InvokeResponse{}, nil
			},
			want: true,
		},
		{
			// Load shedding reports the same code as an undeliverable response.
			name: "query exhausted a resource",
			plan: readPlan,
			invoke: func(context.Context, *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
				return nil, status.Error(codes.ResourceExhausted, "concurrency limit exceeded")
			},
		},
		{
			name: "query failed",
			plan: readPlan,
			invoke: func(context.Context, *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
				return nil, status.Error(codes.Unavailable, "block reader unavailable")
			},
		},
		{
			name:  "every child failed",
			plan:  mergePlan,
			merge: true,
			invoke: func(context.Context, *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
				return nil, status.Error(codes.Unavailable, "backend unavailable")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &testQueryHandler{invoke: tt.invoke}
			var backendClient, blockReader QueryHandler = nil, handler
			if tt.merge {
				backendClient, blockReader = handler, nil
			}
			q, err := New(Config{}, log.NewNopLogger(), nil, backendClient, blockReader)
			require.NoError(t, err)

			stream := &testServerStream{}
			ctx := grpc.NewContextWithServerTransportStream(context.Background(), stream)
			_, _ = q.Invoke(ctx, &queryv1.InvokeRequest{QueryPlan: tt.plan})

			require.Equal(t, tt.want, pushback.IsNoRetry(stream.trailer))
		})
	}
}

// The child that cannot deliver may not be the one errgroup reports.
func TestQueryBackend_NoRetrySignal_SiblingErrorWins(t *testing.T) {
	first := status.Error(codes.Unavailable, "fast failure")
	undeliverable := pushback.Mark(status.Error(codes.ResourceExhausted, "trying to send message larger than max"))
	scheduled := make(chan struct{}, 1)

	handler := &testQueryHandler{
		invoke: func(context.Context, *queryv1.InvokeRequest) (*queryv1.InvokeResponse, error) {
			select {
			case scheduled <- struct{}{}:
				return nil, first
			default:
				time.Sleep(50 * time.Millisecond)
				return nil, undeliverable
			}
		},
	}
	q, err := New(Config{}, log.NewNopLogger(), nil, handler, nil)
	require.NoError(t, err)

	stream := &testServerStream{}
	ctx := grpc.NewContextWithServerTransportStream(context.Background(), stream)
	_, err = q.Invoke(ctx, &queryv1.InvokeRequest{QueryPlan: &queryv1.QueryPlan{
		Root: &queryv1.QueryNode{
			Type:     queryv1.QueryNode_MERGE,
			Children: []*queryv1.QueryNode{{Type: queryv1.QueryNode_READ}, {Type: queryv1.QueryNode_READ}},
		},
	}})

	require.ErrorIs(t, err, first)
	require.True(t, pushback.IsNoRetry(stream.trailer))
}

func TestConfig_Validate(t *testing.T) {
	var cfg Config
	cfg.RegisterFlags(flag.NewFlagSet("", flag.PanicOnError))
	require.NoError(t, cfg.Validate())

	cfg.GRPCClientConfig.BackoffOnRatelimits = true
	require.ErrorContains(t, cfg.Validate(), "backoff-on-ratelimits")
}

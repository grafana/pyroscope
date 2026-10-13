package queryfrontend

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/frontend"
)

func TestFrameFilterInvalidRegexIsRejectedBeforeQuery(t *testing.T) {
	q := NewQueryFrontend(log.NewNopLogger(), nil, frontend.Config{}, nil, nil, nil, nil, nil, nil)
	selector := &typesv1.StackTraceSelector{FrameFilter: &typesv1.StackFrameFilter{
		IncludeFunctionNameRegexes: []string{"["},
	}}
	ctx := context.Background()
	_, err := q.SelectMergeStacktraces(ctx, connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
		StackTraceSelector: selector,
	}))
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = q.SelectMergeProfile(ctx, connect.NewRequest(&querierv1.SelectMergeProfileRequest{
		StackTraceSelector: selector,
	}))
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	_, err = q.SelectSeries(ctx, connect.NewRequest(&querierv1.SelectSeriesRequest{
		StackTraceSelector: selector,
	}))
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

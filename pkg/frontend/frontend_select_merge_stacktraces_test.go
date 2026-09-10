package frontend

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/user"
	"github.com/stretchr/testify/require"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockfrontend"
	"github.com/grafana/pyroscope/v2/pkg/util/connectgrpc"
	"github.com/grafana/pyroscope/v2/pkg/util/httpgrpc"
)

func TestFrontend_SelectMergeStacktraces_SpanPprofUsesLegacySpanRPC(t *testing.T) {
	limits := mockfrontend.NewMockLimits(t)
	limits.On("MaxFlameGraphNodesDefault", "test").Return(10_000)
	limits.On("MaxQueryLookback", "test").Return(time.Duration(0))
	limits.On("MaxQueryLength", "test").Return(time.Duration(0))
	limits.On("MaxQueryParallelism", "test").Return(100)
	limits.On("QuerySplitDuration", "test").Return(time.Hour)

	spanSelector := []string{"0000000000000001"}
	frontend := &Frontend{limits: limits}
	frontend.GRPCRoundTripper = &mockRoundTripper{callback: func(ctx context.Context, req *httpgrpc.HTTPRequest) (*httpgrpc.HTTPResponse, error) {
		return connectgrpc.HandleUnary[querierv1.SelectMergeSpanProfileRequest, querierv1.SelectMergeSpanProfileResponse](ctx, req, func(_ context.Context, req *connect.Request[querierv1.SelectMergeSpanProfileRequest]) (*connect.Response[querierv1.SelectMergeSpanProfileResponse], error) {
			if !slices.Equal(spanSelector, req.Msg.SpanSelector) {
				return nil, errors.New("unexpected span selector")
			}
			tree := new(model.FunctionNameTree)
			tree.InsertStack(1, "foo")
			return connect.NewResponse(&querierv1.SelectMergeSpanProfileResponse{Tree: tree.Bytes(-1, nil)}), nil
		})
	}}

	ctx := user.InjectOrgID(context.Background(), "test")
	now := time.Now()
	resp, err := frontend.SelectMergeStacktraces(ctx, connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
		ProfileTypeID: "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
		LabelSelector: "{}",
		Start:         now.UnixMilli(),
		End:           now.Add(time.Minute).UnixMilli(),
		Format:        querierv1.ProfileFormat_PROFILE_FORMAT_PPROF,
		SpanSelector:  spanSelector,
	}))

	require.NoError(t, err)
	require.NotNil(t, resp.Msg.GetPprof().GetProfile())
	require.Len(t, resp.Msg.Pprof.Profile.Sample, 1)
}

func TestFrontend_SelectFunctions_AppliesLimitAfterSplitting(t *testing.T) {
	limits := mockfrontend.NewMockLimits(t)
	limits.On("MaxQueryLookback", "test").Return(time.Duration(0))
	limits.On("MaxQueryLength", "test").Return(time.Duration(0))
	limits.On("MaxQueryParallelism", "test").Return(2)
	limits.On("QuerySplitDuration", "test").Return(time.Hour)
	var calls atomic.Int64
	f := &Frontend{limits: limits}
	f.GRPCRoundTripper = &mockRoundTripper{callback: func(ctx context.Context, req *httpgrpc.HTTPRequest) (*httpgrpc.HTTPResponse, error) {
		return connectgrpc.HandleUnary[querierv1.SelectMergeStacktracesRequest, querierv1.SelectMergeStacktracesResponse](ctx, req, func(_ context.Context, req *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
			calls.Add(1)
			if req.Msg.Format != querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS || req.Msg.MaxFunctions != -1 {
				return nil, errors.New("expected an unlimited function table for each split")
			}
			if !slices.Equal(req.Msg.SpanSelector, []string{"0000000000000001"}) {
				return nil, errors.New("span selector was dropped")
			}
			name := time.UnixMilli(req.Msg.Start).String()
			return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{Functions: &querierv1.FunctionTable{
				Total: 19, Functions: []*querierv1.FunctionRow{
					{Name: name, Self: 10, Total: 10}, {Name: "global", Self: 9, Total: 9},
				},
			}}), nil
		})
	}}
	start := time.Now().Add(-3 * time.Hour).Truncate(time.Hour)
	resp, err := f.SelectMergeStacktraces(user.InjectOrgID(context.Background(), "test"), connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
		Start: start.UnixMilli(), End: start.Add(2*time.Hour).UnixMilli() - 1,
		Format: querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS, MaxFunctions: 1,
		SpanSelector: []string{"0000000000000001"},
	}))
	require.NoError(t, err)
	require.Equal(t, int64(2), calls.Load())
	require.Equal(t, int64(38), resp.Msg.Functions.Total)
	require.Equal(t, []*querierv1.FunctionRow{{Name: "global", Self: 18, Total: 18}}, resp.Msg.Functions.Functions)
}

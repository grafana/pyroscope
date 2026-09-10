package frontend

import (
	"context"
	"errors"
	"slices"
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

func TestFrontend_SelectFunctions_Unimplemented(t *testing.T) {
	t.Parallel()
	for _, limit := range []int64{0, 1, -1, -2} {
		// No limits or upstream are configured: v1 must reject the format
		// before validating the request or dispatching any queries.
		resp, err := new(Frontend).SelectMergeStacktraces(context.Background(), connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
			Format: querierv1.ProfileFormat_PROFILE_FORMAT_FUNCTIONS, MaxFunctions: limit,
		}))
		require.Nil(t, resp)
		require.Equal(t, connect.CodeUnimplemented, connect.CodeOf(err))
		require.ErrorContains(t, err, "functions format is only supported with the v2 query backend")
	}
}

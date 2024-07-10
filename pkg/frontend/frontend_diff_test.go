package frontend

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/user"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
	"github.com/grafana/pyroscope/pkg/util/httpgrpc"
)

type mockLimits struct{}

func (m *mockLimits) QuerySplitDuration(_ string) time.Duration {
	return time.Hour
}

func (m *mockLimits) MaxQueryParallelism(_ string) int {
	return 100
}

func (m *mockLimits) MaxQueryLength(_ string) time.Duration {
	return time.Hour
}

func (m *mockLimits) MaxQueryLookback(_ string) time.Duration {
	return time.Hour * 24
}

func (m *mockLimits) QueryAnalysisEnabled(_ string) bool {
	return true
}

func (m *mockLimits) MaxFlameGraphNodesDefault(_ string) int {
	return 10_000
}

func (m *mockLimits) MaxFlameGraphNodesMax(_ string) int {
	return 100_000
}

type mockRoundTripper struct {
	callback func(ctx context.Context, req *httpgrpc.HTTPRequest) (*httpgrpc.HTTPResponse, error)
}

func (m *mockRoundTripper) RoundTripGRPC(ctx context.Context, req *httpgrpc.HTTPRequest) (*httpgrpc.HTTPResponse, error) {
	if m.callback != nil {
		return m.callback(ctx, req)
	}
	return &httpgrpc.HTTPResponse{}, errors.New("not implemented")
}

func Test_Frontend_Diff(t *testing.T) {
	frontend := Frontend{
		limits: &mockLimits{},
	}

	ctx := user.InjectOrgID(context.Background(), "test")
	_, ctx = opentracing.StartSpanFromContext(ctx, "test")
	now := time.Now().UnixMilli()

	profileType := "memory:inuse_space:bytes:space:byte"

	t.Run("Diff outside of the query window", func(t *testing.T) {
		resp, err := frontend.Diff(
			ctx,
			connect.NewRequest(&querierv1.DiffRequest{
				Left: &querierv1.SelectMergeStacktracesRequest{
					ProfileTypeID: profileType,
					LabelSelector: "{}",
					Start:         1,
					End:           1000,
				},
				Right: &querierv1.SelectMergeStacktracesRequest{
					ProfileTypeID: profileType,
					LabelSelector: "{}",
					Start:         2000,
					End:           3000,
				},
			}),
		)
		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	t.Run("Failing left hand side", func(t *testing.T) {
		frontend.GRPCRoundTripper = &mockRoundTripper{callback: func(ctx context.Context, req *httpgrpc.HTTPRequest) (*httpgrpc.HTTPResponse, error) {
			return connectgrpc.HandleUnary[querierv1.SelectMergeStacktracesRequest, querierv1.SelectMergeStacktracesResponse](ctx, req, func(ctx context.Context, req *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
				if req.Msg.Start == now {
					return nil, errors.New("left fails")
				}

				return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{
					Flamegraph: &querierv1.FlameGraph{},
				}), nil
			})
		}}

		_, err := frontend.Diff(
			ctx,
			connect.NewRequest(&querierv1.DiffRequest{
				Left: &querierv1.SelectMergeStacktracesRequest{
					ProfileTypeID: profileType,
					LabelSelector: "{}",
					Start:         now + 0000,
					End:           now + 1000,
				},
				Right: &querierv1.SelectMergeStacktracesRequest{
					ProfileTypeID: profileType,
					LabelSelector: "{}",
					Start:         now + 2000,
					End:           now + 3000,
				},
			}),
		)
		require.ErrorContains(t, err, "left fails")
	})

	t.Run("simple diff", func(t *testing.T) {
		frontend.GRPCRoundTripper = &mockRoundTripper{callback: func(ctx context.Context, req *httpgrpc.HTTPRequest) (*httpgrpc.HTTPResponse, error) {
			return connectgrpc.HandleUnary[querierv1.SelectMergeStacktracesRequest, querierv1.SelectMergeStacktracesResponse](ctx, req, func(ctx context.Context, req *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {

				s := new(model.Tree)
				s.InsertStack(1, "foo", "bar")

				if req.Msg.Start == now {
					//left
					s.InsertStack(1, "foo", "bar", "baz")
				} else {
					//right
					s.InsertStack(2, "foo", "bar", "buz")
				}

				return connect.NewResponse(&querierv1.SelectMergeStacktracesResponse{
					Flamegraph: model.NewFlameGraph(s, -1),
				}), nil
			})
		}}

		resp, err := frontend.Diff(
			ctx,
			connect.NewRequest(&querierv1.DiffRequest{
				Left: &querierv1.SelectMergeStacktracesRequest{
					ProfileTypeID: profileType,
					LabelSelector: "{}",
					Start:         now + 0000,
					End:           now + 1000,
				},
				Right: &querierv1.SelectMergeStacktracesRequest{
					ProfileTypeID: profileType,
					LabelSelector: "{}",
					Start:         now + 2000,
					End:           now + 3000,
				},
			}),
		)
		require.NoError(t, err)
		require.Equal(
			t,
			&querierv1.FlameGraphDiff{
				Names: []string{"total", "foo", "bar", "buz", "baz"},
				Total: 5,
				Levels: []*querierv1.Level{
					{Values: []int64{0, 2, 0, 0, 3, 0, 0}},
					{Values: []int64{0, 2, 0, 0, 3, 0, 1}},
					{Values: []int64{0, 2, 1, 0, 3, 1, 2}},
					{Values: []int64{1, 1, 1, 1, 0, 0, 4, 0, 0, 0, 0, 2, 2, 3}},
				},
				LeftTicks:  2,
				RightTicks: 3,
				MaxSelf:    2,
			},
			resp.Msg.Flamegraph,
		)
	})

}

package queryfrontend

import (
	"context"

	"connectrpc.com/connect"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

func (q *QueryFrontend) SelectMergeSpanProfile(
	ctx context.Context,
	c *connect.Request[querierv1.SelectMergeSpanProfileRequest],
) (*connect.Response[querierv1.SelectMergeSpanProfileResponse], error) {
	format := c.Msg.Format
	if format != querierv1.ProfileFormat_PROFILE_FORMAT_TREE {
		// The deprecated RPC historically treated every non-tree format as a flamegraph.
		format = querierv1.ProfileFormat_PROFILE_FORMAT_FLAMEGRAPH
	}
	resp, err := q.SelectMergeStacktraces(ctx, connect.NewRequest(&querierv1.SelectMergeStacktracesRequest{
		ProfileTypeID: c.Msg.ProfileTypeID,
		LabelSelector: c.Msg.LabelSelector,
		Start:         c.Msg.Start,
		End:           c.Msg.End,
		MaxNodes:      c.Msg.MaxNodes,
		Format:        format,
		SpanSelector:  c.Msg.SpanSelector,
	}))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&querierv1.SelectMergeSpanProfileResponse{
		Flamegraph: resp.Msg.Flamegraph,
		Tree:       resp.Msg.Tree,
	}), nil
}

package queryfrontend

import (
	"context"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/validation"
)

func (q *QueryFrontend) LabelValues(
	ctx context.Context,
	c *connect.Request[typesv1.LabelValuesRequest],
) (*connect.Response[typesv1.LabelValuesResponse], error) {
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	empty, err := validation.SanitizeTimeRange(q.limits, tenantIDs, &c.Msg.Start, &c.Msg.End)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if empty {
		return connect.NewResponse(&typesv1.LabelValuesResponse{}), nil
	}

	labelSelector, err := buildLabelSelectorFromMatchers(c.Msg.Matchers)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	report, err := q.querySingle(ctx, &queryv1.QueryRequest{
		StartTime:     c.Msg.Start,
		EndTime:       c.Msg.End,
		LabelSelector: labelSelector,
		Query: []*queryv1.Query{{
			QueryType:   queryv1.QueryType_QUERY_LABEL_VALUES,
			LabelValues: &queryv1.LabelValuesQuery{LabelName: c.Msg.Name},
		}},
	})
	if err != nil {
		return nil, err
	}
	if report == nil {
		return connect.NewResponse(&typesv1.LabelValuesResponse{}), nil
	}
	return connect.NewResponse(&typesv1.LabelValuesResponse{Names: report.LabelValues.LabelValues}), nil
}

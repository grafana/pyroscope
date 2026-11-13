package queryfrontend

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/validation"
)

func (q *QueryFrontend) SelectSpanPresence(ctx context.Context, c *connect.Request[querierv1.SelectSpanPresenceRequest]) (*connect.Response[querierv1.SelectSpanPresenceResponse], error) {
	r := c.Msg
	invalid := func(err error) (*connect.Response[querierv1.SelectSpanPresenceResponse], error) {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if r.Start >= r.End {
		return invalid(fmt.Errorf("start must be before end"))
	}
	if len(r.SpanSelector)+len(r.TraceIdSelector) == 0 {
		return invalid(fmt.Errorf("at least one span or trace selector is required"))
	}
	if _, err := model.NewSpanSelector(r.SpanSelector); err != nil {
		return invalid(err)
	}
	if _, err := model.NewTraceSelector(r.TraceIdSelector); err != nil {
		return invalid(err)
	}
	tenants, err := tenant.TenantIDs(ctx)
	if err != nil {
		return invalid(err)
	}
	empty, err := validation.SanitizeTimeRange(q.limits, tenants, &r.Start, &r.End)
	if err != nil {
		return invalid(err)
	}
	result := &querierv1.SelectSpanPresenceResponse{}
	if empty {
		return connect.NewResponse(result), nil
	}
	report, err := q.querySingle(ctx, &queryv1.QueryRequest{
		StartTime: r.Start, EndTime: r.End, LabelSelector: r.LabelSelector,
		Query: []*queryv1.Query{{
			QueryType: queryv1.QueryType_QUERY_SPAN_PRESENCE,
			SpanPresence: &queryv1.SpanPresenceQuery{
				SpanSelector: r.SpanSelector, TraceIdSelector: r.TraceIdSelector,
			},
		}},
	})
	if err != nil {
		return nil, err
	}
	if report != nil && report.SpanPresence != nil {
		result.Spans = report.SpanPresence.Spans
	}
	return connect.NewResponse(result), nil
}

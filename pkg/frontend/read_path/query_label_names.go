package read_path

import (
	"context"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/common/model"

	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/validation"
)

func (q *QueryBackend) LabelNames(
	ctx context.Context,
	c *connect.Request[typesv1.LabelNamesRequest],
) (*connect.Response[typesv1.LabelNamesResponse], error) {
	opentracing.SpanFromContext(ctx).
		SetTag("start", model.Time(c.Msg.Start).Time().String()).
		SetTag("end", model.Time(c.Msg.End).Time().String()).
		SetTag("matchers", c.Msg.Matchers)

	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	empty, err := validation.ValidateTimeRange(q.limits, tenantIDs, &c.Msg.Start, &c.Msg.End)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if empty {
		return connect.NewResponse(&typesv1.LabelNamesResponse{}), nil
	}

	labelSelector, err := buildLabelSelectorFromMatchers(c.Msg.Matchers)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	report, err := q.Query(ctx, c.Msg.Start, c.Msg.End, tenantIDs, labelSelector, &querybackendv1.Query{
		QueryType:  querybackendv1.QueryType_QUERY_LABEL_NAMES,
		LabelNames: &querybackendv1.LabelNamesQuery{},
	})
	if err != nil {
		return nil, err
	}
	if report == nil {
		return connect.NewResponse(&typesv1.LabelNamesResponse{}), nil
	}
	return connect.NewResponse(&typesv1.LabelNamesResponse{Names: report.LabelNames.LabelNames}), nil
}

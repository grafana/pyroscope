package frontend

import (
	"context"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/common/model"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
	"github.com/grafana/pyroscope/pkg/validation"
)

func (f *Frontend) AnalyzeQuery(ctx context.Context,
	c *connect.Request[querierv1.AnalyzeQueryRequest]) (
	*connect.Response[querierv1.AnalyzeQueryResponse], error,
) {
	opentracing.SpanFromContext(ctx)

	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	interval, ok := phlaremodel.GetTimeRange(c.Msg)
	if ok {
		validated, err := validation.ValidateRangeRequest(f.limits, tenantIDs, interval, model.Now())
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		if validated.IsEmpty {
			return connect.NewResponse(&querierv1.AnalyzeQueryResponse{}), nil
		}
		c.Msg.Start = int64(validated.Start)
		c.Msg.End = int64(validated.End)
	} else {
		return connect.NewResponse(&querierv1.AnalyzeQueryResponse{}), nil
	}

	ctx = connectgrpc.WithProcedure(ctx, querierv1connect.QuerierServiceAnalyzeQueryProcedure)
	return connectgrpc.RoundTripUnary[querierv1.AnalyzeQueryRequest, querierv1.AnalyzeQueryResponse](ctx, f, c)
}

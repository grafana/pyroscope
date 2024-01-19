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

func (f *Frontend) ProfileTypes(ctx context.Context, c *connect.Request[querierv1.ProfileTypesRequest]) (*connect.Response[querierv1.ProfileTypesResponse], error) {
	opentracing.SpanFromContext(ctx).
		SetTag("start", model.Time(c.Msg.Start).Time().String()).
		SetTag("end", model.Time(c.Msg.End).Time().String())

	ctx = connectgrpc.WithProcedure(ctx, querierv1connect.QuerierServiceProfileTypesProcedure)

	interval, ok := phlaremodel.GetTimeRange(c.Msg)
	if ok {
		tenantIDs, err := tenant.TenantIDs(ctx)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}

		validated, err := validation.ValidateRangeRequest(f.limits, tenantIDs, interval, model.Now())
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		if validated.IsEmpty {
			return connect.NewResponse(&querierv1.ProfileTypesResponse{}), nil
		}
		c.Msg.Start = int64(validated.Start)
		c.Msg.End = int64(validated.End)
	}

	return connectgrpc.RoundTripUnary[querierv1.ProfileTypesRequest, querierv1.ProfileTypesResponse](ctx, f, c)
}

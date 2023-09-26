package frontend

import (
	"context"

	"github.com/bufbuild/connect-go"
	"github.com/grafana/dskit/tenant"
	"github.com/prometheus/common/model"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
	"github.com/grafana/pyroscope/pkg/validation"
)

func (f *Frontend) Series(ctx context.Context, c *connect.Request[querierv1.SeriesRequest]) (*connect.Response[querierv1.SeriesResponse], error) {
	ctx = connectgrpc.WithProcedure(ctx, querierv1connect.QuerierServiceSeriesProcedure)

	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	// TODO(bryan) we probably want to skip this validation of start/end are 0
	// (indicating a legacy request).
	interval := model.Interval{
		Start: model.TimeFromUnix(c.Msg.Start),
		End:   model.TimeFromUnix(c.Msg.End),
	}
	validated, err := validation.ValidateRangeRequest(f.limits, tenantIDs, interval, model.Now())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if validated.IsEmpty {
		return connect.NewResponse(&querierv1.SeriesResponse{}), nil
	}
	c.Msg.Start = validated.Start.Unix()
	c.Msg.End = validated.End.Unix()

	return connectgrpc.RoundTripUnary[querierv1.SeriesRequest, querierv1.SeriesResponse](ctx, f, c)
}

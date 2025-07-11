package frontend

import (
	"context"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"
	"github.com/prometheus/common/model"

	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
	"github.com/grafana/pyroscope/pkg/validation"
)

func (f *Frontend) LabelValues(
	ctx context.Context,
	c *connect.Request[typesv1.LabelValuesRequest],
) (*connect.Response[typesv1.LabelValuesResponse], error) {
	ctx = connectgrpc.WithProcedure(ctx, querierv1connect.QuerierServiceLabelValuesProcedure)

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
			return connect.NewResponse(&typesv1.LabelValuesResponse{}), nil
		}
		c.Msg.Start = int64(validated.Start)
		c.Msg.End = int64(validated.End)
	}

	return connectgrpc.RoundTripUnary[typesv1.LabelValuesRequest, typesv1.LabelValuesResponse](ctx, f, c)
}

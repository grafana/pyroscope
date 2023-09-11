package frontend

import (
	"context"

	"github.com/bufbuild/connect-go"

	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
)

func (f *Frontend) LabelValues(ctx context.Context, c *connect.Request[typesv1.LabelValuesRequest]) (*connect.Response[typesv1.LabelValuesResponse], error) {
	ctx = connectgrpc.WithProcedure(ctx, querierv1connect.QuerierServiceLabelValuesProcedure)
	return connectgrpc.RoundTripUnary[typesv1.LabelValuesRequest, typesv1.LabelValuesResponse](ctx, f, c)
}

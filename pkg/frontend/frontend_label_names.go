package frontend

import (
	"context"

	"github.com/bufbuild/connect-go"

	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
)

func (f *Frontend) LabelNames(ctx context.Context, c *connect.Request[typesv1.LabelNamesRequest]) (*connect.Response[typesv1.LabelNamesResponse], error) {
	ctx = connectgrpc.WithProcedure(ctx, querierv1connect.QuerierServiceLabelNamesProcedure)
	return connectgrpc.RoundTripUnary[typesv1.LabelNamesRequest, typesv1.LabelNamesResponse](ctx, f, c)
}

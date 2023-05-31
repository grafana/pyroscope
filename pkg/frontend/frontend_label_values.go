package frontend

import (
	"context"

	"github.com/bufbuild/connect-go"

	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	"github.com/grafana/phlare/pkg/util/connectgrpc"
)

func (f *Frontend) LabelValues(ctx context.Context, c *connect.Request[typesv1.LabelValuesRequest]) (*connect.Response[typesv1.LabelValuesResponse], error) {
	return connectgrpc.RoundTripUnary[typesv1.LabelValuesRequest, typesv1.LabelValuesResponse](ctx, f, c)
}

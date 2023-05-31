package frontend

import (
	"context"

	"github.com/bufbuild/connect-go"

	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	"github.com/grafana/phlare/pkg/util/connectgrpc"
)

func (f *Frontend) LabelNames(ctx context.Context, c *connect.Request[typesv1.LabelNamesRequest]) (*connect.Response[typesv1.LabelNamesResponse], error) {
	return connectgrpc.RoundTripUnary[typesv1.LabelNamesRequest, typesv1.LabelNamesResponse](ctx, f, c)
}

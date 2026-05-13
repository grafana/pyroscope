package querier

import (
	"context"

	"connectrpc.com/connect"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	connectapi "github.com/grafana/pyroscope/v2/pkg/api/connect"
	"github.com/grafana/pyroscope/v2/pkg/util/connectgrpc"
)

// grpcRoundTripperHandler adapts a QuerierServiceClient for use as a
// QuerierServiceHandler on the V1 read path. Server-streaming RPCs are not
// supported on V1 and return CodeUnimplemented.
type grpcRoundTripperHandler struct {
	querierv1connect.QuerierServiceClient
}

func (g *grpcRoundTripperHandler) SelectMergeStacktracesStream(
	_ context.Context,
	_ *connect.Request[querierv1.SelectMergeStacktracesRequest],
	_ *connect.ServerStream[querierv1.SelectMergeStacktracesPartial],
) error {
	return connect.NewError(connect.CodeUnimplemented, nil)
}

func (g *grpcRoundTripperHandler) SelectSeriesStream(
	_ context.Context,
	_ *connect.Request[querierv1.SelectSeriesRequest],
	_ *connect.ServerStream[querierv1.SelectSeriesPartial],
) error {
	return connect.NewError(connect.CodeUnimplemented, nil)
}

func NewGRPCRoundTripper(transport connectgrpc.GRPCRoundTripper) querierv1connect.QuerierServiceHandler {
	return &grpcRoundTripperHandler{
		QuerierServiceClient: querierv1connect.NewQuerierServiceClient(
			connectgrpc.NewClient(transport),
			"http://httpgrpc",
			append(
				connectapi.DefaultClientOptions(),
				connect.WithGRPCWeb(),
			)...,
		),
	}
}

package querier

import (
	"context"

	"github.com/bufbuild/connect-go"

	googlev1 "github.com/grafana/phlare/api/gen/proto/go/google/v1"
	querierv1 "github.com/grafana/phlare/api/gen/proto/go/querier/v1"
	"github.com/grafana/phlare/api/gen/proto/go/querier/v1/querierv1connect"
	"github.com/grafana/phlare/pkg/util/connectgrpc"
)

// todo: this could be generated.
type grpcRoundTripper struct {
	connectgrpc.GRPCRoundTripper
}

func NewGRPCRoundTripper(transport connectgrpc.GRPCRoundTripper) querierv1connect.QuerierServiceHandler {
	return &grpcRoundTripper{transport}
}

func (f *grpcRoundTripper) ProfileTypes(ctx context.Context, in *connect.Request[querierv1.ProfileTypesRequest]) (*connect.Response[querierv1.ProfileTypesResponse], error) {
	return connectgrpc.RoundTripUnary[querierv1.ProfileTypesRequest, querierv1.ProfileTypesResponse](f, ctx, in)
}

func (f *grpcRoundTripper) LabelValues(ctx context.Context, in *connect.Request[querierv1.LabelValuesRequest]) (*connect.Response[querierv1.LabelValuesResponse], error) {
	return connectgrpc.RoundTripUnary[querierv1.LabelValuesRequest, querierv1.LabelValuesResponse](f, ctx, in)
}

func (f *grpcRoundTripper) LabelNames(ctx context.Context, in *connect.Request[querierv1.LabelNamesRequest]) (*connect.Response[querierv1.LabelNamesResponse], error) {
	return connectgrpc.RoundTripUnary[querierv1.LabelNamesRequest, querierv1.LabelNamesResponse](f, ctx, in)
}

func (f *grpcRoundTripper) Series(ctx context.Context, in *connect.Request[querierv1.SeriesRequest]) (*connect.Response[querierv1.SeriesResponse], error) {
	return connectgrpc.RoundTripUnary[querierv1.SeriesRequest, querierv1.SeriesResponse](f, ctx, in)
}

func (f *grpcRoundTripper) SelectMergeStacktraces(ctx context.Context, in *connect.Request[querierv1.SelectMergeStacktracesRequest]) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
	return connectgrpc.RoundTripUnary[querierv1.SelectMergeStacktracesRequest, querierv1.SelectMergeStacktracesResponse](f, ctx, in)
}

func (f *grpcRoundTripper) SelectMergeProfile(ctx context.Context, in *connect.Request[querierv1.SelectMergeProfileRequest]) (*connect.Response[googlev1.Profile], error) {
	return connectgrpc.RoundTripUnary[querierv1.SelectMergeProfileRequest, googlev1.Profile](f, ctx, in)
}

func (f *grpcRoundTripper) SelectSeries(ctx context.Context, in *connect.Request[querierv1.SelectSeriesRequest]) (*connect.Response[querierv1.SelectSeriesResponse], error) {
	return connectgrpc.RoundTripUnary[querierv1.SelectSeriesRequest, querierv1.SelectSeriesResponse](f, ctx, in)
}

package testutil

import (
	"context"
	"net/http"
	"testing"

	"connectrpc.com/connect"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"

	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1/ingesterv1connect"
	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	connectapi "github.com/grafana/pyroscope/pkg/api/connect"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/grafana/pyroscope/pkg/testhelper"
)

// Copied from phlaredb, todo
type IngesterHandlerPhlareDB struct {
	phlaredb.Queriers
}

func IngesterClientForTest(t *testing.T, q phlaredb.Queriers) (ingesterv1connect.IngesterServiceClient, func()) {
	assert.NotEmpty(t, q)
	mux := http.NewServeMux()
	mux.Handle(ingesterv1connect.NewIngesterServiceHandler(&IngesterHandlerPhlareDB{q}, connectapi.DefaultHandlerOptions()...))
	serv := testhelper.NewInMemoryServer(mux)

	var httpClient = serv.Client()

	client := ingesterv1connect.NewIngesterServiceClient(
		httpClient, serv.URL(), connectapi.DefaultClientOptions()...,
	)

	return client, serv.Close
}

func (i *IngesterHandlerPhlareDB) MergeProfilesStacktraces(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesStacktracesRequest, ingestv1.MergeProfilesStacktracesResponse]) error {
	return phlaredb.MergeProfilesStacktraces(ctx, stream, i.ForTimeRange)
}

func (i *IngesterHandlerPhlareDB) MergeProfilesLabels(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesLabelsRequest, ingestv1.MergeProfilesLabelsResponse]) error {
	return phlaredb.MergeProfilesLabels(ctx, stream, i.ForTimeRange)
}

func (i *IngesterHandlerPhlareDB) MergeProfilesPprof(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesPprofRequest, ingestv1.MergeProfilesPprofResponse]) error {
	return phlaredb.MergeProfilesPprof(ctx, stream, i.ForTimeRange)
}

func (i *IngesterHandlerPhlareDB) MergeSpanProfile(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeSpanProfileRequest, ingestv1.MergeSpanProfileResponse]) error {
	return phlaredb.MergeSpanProfile(ctx, stream, i.ForTimeRange)
}

func (i *IngesterHandlerPhlareDB) Push(context.Context, *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
	return nil, errors.New("not implemented")
}

func (i *IngesterHandlerPhlareDB) LabelValues(context.Context, *connect.Request[typesv1.LabelValuesRequest]) (*connect.Response[typesv1.LabelValuesResponse], error) {
	return nil, errors.New("not implemented")
}

func (i *IngesterHandlerPhlareDB) LabelNames(context.Context, *connect.Request[typesv1.LabelNamesRequest]) (*connect.Response[typesv1.LabelNamesResponse], error) {
	return nil, errors.New("not implemented")
}

func (i *IngesterHandlerPhlareDB) ProfileTypes(context.Context, *connect.Request[ingestv1.ProfileTypesRequest]) (*connect.Response[ingestv1.ProfileTypesResponse], error) {
	return nil, errors.New("not implemented")
}

func (i *IngesterHandlerPhlareDB) Series(context.Context, *connect.Request[ingestv1.SeriesRequest]) (*connect.Response[ingestv1.SeriesResponse], error) {
	return nil, errors.New("not implemented")
}

func (i *IngesterHandlerPhlareDB) Flush(context.Context, *connect.Request[ingestv1.FlushRequest]) (*connect.Response[ingestv1.FlushResponse], error) {
	return nil, errors.New("not implemented")
}

func (i *IngesterHandlerPhlareDB) BlockMetadata(context.Context, *connect.Request[ingestv1.BlockMetadataRequest]) (*connect.Response[ingestv1.BlockMetadataResponse], error) {
	return nil, errors.New("not implemented")
}

func (i *IngesterHandlerPhlareDB) GetProfileStats(context.Context, *connect.Request[typesv1.GetProfileStatsRequest]) (*connect.Response[typesv1.GetProfileStatsResponse], error) {
	return nil, errors.New("not implemented")
}

func (i *IngesterHandlerPhlareDB) GetBlockStats(context.Context, *connect.Request[ingestv1.GetBlockStatsRequest]) (*connect.Response[ingestv1.GetBlockStatsResponse], error) {
	return nil, errors.New("not implemented")
}

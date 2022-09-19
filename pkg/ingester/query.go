package ingester

import (
	"context"
	"fmt"

	"github.com/apache/arrow/go/v8/arrow"
	"github.com/apache/arrow/go/v8/arrow/array"
	"github.com/bufbuild/connect-go"

	ingestv1 "github.com/grafana/fire/pkg/gen/ingester/v1"
)

// LabelValues returns the possible label values for a given label name.
func (i *Ingester) LabelValues(ctx context.Context, req *connect.Request[ingestv1.LabelValuesRequest]) (*connect.Response[ingestv1.LabelValuesResponse], error) {
	return i.fireDB.Head().LabelValues(ctx, req)
}

// LabelValues returns the possible label names.
func (i *Ingester) LabelNames(ctx context.Context, req *connect.Request[ingestv1.LabelNamesRequest]) (*connect.Response[ingestv1.LabelNamesResponse], error) {
	return i.fireDB.Head().LabelNames(ctx, req)
}

// ProfileTypes returns the possible profile types.
func (i *Ingester) ProfileTypes(ctx context.Context, req *connect.Request[ingestv1.ProfileTypesRequest]) (*connect.Response[ingestv1.ProfileTypesResponse], error) {
	return i.fireDB.Head().ProfileTypes(ctx, req)
}

// Series returns labels series for the given set of matchers.
func (i *Ingester) Series(ctx context.Context, req *connect.Request[ingestv1.SeriesRequest]) (*connect.Response[ingestv1.SeriesResponse], error) {
	return i.fireDB.Head().Series(ctx, req)
}

func (i *Ingester) SelectProfiles(ctx context.Context, req *connect.Request[ingestv1.SelectProfilesRequest]) (*connect.Response[ingestv1.SelectProfilesResponse], error) {
	return i.fireDB.SelectProfiles(ctx, req)
}

func (i *Ingester) MergeProfilesStacktraces(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesStacktracesRequest, ingestv1.MergeProfilesStacktracesResponse]) error {
	return i.fireDB.MergeProfilesStacktraces(ctx, stream)
}

func (i *Ingester) MergeProfilesLabels(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesLabelsRequest, ingestv1.MergeProfilesLabelsResponse]) error {
	return i.fireDB.MergeProfilesLabels(ctx, stream)
}

func binaryFieldFromRecord(ar arrow.Record, name string) (*array.Binary, error) {
	indices := ar.Schema().FieldIndices(name)
	if len(indices) != 1 {
		return nil, fmt.Errorf("expected 1 column named %q, got %d", name, len(indices))
	}

	col, ok := ar.Column(indices[0]).(*array.Binary)
	if !ok {
		return nil, fmt.Errorf("expected column %q to be a binary column, got %T", name, ar.Column(indices[0]))
	}

	return col, nil
}

func int64FieldFromRecord(ar arrow.Record, name string) (*array.Int64, error) {
	indices := ar.Schema().FieldIndices(name)
	if len(indices) != 1 {
		return nil, fmt.Errorf("expected 1 column named %q, got %d", name, len(indices))
	}

	col, ok := ar.Column(indices[0]).(*array.Int64)
	if !ok {
		return nil, fmt.Errorf("expected column %q to be a int64 column, got %T", name, ar.Column(indices[0]))
	}

	return col, nil
}

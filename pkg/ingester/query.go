package ingester

import (
	"context"

	"github.com/bufbuild/connect-go"

	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
)

// LabelValues returns the possible label values for a given label name.
func (i *Ingester) LabelValues(ctx context.Context, req *connect.Request[ingestv1.LabelValuesRequest]) (*connect.Response[ingestv1.LabelValuesResponse], error) {
	return forInstanceUnary(ctx, i, func(instance *instance) (*connect.Response[ingestv1.LabelValuesResponse], error) {
		return instance.Head().LabelValues(ctx, req)
	})
}

// LabelValues returns the possible label names.
func (i *Ingester) LabelNames(ctx context.Context, req *connect.Request[ingestv1.LabelNamesRequest]) (*connect.Response[ingestv1.LabelNamesResponse], error) {
	return forInstanceUnary(ctx, i, func(instance *instance) (*connect.Response[ingestv1.LabelNamesResponse], error) {
		return instance.Head().LabelNames(ctx, req)
	})
}

// ProfileTypes returns the possible profile types.
func (i *Ingester) ProfileTypes(ctx context.Context, req *connect.Request[ingestv1.ProfileTypesRequest]) (*connect.Response[ingestv1.ProfileTypesResponse], error) {
	return forInstanceUnary(ctx, i, func(instance *instance) (*connect.Response[ingestv1.ProfileTypesResponse], error) {
		return instance.Head().ProfileTypes(ctx, req)
	})
}

// Series returns labels series for the given set of matchers.
func (i *Ingester) Series(ctx context.Context, req *connect.Request[ingestv1.SeriesRequest]) (*connect.Response[ingestv1.SeriesResponse], error) {
	return forInstanceUnary(ctx, i, func(instance *instance) (*connect.Response[ingestv1.SeriesResponse], error) {
		return instance.Head().Series(ctx, req)
	})
}

func (i *Ingester) MergeProfilesStacktraces(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesStacktracesRequest, ingestv1.MergeProfilesStacktracesResponse]) error {
	return i.forInstance(ctx, func(instance *instance) error {
		return instance.MergeProfilesStacktraces(ctx, stream)
	})
}

func (i *Ingester) MergeProfilesLabels(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesLabelsRequest, ingestv1.MergeProfilesLabelsResponse]) error {
	return i.forInstance(ctx, func(instance *instance) error {
		return instance.MergeProfilesLabels(ctx, stream)
	})
}

func (i *Ingester) MergeProfilesPprof(ctx context.Context, stream *connect.BidiStream[ingestv1.MergeProfilesPprofRequest, ingestv1.MergeProfilesPprofResponse]) error {
	return i.forInstance(ctx, func(instance *instance) error {
		return instance.MergeProfilesPprof(ctx, stream)
	})
}

package readpath

import (
	"context"
	"slices"

	"connectrpc.com/connect"
	"golang.org/x/sync/errgroup"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/querier/v1/querierv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/pprof"
)

var _ querierv1connect.QuerierServiceHandler = (*Router)(nil)

func (r *Router) LabelValues(
	ctx context.Context,
	c *connect.Request[typesv1.LabelValuesRequest],
) (*connect.Response[typesv1.LabelValuesResponse], error) {
	return Query[typesv1.LabelValuesRequest, typesv1.LabelValuesResponse](ctx, r, c,
		func(_, _ *typesv1.LabelValuesRequest) {},
		func(a, b *typesv1.LabelValuesResponse) (*typesv1.LabelValuesResponse, error) {
			m := phlaremodel.NewLabelMerger()
			m.MergeLabelValues(a.Names)
			m.MergeLabelValues(b.Names)
			return &typesv1.LabelValuesResponse{Names: m.LabelValues()}, nil
		})
}

func (r *Router) LabelNames(
	ctx context.Context,
	c *connect.Request[typesv1.LabelNamesRequest],
) (*connect.Response[typesv1.LabelNamesResponse], error) {
	return Query[typesv1.LabelNamesRequest, typesv1.LabelNamesResponse](ctx, r, c,
		func(_, _ *typesv1.LabelNamesRequest) {},
		func(a, b *typesv1.LabelNamesResponse) (*typesv1.LabelNamesResponse, error) {
			m := phlaremodel.NewLabelMerger()
			m.MergeLabelNames(a.Names)
			m.MergeLabelNames(b.Names)
			return &typesv1.LabelNamesResponse{Names: m.LabelNames()}, nil
		})
}

func (r *Router) Series(
	ctx context.Context,
	c *connect.Request[querierv1.SeriesRequest],
) (*connect.Response[querierv1.SeriesResponse], error) {
	return Query[querierv1.SeriesRequest, querierv1.SeriesResponse](ctx, r, c,
		func(_, _ *querierv1.SeriesRequest) {},
		func(a, b *querierv1.SeriesResponse) (*querierv1.SeriesResponse, error) {
			m := phlaremodel.NewLabelMerger()
			m.MergeSeries(a.LabelsSet)
			m.MergeSeries(b.LabelsSet)
			return &querierv1.SeriesResponse{LabelsSet: m.Labels()}, nil
		})
}

func (r *Router) SelectMergeStacktraces(
	ctx context.Context,
	c *connect.Request[querierv1.SelectMergeStacktracesRequest],
) (*connect.Response[querierv1.SelectMergeStacktracesResponse], error) {
	// We always query data in the tree format and
	// return it in the format requested by the client.
	f := c.Msg.Format
	c.Msg.Format = querierv1.ProfileFormat_PROFILE_FORMAT_TREE
	resp, err := Query[querierv1.SelectMergeStacktracesRequest, querierv1.SelectMergeStacktracesResponse](ctx, r, c,
		func(_, _ *querierv1.SelectMergeStacktracesRequest) {},
		func(a, b *querierv1.SelectMergeStacktracesResponse) (*querierv1.SelectMergeStacktracesResponse, error) {
			m := phlaremodel.NewTreeMerger()
			if err := m.MergeTreeBytes(a.Tree); err != nil {
				return nil, err
			}
			if err := m.MergeTreeBytes(b.Tree); err != nil {
				return nil, err
			}
			tree := m.Tree().Bytes(c.Msg.GetMaxNodes())
			return &querierv1.SelectMergeStacktracesResponse{Tree: tree}, nil
		},
	)
	if err == nil && f != c.Msg.Format {
		resp.Msg.Flamegraph = phlaremodel.NewFlameGraph(
			phlaremodel.MustUnmarshalTree(resp.Msg.Tree),
			c.Msg.GetMaxNodes())
	}
	return resp, err
}

func (r *Router) SelectMergeSpanProfile(
	ctx context.Context,
	c *connect.Request[querierv1.SelectMergeSpanProfileRequest],
) (*connect.Response[querierv1.SelectMergeSpanProfileResponse], error) {
	// We always query data in the tree format and
	// return it in the format requested by the client.
	f := c.Msg.Format
	c.Msg.Format = querierv1.ProfileFormat_PROFILE_FORMAT_TREE
	resp, err := Query[querierv1.SelectMergeSpanProfileRequest, querierv1.SelectMergeSpanProfileResponse](ctx, r, c,
		func(_, _ *querierv1.SelectMergeSpanProfileRequest) {},
		func(a, b *querierv1.SelectMergeSpanProfileResponse) (*querierv1.SelectMergeSpanProfileResponse, error) {
			m := phlaremodel.NewTreeMerger()
			if err := m.MergeTreeBytes(a.Tree); err != nil {
				return nil, err
			}
			if err := m.MergeTreeBytes(b.Tree); err != nil {
				return nil, err
			}
			tree := m.Tree().Bytes(c.Msg.GetMaxNodes())
			return &querierv1.SelectMergeSpanProfileResponse{Tree: tree}, nil
		},
	)
	if err == nil && f != c.Msg.Format {
		resp.Msg.Flamegraph = phlaremodel.NewFlameGraph(
			phlaremodel.MustUnmarshalTree(resp.Msg.Tree),
			c.Msg.GetMaxNodes())
	}
	return resp, err
}

func (r *Router) SelectMergeProfile(
	ctx context.Context,
	c *connect.Request[querierv1.SelectMergeProfileRequest],
) (*connect.Response[profilev1.Profile], error) {
	return Query[querierv1.SelectMergeProfileRequest, profilev1.Profile](ctx, r, c,
		func(_, _ *querierv1.SelectMergeProfileRequest) {},
		func(a, b *profilev1.Profile) (*profilev1.Profile, error) {
			var m pprof.ProfileMerge
			if err := m.Merge(a, false); err != nil {
				return nil, err
			}
			if err := m.Merge(b, false); err != nil {
				return nil, err
			}
			return m.Profile(), nil
		})
}

func (r *Router) SelectSeries(
	ctx context.Context,
	c *connect.Request[querierv1.SelectSeriesRequest],
) (*connect.Response[querierv1.SelectSeriesResponse], error) {
	limit := int(c.Msg.GetLimit())
	return Query[querierv1.SelectSeriesRequest, querierv1.SelectSeriesResponse](ctx, r, c,
		func(a, b *querierv1.SelectSeriesRequest) {
			// If both frontends are queries, the limit must
			// be applied after merging the time series, as
			// the function is not associative. Otherwise, each
			// frontend applies the limit independently.
			if a != nil && b != nil && limit > 0 {
				a.Limit = nil
				b.Limit = nil
			}
		},
		func(a, b *querierv1.SelectSeriesResponse) (*querierv1.SelectSeriesResponse, error) {
			m := phlaremodel.NewTimeSeriesMerger(true)
			m.MergeTimeSeries(a.Series)
			m.MergeTimeSeries(b.Series)
			return &querierv1.SelectSeriesResponse{Series: m.Top(limit)}, nil
		})
}

func (r *Router) Diff(
	ctx context.Context,
	c *connect.Request[querierv1.DiffRequest],
) (*connect.Response[querierv1.DiffResponse], error) {
	g, ctx := errgroup.WithContext(ctx)
	getTree := func(dst *phlaremodel.Tree, req *querierv1.SelectMergeStacktracesRequest) func() error {
		return func() error {
			resp, err := r.SelectMergeStacktraces(ctx, connect.NewRequest(req))
			if err != nil {
				return err
			}
			tree, err := phlaremodel.UnmarshalTree(resp.Msg.Tree)
			if err != nil {
				return err
			}
			*dst = *tree
			return nil
		}
	}

	var left, right phlaremodel.Tree
	g.Go(getTree(&left, c.Msg.Left))
	g.Go(getTree(&right, c.Msg.Right))
	if err := g.Wait(); err != nil {
		return nil, err
	}

	diff, err := phlaremodel.NewFlamegraphDiff(&left, &right, 0)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&querierv1.DiffResponse{Flamegraph: diff}), nil
}

// Stubs: these methods are not supposed to be implemented
// and only needed to satisfy interfaces.

func (r *Router) AnalyzeQuery(
	ctx context.Context,
	req *connect.Request[querierv1.AnalyzeQueryRequest],
) (*connect.Response[querierv1.AnalyzeQueryResponse], error) {
	if r.oldFrontend != nil {
		return r.oldFrontend.AnalyzeQuery(ctx, req)
	}
	return connect.NewResponse(&querierv1.AnalyzeQueryResponse{}), nil
}

func (r *Router) GetProfileStats(
	ctx context.Context,
	c *connect.Request[typesv1.GetProfileStatsRequest],
) (*connect.Response[typesv1.GetProfileStatsResponse], error) {
	return Query[typesv1.GetProfileStatsRequest, typesv1.GetProfileStatsResponse](ctx, r, c,
		func(_, _ *typesv1.GetProfileStatsRequest) {},
		func(a, b *typesv1.GetProfileStatsResponse) (*typesv1.GetProfileStatsResponse, error) {
			oldestProfileTime := a.OldestProfileTime
			newestProfileTime := a.NewestProfileTime
			if b.OldestProfileTime < oldestProfileTime {
				oldestProfileTime = b.OldestProfileTime
			}
			if b.NewestProfileTime > newestProfileTime {
				newestProfileTime = b.NewestProfileTime
			}
			return &typesv1.GetProfileStatsResponse{
				DataIngested:      a.DataIngested || b.DataIngested,
				OldestProfileTime: oldestProfileTime,
				NewestProfileTime: newestProfileTime,
			}, nil
		})
}

func (r *Router) ProfileTypes(
	ctx context.Context,
	c *connect.Request[querierv1.ProfileTypesRequest],
) (*connect.Response[querierv1.ProfileTypesResponse], error) {
	return Query[querierv1.ProfileTypesRequest, querierv1.ProfileTypesResponse](ctx, r, c,
		func(_, _ *querierv1.ProfileTypesRequest) {},
		func(a, b *querierv1.ProfileTypesResponse) (*querierv1.ProfileTypesResponse, error) {
			pTypes := a.ProfileTypes
			for _, pType := range b.ProfileTypes {
				if !slices.Contains(pTypes, pType) {
					pTypes = append(pTypes, pType)
				}
			}
			return &querierv1.ProfileTypesResponse{ProfileTypes: pTypes}, nil
		})
}

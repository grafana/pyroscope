package phlaredb

import (
	"context"
	"sort"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
	"github.com/opentracing/opentracing-go"
	"github.com/parquet-go/parquet-go"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb/query"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
)

type headOnDiskQuerier struct {
	head        *Head
	rowGroupIdx int
}

func (q *headOnDiskQuerier) rowGroup() *rowGroupOnDisk {
	q.head.profiles.rowsLock.RLock()
	defer q.head.profiles.rowsLock.RUnlock()
	return q.head.profiles.rowGroups[q.rowGroupIdx]
}

func (q *headOnDiskQuerier) Open(_ context.Context) error {
	return nil
}

func (q *headOnDiskQuerier) BlockID() string {
	return q.head.meta.ULID.String()
}

func (q *headOnDiskQuerier) SelectMatchingProfiles(ctx context.Context, params *ingestv1.SelectProfilesRequest) (iter.Iterator[Profile], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectMatchingProfiles - HeadOnDisk")
	defer sp.Finish()

	// query the index for rows
	rowIter, labelsPerFP, err := q.head.profiles.index.selectMatchingRowRanges(ctx, params, q.rowGroupIdx)
	if err != nil {
		return nil, err
	}

	// get time nano information for profiles
	var (
		start = model.Time(params.Start)
		end   = model.Time(params.End)
	)
	pIt := query.NewBinaryJoinIterator(0,
		query.NewBinaryJoinIterator(
			0,
			rowIter,
			q.rowGroup().columnIter(ctx, "TimeNanos", query.NewIntBetweenPredicate(start.UnixNano(), end.UnixNano()), "TimeNanos"),
		),
		q.rowGroup().columnIter(ctx, "StacktracePartition", nil, "StacktracePartition"),
	)
	defer pIt.Close()

	var (
		profiles []Profile
		buf      = make([][]parquet.Value, 2)
	)
	for pIt.Next() {
		res := pIt.At()

		v, ok := res.Entries[0].RowValue.(fingerprintWithRowNum)
		if !ok {
			panic("no fingerprint information found")
		}

		lbls, ok := labelsPerFP[v.fp]
		if !ok {
			panic("no profile series labels with matching fingerprint found")
		}

		buf = res.Columns(buf, "TimeNanos", "StacktracePartition")
		if len(buf) < 1 || len(buf[0]) != 1 {
			level.Error(q.head.logger).Log("msg", "unable to read timeNanos from profiles", "row", res.RowNumber[0], "rowGroup", q.rowGroupIdx)
			continue
		}
		profiles = append(profiles, BlockProfile{
			labels:      lbls,
			fingerprint: v.fp,
			timestamp:   model.TimeFromUnixNano(buf[0][0].Int64()),
			partition:   retrieveStacktracePartition(buf, 1),
			rowNum:      res.RowNumber[0],
		})
	}
	if err := pIt.Err(); err != nil {
		return nil, errors.Wrap(pIt.Err(), "iterator error")
	}

	// Sort profiles by time, the slice is already sorted by series order
	sort.Slice(profiles, func(i, j int) bool {
		a, b := profiles[i], profiles[j]
		if a.Timestamp() != b.Timestamp() {
			return a.Timestamp() < b.Timestamp()
		}
		return phlaremodel.CompareLabelPairs(a.Labels(), b.Labels()) < 0
	})

	return iter.NewSliceIterator(profiles), nil
}

func (q *headOnDiskQuerier) SelectMergeByStacktraces(ctx context.Context, params *ingestv1.SelectProfilesRequest) (*phlaremodel.Tree, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectMergeByStacktraces - HeadOnDisk")
	defer sp.Finish()

	// query the index for rows
	rowIter, _, err := q.head.profiles.index.selectMatchingRowRanges(ctx, params, q.rowGroupIdx)
	if err != nil {
		return nil, err
	}

	// get time nano information for profiles
	var (
		start = model.Time(params.Start)
		end   = model.Time(params.End)
	)
	it := query.NewBinaryJoinIterator(0,
		query.NewBinaryJoinIterator(
			0,
			rowIter,
			q.rowGroup().columnIter(ctx, "TimeNanos", query.NewIntBetweenPredicate(start.UnixNano(), end.UnixNano()), ""),
		),
		q.rowGroup().columnIter(ctx, "StacktracePartition", nil, "StacktracePartition"))

	rows := profileRowBatchIterator(it)
	defer rows.Close()

	r := symdb.NewResolver(ctx, q.head.symdb)
	defer r.Release()

	if err := mergeByStacktraces[rowProfile](ctx, q.rowGroup(), rows, r); err != nil {
		return nil, err
	}
	return r.Tree()
}

func (q *headOnDiskQuerier) SelectMergeBySpans(ctx context.Context, params *ingestv1.SelectSpanProfileRequest) (*phlaremodel.Tree, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectMergeBySpans - HeadOnDisk")
	defer sp.Finish()

	// query the index for rows
	rowIter, _, err := q.head.profiles.index.selectMatchingRowRanges(ctx, &ingestv1.SelectProfilesRequest{
		LabelSelector: params.LabelSelector,
		Type:          params.Type,
		Start:         params.Start,
		End:           params.End,
		Hints:         params.Hints,
	}, q.rowGroupIdx)
	if err != nil {
		return nil, err
	}
	spans, err := phlaremodel.NewSpanSelector(params.SpanSelector)
	if err != nil {
		return nil, err
	}

	// get time nano information for profiles
	var (
		start = model.Time(params.Start)
		end   = model.Time(params.End)
	)
	it := query.NewBinaryJoinIterator(0,
		query.NewBinaryJoinIterator(
			0,
			rowIter,
			q.rowGroup().columnIter(ctx, "TimeNanos", query.NewIntBetweenPredicate(start.UnixNano(), end.UnixNano()), ""),
		),
		q.rowGroup().columnIter(ctx, "StacktracePartition", nil, "StacktracePartition"))

	rows := profileRowBatchIterator(it)
	defer rows.Close()

	r := symdb.NewResolver(ctx, q.head.symdb)
	defer r.Release()

	if err = mergeBySpans[rowProfile](ctx, q.rowGroup(), rows, r, spans); err != nil {
		return nil, err
	}
	return r.Tree()
}

func (q *headOnDiskQuerier) SelectMergePprof(ctx context.Context, params *ingestv1.SelectProfilesRequest, maxNodes int64, sts *typesv1.StackTraceSelector) (*profilev1.Profile, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectMergePprof - HeadOnDisk")
	defer sp.Finish()

	// query the index for rows
	rowIter, _, err := q.head.profiles.index.selectMatchingRowRanges(ctx, params, q.rowGroupIdx)
	if err != nil {
		return nil, err
	}

	// get time nano information for profiles
	var (
		start = model.Time(params.Start)
		end   = model.Time(params.End)
	)
	it := query.NewBinaryJoinIterator(0,
		query.NewBinaryJoinIterator(
			0,
			rowIter,
			q.rowGroup().columnIter(ctx, "TimeNanos", query.NewIntBetweenPredicate(start.UnixNano(), end.UnixNano()), ""),
		),
		q.rowGroup().columnIter(ctx, "StacktracePartition", nil, "StacktracePartition"))

	rows := profileRowBatchIterator(it)
	defer rows.Close()

	r := symdb.NewResolver(ctx, q.head.symdb,
		symdb.WithResolverMaxNodes(maxNodes),
		symdb.WithResolverStackTraceSelector(sts))
	defer r.Release()

	if err = mergeByStacktraces[rowProfile](ctx, q.rowGroup(), rows, r); err != nil {
		return nil, err
	}
	return r.Pprof()
}

func (q *headOnDiskQuerier) Bounds() (model.Time, model.Time) {
	// TODO: Use per rowgroup information
	return q.head.Bounds()
}

func (q *headOnDiskQuerier) ProfileTypes(context.Context, *connect.Request[ingestv1.ProfileTypesRequest]) (*connect.Response[ingestv1.ProfileTypesResponse], error) {
	return connect.NewResponse(&ingestv1.ProfileTypesResponse{}), nil
}

func (q *headOnDiskQuerier) LabelValues(ctx context.Context, req *connect.Request[typesv1.LabelValuesRequest]) (*connect.Response[typesv1.LabelValuesResponse], error) {
	return connect.NewResponse(&typesv1.LabelValuesResponse{}), nil
}

func (q *headOnDiskQuerier) LabelNames(ctx context.Context, req *connect.Request[typesv1.LabelNamesRequest]) (*connect.Response[typesv1.LabelNamesResponse], error) {
	return connect.NewResponse(&typesv1.LabelNamesResponse{}), nil
}

func (q *headOnDiskQuerier) MergeByStacktraces(ctx context.Context, rows iter.Iterator[Profile]) (*phlaremodel.Tree, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "MergeByStacktraces")
	defer sp.Finish()
	r := symdb.NewResolver(ctx, q.head.symdb)
	defer r.Release()
	if err := mergeByStacktraces(ctx, q.rowGroup(), rows, r); err != nil {
		return nil, err
	}
	return r.Tree()
}

func (q *headOnDiskQuerier) MergePprof(ctx context.Context, rows iter.Iterator[Profile], maxNodes int64, sts *typesv1.StackTraceSelector) (*profilev1.Profile, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "MergePprof")
	defer sp.Finish()
	r := symdb.NewResolver(ctx, q.head.symdb,
		symdb.WithResolverMaxNodes(maxNodes),
		symdb.WithResolverStackTraceSelector(sts))
	defer r.Release()
	if err := mergeByStacktraces(ctx, q.rowGroup(), rows, r); err != nil {
		return nil, err
	}
	return r.Pprof()
}

func (q *headOnDiskQuerier) MergeByLabels(ctx context.Context, rows iter.Iterator[Profile], sts *typesv1.StackTraceSelector, by ...string) ([]*typesv1.Series, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "MergeByLabels - HeadOnDisk")
	defer sp.Finish()
	if len(sts.GetCallSite()) == 0 {
		return mergeByLabels(ctx, q.rowGroup(), "TotalValue", rows, by...)
	}
	r := symdb.NewResolver(ctx, q.head.symdb,
		symdb.WithResolverStackTraceSelector(sts))
	defer r.Release()
	return mergeByLabelsWithStackTraceSelector(ctx, q.rowGroup(), rows, r, by...)
}

func (q *headOnDiskQuerier) SelectMergeByLabels(ctx context.Context, params *ingestv1.SelectProfilesRequest, sts *typesv1.StackTraceSelector, by ...string) ([]*typesv1.Series, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectMergeByLabels - HeadOnDisk")
	defer sp.Finish()

	// query the index for rows
	rowIter, labelsPerFP, err := q.head.profiles.index.selectMatchingRowRanges(ctx, params, q.rowGroupIdx)
	if err != nil {
		return nil, err
	}

	// get time nano information for profiles
	var (
		start = model.Time(params.Start).UnixNano()
		end   = model.Time(params.End).UnixNano()
	)
	it := query.NewBinaryJoinIterator(
		0,
		rowIter,
		q.rowGroup().columnIter(ctx, "TimeNanos", query.NewIntBetweenPredicate(start, end), "TimeNanos"),
	)

	if len(sts.GetCallSite()) == 0 {
		rows := profileBatchIteratorByFingerprints(it, labelsPerFP)
		defer rows.Close()
		return mergeByLabels[Profile](ctx, q.rowGroup(), "TotalValue", rows, by...)
	}

	r := symdb.NewResolver(ctx, q.head.symdb,
		symdb.WithResolverStackTraceSelector(sts))
	defer r.Release()

	it = query.NewBinaryJoinIterator(0, it, q.rowGroup().columnIter(ctx, "StacktracePartition", nil, "StacktracePartition"))
	rows := profileBatchIteratorByFingerprints(it, labelsPerFP)
	defer rows.Close()

	return mergeByLabelsWithStackTraceSelector[Profile](ctx, q.rowGroup(), rows, r, by...)
}

func (q *headOnDiskQuerier) Series(ctx context.Context, params *ingestv1.SeriesRequest) ([]*typesv1.Labels, error) {
	// The TSDB is kept in memory until the head block is flushed to disk.
	return []*typesv1.Labels{}, nil
}

func (q *headOnDiskQuerier) MergeBySpans(ctx context.Context, rows iter.Iterator[Profile], spanSelector phlaremodel.SpanSelector) (*phlaremodel.Tree, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "MergeBySpans")
	defer sp.Finish()
	r := symdb.NewResolver(ctx, q.head.symdb)
	defer r.Release()
	if err := mergeBySpans(ctx, q.rowGroup(), rows, r, spanSelector); err != nil {
		return nil, err
	}
	return r.Tree()
}

func (q *headOnDiskQuerier) Sort(in []Profile) []Profile {
	var rowI, rowJ int64
	sort.Slice(in, func(i, j int) bool {
		if pI, ok := in[i].(BlockProfile); ok {
			rowI = pI.rowNum
		}
		if pJ, ok := in[j].(BlockProfile); ok {
			rowJ = pJ.rowNum
		}
		return rowI < rowJ
	})
	return in
}

type headInMemoryQuerier struct {
	head *Head
}

func (q *headInMemoryQuerier) Open(_ context.Context) error {
	return nil
}

func (q *headInMemoryQuerier) BlockID() string {
	return q.head.meta.ULID.String()
}

func (q *headInMemoryQuerier) SelectMatchingProfiles(ctx context.Context, params *ingestv1.SelectProfilesRequest) (iter.Iterator[Profile], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectMatchingProfiles - HeadInMemory")
	defer sp.Finish()

	index := q.head.profiles.index

	ids, err := index.selectMatchingFPs(ctx, params)
	if err != nil {
		return nil, err
	}

	// get time nano information for profiles
	var (
		start = model.Time(params.Start)
		end   = model.Time(params.End)
	)

	iters := make([]iter.Iterator[Profile], 0, len(ids))
	index.mutex.RLock()
	defer index.mutex.RUnlock()

	for _, fp := range ids {
		profileSeries, ok := index.profilesPerFP[fp]
		if !ok {
			continue
		}

		profiles := make([]*schemav1.InMemoryProfile, len(profileSeries.profiles))
		copy(profiles, profileSeries.profiles)

		iters = append(iters,
			NewSeriesIterator(
				profileSeries.lbs,
				profileSeries.fp,
				iter.NewTimeRangedIterator(iter.NewSliceIterator(profiles), start, end),
			),
		)
	}

	return iter.NewMergeIterator(maxBlockProfile, false, iters...), nil
}

func (q *headInMemoryQuerier) SelectMergeByStacktraces(ctx context.Context, params *ingestv1.SelectProfilesRequest) (*phlaremodel.Tree, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectMergeByStacktraces - HeadInMemory")
	defer sp.Finish()
	r := symdb.NewResolver(ctx, q.head.symdb)
	defer r.Release()
	index := q.head.profiles.index

	ids, err := index.selectMatchingFPs(ctx, params)
	if err != nil {
		return nil, err
	}

	// get time nano information for profiles
	var (
		start = model.Time(params.Start)
		end   = model.Time(params.End)
	)

	index.mutex.RLock()
	defer index.mutex.RUnlock()

	for _, fp := range ids {
		profileSeries, ok := index.profilesPerFP[fp]
		if !ok {
			continue
		}
		for _, p := range profileSeries.profiles {
			if p.Timestamp() < start {
				continue
			}
			if p.Timestamp() > end {
				break
			}
			r.AddSamples(p.StacktracePartition, p.Samples)
		}
	}
	return r.Tree()
}

func (q *headInMemoryQuerier) SelectMergeBySpans(ctx context.Context, params *ingestv1.SelectSpanProfileRequest) (*phlaremodel.Tree, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectMergeBySpans - HeadInMemory")
	defer sp.Finish()
	r := symdb.NewResolver(ctx, q.head.symdb)
	defer r.Release()
	index := q.head.profiles.index

	ids, err := index.selectMatchingFPs(ctx, &ingestv1.SelectProfilesRequest{
		LabelSelector: params.LabelSelector,
		Type:          params.Type,
		Start:         params.Start,
		End:           params.End,
		Hints:         params.Hints,
	})
	if err != nil {
		return nil, err
	}
	spans, err := phlaremodel.NewSpanSelector(params.SpanSelector)
	if err != nil {
		return nil, err
	}

	// get time nano information for profiles
	var (
		start = model.Time(params.Start)
		end   = model.Time(params.End)
	)

	index.mutex.RLock()
	defer index.mutex.RUnlock()

	for _, fp := range ids {
		profileSeries, ok := index.profilesPerFP[fp]
		if !ok {
			continue
		}
		for _, p := range profileSeries.profiles {
			if p.Timestamp() < start {
				continue
			}
			if p.Timestamp() > end {
				break
			}
			if len(p.Samples.Spans) > 0 {
				r.AddSamplesWithSpanSelector(p.StacktracePartition, p.Samples, spans)
			}
		}
	}
	return r.Tree()
}

func (q *headInMemoryQuerier) SelectMergePprof(ctx context.Context, params *ingestv1.SelectProfilesRequest, maxNodes int64, sts *typesv1.StackTraceSelector) (*profilev1.Profile, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectMergePprof - HeadInMemory")
	defer sp.Finish()
	r := symdb.NewResolver(ctx, q.head.symdb,
		symdb.WithResolverMaxNodes(maxNodes),
		symdb.WithResolverStackTraceSelector(sts))
	defer r.Release()
	index := q.head.profiles.index

	ids, err := index.selectMatchingFPs(ctx, params)
	if err != nil {
		return nil, err
	}

	// get time nano information for profiles
	var (
		start = model.Time(params.Start)
		end   = model.Time(params.End)
	)

	index.mutex.RLock()
	defer index.mutex.RUnlock()

	for _, fp := range ids {
		profileSeries, ok := index.profilesPerFP[fp]
		if !ok {
			continue
		}
		for _, p := range profileSeries.profiles {
			if p.Timestamp() < start {
				continue
			}
			if p.Timestamp() > end {
				break
			}
			r.AddSamples(p.StacktracePartition, p.Samples)
		}
	}
	return r.Pprof()
}

func (q *headInMemoryQuerier) Bounds() (model.Time, model.Time) {
	// TODO: Use per rowgroup information
	return q.head.Bounds()
}

func (q *headInMemoryQuerier) ProfileTypes(ctx context.Context, req *connect.Request[ingestv1.ProfileTypesRequest]) (*connect.Response[ingestv1.ProfileTypesResponse], error) {
	return q.head.ProfileTypes(ctx, req)
}

func (q *headInMemoryQuerier) LabelValues(ctx context.Context, req *connect.Request[typesv1.LabelValuesRequest]) (*connect.Response[typesv1.LabelValuesResponse], error) {
	return q.head.LabelValues(ctx, req)
}

func (q *headInMemoryQuerier) LabelNames(ctx context.Context, req *connect.Request[typesv1.LabelNamesRequest]) (*connect.Response[typesv1.LabelNamesResponse], error) {
	return q.head.LabelNames(ctx, req)
}

func (q *headInMemoryQuerier) MergeByStacktraces(ctx context.Context, rows iter.Iterator[Profile]) (*phlaremodel.Tree, error) {
	sp, _ := opentracing.StartSpanFromContext(ctx, "MergeByStacktraces - HeadInMemory")
	defer sp.Finish()
	r := symdb.NewResolver(ctx, q.head.symdb)
	defer r.Release()
	for rows.Next() {
		p, ok := rows.At().(ProfileWithLabels)
		if !ok {
			return nil, errors.New("expected ProfileWithLabels")
		}
		r.AddSamples(p.StacktracePartition(), p.Samples())
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return r.Tree()
}

func (q *headInMemoryQuerier) MergePprof(ctx context.Context, rows iter.Iterator[Profile], maxNodes int64, sts *typesv1.StackTraceSelector) (*profilev1.Profile, error) {
	sp, _ := opentracing.StartSpanFromContext(ctx, "MergePprof - HeadInMemory")
	defer sp.Finish()
	r := symdb.NewResolver(ctx, q.head.symdb,
		symdb.WithResolverMaxNodes(maxNodes),
		symdb.WithResolverStackTraceSelector(sts))
	defer r.Release()
	for rows.Next() {
		p, ok := rows.At().(ProfileWithLabels)
		if !ok {
			return nil, errors.New("expected ProfileWithLabels")
		}
		r.AddSamples(p.StacktracePartition(), p.Samples())
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return r.Pprof()
}

func (q *headInMemoryQuerier) MergeByLabels(
	ctx context.Context,
	rows iter.Iterator[Profile],
	sts *typesv1.StackTraceSelector,
	by ...string,
) ([]*typesv1.Series, error) {
	sp, _ := opentracing.StartSpanFromContext(ctx, "MergeByLabels - HeadInMemory")
	defer sp.Finish()

	seriesBuilder := seriesBuilder{}
	seriesBuilder.init(by...)

	if len(sts.GetCallSite()) == 0 {
		for rows.Next() {
			p, ok := rows.At().(ProfileWithLabels)
			if !ok {
				return nil, errors.New("expected ProfileWithLabels")
			}
			seriesBuilder.add(p.Fingerprint(), p.Labels(), int64(p.Timestamp()), float64(p.Total()))
		}
	} else {
		r := symdb.NewResolver(ctx, q.head.symdb,
			symdb.WithResolverStackTraceSelector(sts))
		defer r.Release()
		var v symdb.CallSiteValues
		for rows.Next() {
			p, ok := rows.At().(ProfileWithLabels)
			if !ok {
				return nil, errors.New("expected ProfileWithLabels")
			}
			if err := r.CallSiteValues(&v, p.StacktracePartition(), p.Samples()); err != nil {
				return nil, err
			}
			seriesBuilder.add(p.Fingerprint(), p.Labels(), int64(p.Timestamp()), float64(v.Total))
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return seriesBuilder.build(), nil
}

func (q *headInMemoryQuerier) SelectMergeByLabels(
	ctx context.Context,
	params *ingestv1.SelectProfilesRequest,
	sts *typesv1.StackTraceSelector,
	by ...string,
) ([]*typesv1.Series, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectMergeByLabels - HeadInMemory")
	defer sp.Finish()

	index := q.head.profiles.index

	ids, err := index.selectMatchingFPs(ctx, params)
	if err != nil {
		return nil, err
	}

	// get time nano information for profiles
	var (
		start = model.Time(params.Start)
		end   = model.Time(params.End)
	)
	seriesBuilder := seriesBuilder{}
	seriesBuilder.init(by...)

	index.mutex.RLock()
	defer index.mutex.RUnlock()

	if len(sts.GetCallSite()) == 0 {
		for _, fp := range ids {
			profileSeries, ok := index.profilesPerFP[fp]
			if !ok {
				continue
			}
			for _, p := range profileSeries.profiles {
				if p.Timestamp() < start {
					continue
				}
				if p.Timestamp() > end {
					break
				}
				seriesBuilder.add(fp, profileSeries.lbs, int64(p.Timestamp()), float64(p.Total()))
			}
		}
	} else {
		r := symdb.NewResolver(ctx, q.head.symdb,
			symdb.WithResolverStackTraceSelector(sts))
		defer r.Release()
		var v symdb.CallSiteValues
		for _, fp := range ids {
			profileSeries, ok := index.profilesPerFP[fp]
			if !ok {
				continue
			}
			for _, p := range profileSeries.profiles {
				if p.Timestamp() < start {
					continue
				}
				if p.Timestamp() > end {
					break
				}
				if err = r.CallSiteValues(&v, p.StacktracePartition, p.Samples); err != nil {
					return nil, err
				}
				seriesBuilder.add(fp, profileSeries.lbs, int64(p.Timestamp()), float64(v.Total))
			}
		}
	}
	return seriesBuilder.build(), nil
}

func (q *headInMemoryQuerier) Series(ctx context.Context, params *ingestv1.SeriesRequest) ([]*typesv1.Labels, error) {
	res, err := q.head.Series(ctx, connect.NewRequest(params))
	if err != nil {
		return nil, err
	}
	return res.Msg.LabelsSet, nil
}

func (q *headInMemoryQuerier) MergeBySpans(ctx context.Context, rows iter.Iterator[Profile], spanSelector phlaremodel.SpanSelector) (*phlaremodel.Tree, error) {
	sp, _ := opentracing.StartSpanFromContext(ctx, "MergeBySpans - HeadInMemory")
	defer sp.Finish()
	r := symdb.NewResolver(ctx, q.head.symdb)
	defer r.Release()
	for rows.Next() {
		p, ok := rows.At().(ProfileWithLabels)
		if !ok {
			return nil, errors.New("expected ProfileWithLabels")
		}
		samples := p.Samples()
		if len(samples.Spans) > 0 {
			r.AddSamplesWithSpanSelector(p.StacktracePartition(), samples, spanSelector)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return r.Tree()
}

func (q *headInMemoryQuerier) Sort(in []Profile) []Profile {
	return in
}

package phlaredb

import (
	"context"
	"sort"

	"github.com/go-kit/log/level"
	"github.com/google/pprof/profile"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/segmentio/parquet-go"

	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	"github.com/grafana/phlare/pkg/iter"
	query "github.com/grafana/phlare/pkg/phlaredb/query"
	schemav1 "github.com/grafana/phlare/pkg/phlaredb/schemas/v1"
)

type headOnDiskQuerier struct {
	head        *Head
	rowGroupIdx int
}

func (q *headOnDiskQuerier) rowGroup() *rowGroupOnDisk {
	q.head.profiles.lock.RLock()
	defer q.head.profiles.lock.RUnlock()
	return q.head.profiles.rowGroups[q.rowGroupIdx]
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
	pIt := query.NewJoinIterator(
		0,
		[]query.Iterator{
			rowIter,
			q.rowGroup().columnIter(ctx, "TimeNanos", query.NewIntBetweenPredicate(start.UnixNano(), end.UnixNano()), "TimeNanos"),
		},
		nil,
	)
	defer pIt.Close()

	var (
		profiles []Profile
		buf      = make([][]parquet.Value, 1)
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

		buf = res.Columns(buf, "TimeNanos")
		if len(buf) != 1 || len(buf[0]) != 1 {
			level.Error(q.head.logger).Log("msg", "unable to read timeNanos from profiles", "row", res.RowNumber[0], "rowGroup", q.rowGroupIdx)
			continue
		}
		profiles = append(profiles, BlockProfile{
			labels: lbls,
			fp:     v.fp,
			ts:     model.TimeFromUnixNano(buf[0][0].Int64()),
			RowNum: res.RowNumber[0],
		})
	}
	if err := pIt.Err(); err != nil {
		return nil, errors.Wrap(pIt.Err(), "iterator error")
	}

	// Sort profiles by time, the slice is already sorted by series order
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Timestamp() < profiles[j].Timestamp()
	})

	return iter.NewSliceIterator(profiles), nil
}

func (q *headOnDiskQuerier) InRange(start, end model.Time) bool {
	// TODO: Use per rowgroup information
	return q.head.InRange(start, end)
}

func (q *headOnDiskQuerier) MergeByStacktraces(ctx context.Context, rows iter.Iterator[Profile]) (*ingestv1.MergeProfilesStacktracesResult, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "MergeByStacktraces - HeadOnDisk")
	defer sp.Finish()

	stacktraceSamples := stacktraceSampleMap{}

	if err := mergeByStacktraces(ctx, q.rowGroup(), rows, stacktraceSamples); err != nil {
		return nil, err
	}

	return q.head.resolveStacktraces(ctx, stacktraceSamples), nil
}

func (q *headOnDiskQuerier) MergePprof(ctx context.Context, rows iter.Iterator[Profile]) (*profile.Profile, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "MergeByPprof - HeadOnDisk")
	defer sp.Finish()

	stacktraceSamples := profileSampleMap{}

	if err := mergeByStacktraces(ctx, q.head.profiles.rowGroups[q.rowGroupIdx], rows, stacktraceSamples); err != nil {
		return nil, err
	}

	return q.head.resolvePprof(ctx, stacktraceSamples), nil
}

func (q *headOnDiskQuerier) MergeByLabels(ctx context.Context, rows iter.Iterator[Profile], by ...string) ([]*typesv1.Series, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "MergeByLabels - HeadOnDisk")
	defer sp.Finish()

	seriesByLabels := make(seriesByLabels)

	if err := mergeByLabels(ctx, q.head.profiles.rowGroups[q.rowGroupIdx], rows, seriesByLabels, by...); err != nil {
		return nil, err
	}

	return seriesByLabels.normalize(), nil
}

func (q *headOnDiskQuerier) Sort(in []Profile) []Profile {
	var rowI, rowJ int64
	sort.Slice(in, func(i, j int) bool {
		if pI, ok := in[i].(BlockProfile); ok {
			rowI = pI.RowNum
		}
		if pJ, ok := in[j].(BlockProfile); ok {
			rowJ = pJ.RowNum
		}
		return rowI < rowJ
	})
	return in
}

type headInMemoryQuerier struct {
	head *Head
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

		var profiles = make([]*schemav1.Profile, len(profileSeries.profiles))
		copy(profiles, profileSeries.profiles)

		iters = append(iters,
			NewSeriesIterator(
				profileSeries.lbs,
				profileSeries.fp,
				iter.NewTimeRangedIterator(iter.NewSliceIterator(profiles), start, end),
			),
		)
	}

	return iter.NewSortProfileIterator(iters), nil
}

func (q *headInMemoryQuerier) InRange(start, end model.Time) bool {
	// TODO: Use per rowgroup information
	return q.head.InRange(start, end)
}

func (q *headInMemoryQuerier) MergeByStacktraces(ctx context.Context, rows iter.Iterator[Profile]) (*ingestv1.MergeProfilesStacktracesResult, error) {
	sp, _ := opentracing.StartSpanFromContext(ctx, "MergeByStacktraces - HeadInMemory")
	defer sp.Finish()

	stacktraceSamples := stacktraceSampleMap{}

	q.head.stacktraces.lock.RLock()
	for rows.Next() {
		p, ok := rows.At().(ProfileWithLabels)
		if !ok {
			return nil, errors.New("expected ProfileWithLabels")
		}

		for _, s := range p.Samples() {
			if s.Value == 0 {
				continue
			}
			if _, exists := stacktraceSamples[int64(s.StacktraceID)]; !exists {
				stacktraceSamples[int64(s.StacktraceID)] = &ingestv1.StacktraceSample{}
			}
			stacktraceSamples[int64(s.StacktraceID)].Value += s.Value
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	q.head.stacktraces.lock.RUnlock()

	return q.head.resolveStacktraces(ctx, stacktraceSamples), nil
}

func (q *headInMemoryQuerier) MergePprof(ctx context.Context, rows iter.Iterator[Profile]) (*profile.Profile, error) {
	sp, _ := opentracing.StartSpanFromContext(ctx, "MergePprof - HeadInMemory")
	defer sp.Finish()

	stacktraceSamples := profileSampleMap{}

	for rows.Next() {
		p, ok := rows.At().(ProfileWithLabels)
		if !ok {
			return nil, errors.New("expected ProfileWithLabels")
		}

		for _, s := range p.Samples() {
			if s.Value == 0 {
				continue
			}
			if _, exists := stacktraceSamples[int64(s.StacktraceID)]; !exists {
				stacktraceSamples[int64(s.StacktraceID)] = &profile.Sample{Value: []int64{0}}
			}
			stacktraceSamples[int64(s.StacktraceID)].Value[0] += s.Value
		}
	}

	return q.head.resolvePprof(ctx, stacktraceSamples), nil

}

func (q *headInMemoryQuerier) MergeByLabels(ctx context.Context, rows iter.Iterator[Profile], by ...string) ([]*typesv1.Series, error) {
	sp, _ := opentracing.StartSpanFromContext(ctx, "MergeByLabels - HeadInMemory")
	defer sp.Finish()

	labelsByFingerprint := map[model.Fingerprint]string{}
	seriesByLabels := make(seriesByLabels)
	labelBuf := make([]byte, 0, 1024)

	for rows.Next() {
		p, ok := rows.At().(ProfileWithLabels)
		if !ok {
			return nil, errors.New("expected ProfileWithLabels")
		}

		labelsByString, ok := labelsByFingerprint[p.fp]
		if !ok {
			labelBuf = p.Labels().BytesWithLabels(labelBuf, by...)
			labelsByString = string(labelBuf)
			labelsByFingerprint[p.fp] = labelsByString
			if _, ok := seriesByLabels[labelsByString]; !ok {
				seriesByLabels[labelsByString] = &typesv1.Series{
					Labels: p.Labels().WithLabels(by...),
					Points: []*typesv1.Point{
						{
							Timestamp: int64(p.Timestamp()),
							Value:     float64(p.Total()),
						},
					},
				}
				continue
			}
		}
		series := seriesByLabels[labelsByString]
		series.Points = append(series.Points, &typesv1.Point{
			Timestamp: int64(p.Timestamp()),
			Value:     float64(p.Total()),
		})

	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return seriesByLabels.normalize(), nil
}

func (q *headInMemoryQuerier) Sort(in []Profile) []Profile {
	return in
}

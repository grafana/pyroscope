package phlaredb

import (
	"context"
	"sort"

	"github.com/google/pprof/profile"
	"github.com/opentracing/opentracing-go"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/segmentio/parquet-go"

	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
	"github.com/grafana/phlare/pkg/iter"
	query "github.com/grafana/phlare/pkg/phlaredb/query"
)

type headOnDiskQuerier struct {
	head        *Head
	rowGroupIdx int
}

func (q *headOnDiskQuerier) SelectMatchingProfiles(ctx context.Context, params *ingestv1.SelectProfilesRequest) (iter.Iterator[Profile], error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "SelectMatchingProfiles - HeadOnDisk")
	defer sp.Finish()

	index := q.head.profiles.index

	ids, err := index.selectMatchingFPs(ctx, params)
	if err != nil {
		return nil, err
	}

	// gather rowRanges from matching series
	var rowRanges = make(rowRanges, len(ids))
	for _, fp := range ids {
		// skip if series no longer in index
		profileSeries, ok := index.profilesPerFP[fp]
		if !ok {
			continue
		}

		// skip if rowRange empty
		rR := profileSeries.profilesOnDisk[q.rowGroupIdx]
		if rR == nil {
			continue
		}

		rowRanges[rR] = fp
	}

	// get time nano information for profiles
	var (
		start = model.Time(params.Start)
		end   = model.Time(params.End)
		rg    = q.head.profiles.rowGroups[q.rowGroupIdx]
	)
	pIt := query.NewJoinIterator(
		0,
		[]query.Iterator{
			rowRanges.fingerprintsWithRowNum(),
			rg.columnIter(ctx, "TimeNanos", query.NewIntBetweenPredicate(start.UnixNano(), end.UnixNano()), "TimeNanos"),
		},
		nil,
	)

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

		profileSeries, ok := index.profilesPerFP[v.fp]
		if !ok {
			panic("no profile series matching fingerprint found")
		}

		buf = res.Columns(buf, "TimeNanos")
		profiles = append(profiles, BlockProfile{
			labels: profileSeries.lbs,
			fp:     profileSeries.fp,
			ts:     model.TimeFromUnixNano(buf[0][0].Int64()),
			RowNum: res.RowNumber[0],
		})
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

	if err := mergeByStacktraces(ctx, q.head.profiles.rowGroups[q.rowGroupIdx], rows, stacktraceSamples); err != nil {
		return nil, err
	}

	return q.head.resolveStacktraces(stacktraceSamples), nil
}

func (q *headOnDiskQuerier) MergePprof(ctx context.Context, rows iter.Iterator[Profile]) (*profile.Profile, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "MergeByPprof - HeadOnDisk")
	defer sp.Finish()

	stacktraceSamples := profileSampleMap{}

	if err := mergeByStacktraces(ctx, q.head.profiles.rowGroups[q.rowGroupIdx], rows, stacktraceSamples); err != nil {
		return nil, err
	}

	return q.head.resolvePprof(stacktraceSamples), nil
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
	// TODO: Use per row group order
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

		iters = append(iters,
			NewSeriesIterator(
				profileSeries.lbs,
				profileSeries.fp,
				iter.NewTimeRangedIterator(iter.NewSliceIterator(profileSeries.profiles), start, end),
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

	return q.head.resolveStacktraces(stacktraceSamples), nil
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

	return q.head.resolvePprof(stacktraceSamples), nil

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

package phlaredb

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/google/pprof/profile"
	"github.com/grafana/dskit/multierror"
	"github.com/opentracing/opentracing-go"
	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/common/model"
	"github.com/samber/lo"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb/query"
	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
)

// SplitProfiles splits profiles according to their respective row groups.
// TODO: rows iterator always wrap a slice.
// TODO: We should find a way to stream profiles of a row groups right away
// they were filtered out.
func SplitProfiles(profiles iter.Iterator[Profile], groups []parquet.RowGroup) ([]*RowGroupProfiles, error) {
	p, err := iter.Slice(profiles)
	if err != nil {
		return nil, err
	}
	rows := make([]int64, len(p))
	for i, x := range p {
		r, ok := x.(interface{ RowNumber() int64 })
		if !ok {
			panic(fmt.Sprintf("can't get profile row number: %T", x))
		}
		rows[i] = r.RowNumber()
	}
	r := query.SplitRows(rows, query.RowGroupBoundaries(groups))
	ranges := make([]*RowGroupProfiles, len(r))
	var offset int
	for i, v := range r {
		x := &RowGroupProfiles{
			Profiles: iter.NewSliceIterator(p[offset:len(v)]),
			RowGroup: groups[i],
			Rows:     v,
		}
		ranges[i] = x
		offset += len(v)
	}
	return ranges, nil
}

type ProfileWithSamples struct {
	Profile
	v1.Samples
}

type RowGroupProfiles struct {
	Samples  iter.Iterator[v1.Samples]
	Profiles iter.Iterator[Profile]
	RowGroup parquet.RowGroup
	Rows     []int64

	initOnce sync.Once
	err      error
}

func (x *RowGroupProfiles) init() {
	x.Samples = NewProfileSamplesIterator(x.RowGroup, x.Rows)
}

func (x *RowGroupProfiles) Next() bool {
	x.initOnce.Do(x.init)
	if !x.Profiles.Next() {
		return false
	}
	if !x.Samples.Next() {
		x.err = query.ErrSeekOutOfRange
		return false
	}
	return true
}

func (x *RowGroupProfiles) At() ProfileWithSamples {
	return ProfileWithSamples{
		Profile: x.Profiles.At(),
		Samples: x.Samples.At(),
	}
}

func (x *RowGroupProfiles) Err() error {
	var err multierror.MultiError
	err.Add(x.Samples.Err())
	err.Add(x.Profiles.Err())
	return err.Err()
}

func (x *RowGroupProfiles) Close() error {
	var err multierror.MultiError
	err.Add(x.Samples.Close())
	err.Add(x.Profiles.Close())
	return err.Err()
}

type ProfileSamplesIterator struct {
	StacktraceID iter.Iterator[[]uint32]
	Value        iter.Iterator[[]uint64]
	SpanID       iter.Iterator[[]uint64]
	samples      v1.Samples
}

func (s *ProfileSamplesIterator) Next() bool {
	if !s.StacktraceID.Next() {
		return false
	}
	s.samples.StacktraceIDs = s.StacktraceID.At()
	if !s.Value.Next() {
		return false
	}
	s.samples.Values = s.Value.At()
	if s.SpanID != nil {
		if !s.SpanID.Next() {
			return false
		}
		s.samples.Spans = s.SpanID.At()
	}
	return true
}

func (s *ProfileSamplesIterator) At() v1.Samples { return s.samples }

func (s *ProfileSamplesIterator) Err() error {
	var err multierror.MultiError
	if s.StacktraceID != nil {
		err.Add(s.StacktraceID.Err())
	}
	if s.Value != nil {
		err.Add(s.Value.Err())
	}
	if s.SpanID != nil {
		err.Add(s.SpanID.Err())
	}
	return err.Err()
}

func (s *ProfileSamplesIterator) Close() error {
	var err multierror.MultiError
	if s.StacktraceID != nil {
		err.Add(s.StacktraceID.Close())
	}
	if s.Value != nil {
		err.Add(s.Value.Close())
	}
	if s.SpanID != nil {
		err.Add(s.SpanID.Close())
	}
	return err.Err()
}

func NewProfileSamplesIterator(rowGroup parquet.RowGroup, rows []int64) iter.Iterator[v1.Samples] {
	var sampleColumns v1.SampleColumns
	if err := sampleColumns.Resolve(rowGroup.Schema()); err != nil {
		return iter.NewErrIterator[v1.Samples](err)
	}
	columns := rowGroup.ColumnChunks()
	batchSize := 100 // FIXME: 10 << 10
	return &ProfileSamplesIterator{
		StacktraceID: iter.NewAsyncBatchIterator[[]parquet.Value, []uint32](
			query.NewRepeatedColumnChunkIterator(iter.NewSliceIterator(rows), columns[sampleColumns.StacktraceID.ColumnIndex]),
			batchSize,
			query.CloneUint32ParquetValues,
			query.ReleaseUint32Values,
		),
		Value: iter.NewAsyncBatchIterator[[]parquet.Value, []uint64](
			query.NewRepeatedColumnChunkIterator(iter.NewSliceIterator(rows), columns[sampleColumns.Value.ColumnIndex]),
			batchSize,
			query.CloneUint64ParquetValues,
			query.ReleaseUint64Values,
		),
	}
}

func (b *singleBlockQuerier) MergeByStacktraces(ctx context.Context, rows iter.Iterator[Profile]) (*phlaremodel.Tree, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "MergeByStacktraces - Block")
	defer sp.Finish()
	r := symdb.NewResolver(ctx, b.symbols)
	defer r.Release()
	if err := mergeByStacktraces(ctx, b.profiles.file, rows, r); err != nil {
		return nil, err
	}
	return r.Tree()
}

func (b *singleBlockQuerier) MergePprof(ctx context.Context, rows iter.Iterator[Profile]) (*profile.Profile, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "MergeByStacktraces - Block")
	defer sp.Finish()
	r := symdb.NewResolver(ctx, b.symbols)
	defer r.Release()
	if err := mergeByStacktraces(ctx, b.profiles.file, rows, r); err != nil {
		return nil, err
	}
	return r.Profile()
}

func (b *singleBlockQuerier) MergeByLabels(ctx context.Context, rows iter.Iterator[Profile], by ...string) ([]*typesv1.Series, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "MergeByLabels - Block")
	defer sp.Finish()

	m := make(seriesByLabels)
	columnName := "TotalValue"
	if b.meta.Version == 1 {
		columnName = "Samples.list.element.Value"
	}
	if err := mergeByLabels(ctx, b.profiles.file, columnName, rows, m, by...); err != nil {
		return nil, err
	}
	return m.normalize(), nil
}

func (b *singleBlockQuerier) MergeBySpans(_ context.Context, _ iter.Iterator[Profile], _ phlaremodel.SpanSelector) (*phlaremodel.Tree, error) {
	//	sp, _ := opentracing.StartSpanFromContext(ctx, "MergeBySpans - Block")
	//	defer sp.Finish()
	//	r := symdb.NewResolver(ctx, b.symbols)
	//	defer r.Release()
	//	if err := mergeBySpans(ctx, b.profiles.file, rows, r, spanSelector); err != nil {
	//		return nil, err
	//	}
	//	return r.Tree()
	return new(phlaremodel.Tree), nil
}

type Source interface {
	Schema() *parquet.Schema
	RowGroups() []parquet.RowGroup
}

func mergeByStacktraces(ctx context.Context, profileSource Source, rows iter.Iterator[Profile], r *symdb.Resolver) error {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "mergeByStacktraces")
	defer sp.Finish()
	groups, err := SplitProfiles(rows, profileSource.RowGroups())
	if err != nil {
		return err
	}
	for _, group := range groups {
		for group.Next() {
			p := group.At()
			partition := r.Partition(p.StacktracePartition())
			values := p.Values
			for i, sid := range p.StacktraceIDs {
				partition[sid] += int64(values[i])
			}
		}
		if err = group.Err(); err != nil {
			_ = group.Close()
			return err
		}
		if err = group.Close(); err != nil {
			return err
		}
	}
	return nil
}

type seriesByLabels map[string]*typesv1.Series

func (m seriesByLabels) normalize() []*typesv1.Series {
	result := lo.Values(m)
	sort.Slice(result, func(i, j int) bool {
		return phlaremodel.CompareLabelPairs(result[i].Labels, result[j].Labels) < 0
	})
	// we have to sort the points in each series because labels reduction may have changed the order
	for _, s := range result {
		sort.Slice(s.Points, func(i, j int) bool {
			return s.Points[i].Timestamp < s.Points[j].Timestamp
		})
	}
	return result
}

func mergeByLabels(ctx context.Context, profileSource Source, columnName string, rows iter.Iterator[Profile], m seriesByLabels, by ...string) error {
	it := repeatedColumnIter(ctx, profileSource, columnName, rows)

	defer it.Close()

	labelsByFingerprint := map[model.Fingerprint]string{}
	labelBuf := make([]byte, 0, 1024)

	for it.Next() {
		values := it.At()
		p := values.Row
		var total int64
		for _, e := range values.Values {
			total += e.Int64()
		}
		labelsByString, ok := labelsByFingerprint[p.Fingerprint()]
		if !ok {
			labelBuf = p.Labels().BytesWithLabels(labelBuf, by...)
			labelsByString = string(labelBuf)
			labelsByFingerprint[p.Fingerprint()] = labelsByString
			if _, ok := m[labelsByString]; !ok {
				m[labelsByString] = &typesv1.Series{
					Labels: p.Labels().WithLabels(by...),
					Points: []*typesv1.Point{
						{
							Timestamp: int64(p.Timestamp()),
							Value:     float64(total),
						},
					},
				}
				continue
			}
		}
		series := m[labelsByString]
		series.Points = append(series.Points, &typesv1.Point{
			Timestamp: int64(p.Timestamp()),
			Value:     float64(total),
		})
	}
	return it.Err()
}

package phlaredb

import (
	"context"
	"sort"
	"strings"

	"github.com/grafana/dskit/runutil"
	"github.com/opentracing/opentracing-go"
	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/common/model"
	"github.com/samber/lo"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb/query"
	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
)

func (b *singleBlockQuerier) MergeByStacktraces(ctx context.Context, rows iter.Iterator[Profile]) (*phlaremodel.Tree, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "MergeByStacktraces - Block")
	defer sp.Finish()
	sp.SetTag("block ULID", b.meta.ULID.String())

	if err := b.Open(ctx); err != nil {
		return nil, err
	}
	b.queries.Add(1)
	defer b.queries.Done()

	ctx = query.AddMetricsToContext(ctx, b.metrics.query)
	r := symdb.NewResolver(ctx, b.symbols)
	defer r.Release()
	if err := mergeByStacktraces(ctx, b.profileSourceTable().file, rows, r); err != nil {
		return nil, err
	}
	return r.Tree()
}

func (b *singleBlockQuerier) MergePprof(ctx context.Context, rows iter.Iterator[Profile], maxNodes int64, sts *typesv1.StackTraceSelector) (*profilev1.Profile, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "MergePprof - Block")
	defer sp.Finish()
	sp.SetTag("block ULID", b.meta.ULID.String())

	if err := b.Open(ctx); err != nil {
		return nil, err
	}
	b.queries.Add(1)
	defer b.queries.Done()

	ctx = query.AddMetricsToContext(ctx, b.metrics.query)
	r := symdb.NewResolver(ctx, b.symbols,
		symdb.WithResolverMaxNodes(maxNodes),
		symdb.WithResolverStackTraceSelector(sts))
	defer r.Release()
	if err := mergeByStacktraces(ctx, b.profileSourceTable().file, rows, r); err != nil {
		return nil, err
	}
	return r.Pprof()
}

func (b *singleBlockQuerier) MergeByLabels(ctx context.Context, rows iter.Iterator[Profile], sts *typesv1.StackTraceSelector, by ...string) ([]*typesv1.Series, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "MergeByLabels - Block")
	defer sp.Finish()
	sp.SetTag("block ULID", b.meta.ULID.String())

	if err := b.Open(ctx); err != nil {
		return nil, err
	}
	b.queries.Add(1)
	defer b.queries.Done()

	ctx = query.AddMetricsToContext(ctx, b.metrics.query)
	if len(sts.GetCallSite()) == 0 {
		columnName := "TotalValue"
		if b.meta.Version == 1 {
			columnName = "Samples.list.element.Value"
		}
		return mergeByLabels(ctx, b.profileSourceTable().file, columnName, rows, by...)
	}
	r := symdb.NewResolver(ctx, b.symbols,
		symdb.WithResolverStackTraceSelector(sts))
	defer r.Release()
	return mergeByLabelsWithStackTraceSelector(ctx, b.profileSourceTable().file, rows, r, by...)
}

func (b *singleBlockQuerier) MergeBySpans(ctx context.Context, rows iter.Iterator[Profile], spanSelector phlaremodel.SpanSelector) (*phlaremodel.Tree, error) {
	sp, _ := opentracing.StartSpanFromContext(ctx, "MergeBySpans - Block")
	defer sp.Finish()
	sp.SetTag("block ULID", b.meta.ULID.String())

	if err := b.Open(ctx); err != nil {
		return nil, err
	}
	b.queries.Add(1)
	defer b.queries.Done()

	ctx = query.AddMetricsToContext(ctx, b.metrics.query)
	r := symdb.NewResolver(ctx, b.symbols)
	defer r.Release()
	if err := mergeBySpans(ctx, b.profileSourceTable().file, rows, r, spanSelector); err != nil {
		return nil, err
	}
	return r.Tree()
}

type Source interface {
	Schema() *parquet.Schema
	RowGroups() []parquet.RowGroup
}

func mergeByStacktraces[T interface{ StacktracePartition() uint64 }](ctx context.Context, profileSource Source, rows iter.Iterator[T], r *symdb.Resolver,
) (err error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "mergeByStacktraces")
	defer sp.Finish()
	var columns v1.SampleColumns
	if err = columns.Resolve(profileSource.Schema()); err != nil {
		return err
	}
	profiles := query.NewRepeatedRowIterator(ctx, rows, profileSource.RowGroups(),
		columns.StacktraceID.ColumnIndex,
		columns.Value.ColumnIndex,
	)
	defer runutil.CloseWithErrCapture(&err, profiles, "failed to close profile stream")
	for profiles.Next() {
		p := profiles.At()
		r.AddSamplesFromParquetRow(p.Row.StacktracePartition(), p.Values[0], p.Values[1])
	}
	return profiles.Err()
}

func mergeBySpans[T interface{ StacktracePartition() uint64 }](ctx context.Context, profileSource Source, rows iter.Iterator[T], r *symdb.Resolver, spanSelector phlaremodel.SpanSelector) (err error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "mergeBySpans")
	defer sp.Finish()
	var columns v1.SampleColumns
	if err = columns.Resolve(profileSource.Schema()); err != nil {
		return err
	}
	if !columns.HasSpanID() {
		return nil
	}
	profiles := query.NewRepeatedRowIterator(ctx, rows, profileSource.RowGroups(),
		columns.StacktraceID.ColumnIndex,
		columns.Value.ColumnIndex,
		columns.SpanID.ColumnIndex,
	)
	defer runutil.CloseWithErrCapture(&err, profiles, "failed to close profile stream")
	for profiles.Next() {
		p := profiles.At()
		partition := p.Row.StacktracePartition()
		stacktraces := p.Values[0]
		values := p.Values[1]
		spans := p.Values[2]
		r.WithPartitionSamples(partition, func(samples map[uint32]int64) {
			for i, sid := range stacktraces {
				spanID := spans[i].Uint64()
				if spanID == 0 {
					continue
				}
				if _, ok := spanSelector[spanID]; ok {
					samples[sid.Uint32()] += values[i].Int64()
				}
			}
		})
	}
	return profiles.Err()
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

type seriesBuilder struct {
	labelsByFingerprint map[model.Fingerprint]string
	labelBuf            []byte
	by                  []string

	series seriesByLabels
}

func (s *seriesBuilder) init(by ...string) {
	s.labelsByFingerprint = map[model.Fingerprint]string{}
	s.series = make(seriesByLabels)
	s.labelBuf = make([]byte, 0, 1024)
	s.by = by
}

func (s *seriesBuilder) add(fp model.Fingerprint, lbs phlaremodel.Labels, ts int64, value float64) {
	labelsByString, ok := s.labelsByFingerprint[fp]
	if !ok {
		s.labelBuf = lbs.BytesWithLabels(s.labelBuf, s.by...)
		labelsByString = string(s.labelBuf)
		s.labelsByFingerprint[fp] = labelsByString
		if _, ok := s.series[labelsByString]; !ok {
			s.series[labelsByString] = &typesv1.Series{
				Labels: lbs.WithLabels(s.by...),
				Points: []*typesv1.Point{
					{
						Timestamp: ts,
						Value:     value,
					},
				},
			}
			return
		}
	}
	series := s.series[labelsByString]
	series.Points = append(series.Points, &typesv1.Point{
		Timestamp: ts,
		Value:     value,
	})
}

func (s *seriesBuilder) build() []*typesv1.Series {
	return s.series.normalize()
}

func mergeByLabels[T Profile](
	ctx context.Context,
	profileSource Source,
	columnName string,
	rows iter.Iterator[T],
	by ...string,
) ([]*typesv1.Series, error) {
	column, err := v1.ResolveColumnByPath(profileSource.Schema(), strings.Split(columnName, "."))
	if err != nil {
		return nil, err
	}
	profiles := query.NewRepeatedRowIterator(ctx, rows, profileSource.RowGroups(), column.ColumnIndex)
	defer runutil.CloseWithErrCapture(&err, profiles, "failed to close profile stream")

	seriesBuilder := seriesBuilder{}
	seriesBuilder.init(by...)

	for profiles.Next() {
		values := profiles.At()
		p := values.Row
		var total int64
		for _, e := range values.Values {
			total += e[0].Int64()
		}
		seriesBuilder.add(p.Fingerprint(), p.Labels(), int64(p.Timestamp()), float64(total))

	}
	return seriesBuilder.build(), profiles.Err()
}

func mergeByLabelsWithStackTraceSelector[T Profile](
	ctx context.Context,
	profileSource Source,
	rows iter.Iterator[T],
	r *symdb.Resolver,
	by ...string,
) (s []*typesv1.Series, err error) {
	var columns v1.SampleColumns
	if err = columns.Resolve(profileSource.Schema()); err != nil {
		return nil, err
	}
	profiles := query.NewRepeatedRowIterator(ctx, rows, profileSource.RowGroups(),
		columns.StacktraceID.ColumnIndex,
		columns.Value.ColumnIndex,
	)

	seriesBuilder := seriesBuilder{}
	seriesBuilder.init(by...)

	defer runutil.CloseWithErrCapture(&err, profiles, "failed to close profile stream")
	var v symdb.CallSiteValues
	for profiles.Next() {
		row := profiles.At()
		h := row.Row
		if err = r.CallSiteValuesParquet(&v, h.StacktracePartition(), row.Values[0], row.Values[1]); err != nil {
			return nil, err
		}
		seriesBuilder.add(h.Fingerprint(), h.Labels(), int64(h.Timestamp()), float64(v.Total))
	}

	return seriesBuilder.build(), profiles.Err()
}

package phlaredb

import (
	"context"
	"sort"
	"strings"

	"github.com/google/pprof/profile"
	"github.com/grafana/dskit/runutil"
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

func (b *singleBlockQuerier) MergeBySpans(ctx context.Context, rows iter.Iterator[Profile], spanSelector phlaremodel.SpanSelector) (*phlaremodel.Tree, error) {
	sp, _ := opentracing.StartSpanFromContext(ctx, "MergeBySpans - Block")
	defer sp.Finish()
	r := symdb.NewResolver(ctx, b.symbols)
	defer r.Release()
	if err := mergeBySpans(ctx, b.profiles.file, rows, r, spanSelector); err != nil {
		return nil, err
	}
	return r.Tree()
}

type Source interface {
	Schema() *parquet.Schema
	RowGroups() []parquet.RowGroup
}

func mergeByStacktraces(ctx context.Context, profileSource Source, rows iter.Iterator[Profile], r *symdb.Resolver) (err error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "mergeByStacktraces")
	defer sp.Finish()
	var columns v1.SampleColumns
	if err = columns.Resolve(profileSource.Schema()); err != nil {
		return err
	}
	profiles := query.NewRepeatedRowIterator(rows, profileSource.RowGroups(),
		columns.StacktraceID.ColumnIndex,
		columns.Value.ColumnIndex,
	)
	defer runutil.CloseWithErrCapture(&err, profiles, "failed to close profile stream")
	for profiles.Next() {
		p := profiles.At()
		partition := r.Partition(p.Row.StacktracePartition())
		stacktraces := p.Values[0]
		values := p.Values[1]
		for i, sid := range stacktraces {
			partition[sid.Uint32()] += values[i].Int64()
		}
	}
	return profiles.Err()
}

func mergeBySpans(ctx context.Context, profileSource Source, rows iter.Iterator[Profile], r *symdb.Resolver, spanSelector phlaremodel.SpanSelector) (err error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "mergeBySpans")
	defer sp.Finish()
	var columns v1.SampleColumns
	if err = columns.Resolve(profileSource.Schema()); err != nil {
		return err
	}
	if !columns.HasSpanID() {
		return nil
	}
	profiles := query.NewRepeatedRowIterator(rows, profileSource.RowGroups(),
		columns.StacktraceID.ColumnIndex,
		columns.Value.ColumnIndex,
		columns.SpanID.ColumnIndex,
	)
	defer runutil.CloseWithErrCapture(&err, profiles, "failed to close profile stream")
	for profiles.Next() {
		p := profiles.At()
		partition := r.Partition(p.Row.StacktracePartition())
		stacktraces := p.Values[0]
		values := p.Values[1]
		spans := p.Values[2]
		for i, sid := range stacktraces {
			spanID := spans[i].Uint64()
			if spanID == 0 {
				continue
			}
			if _, ok := spanSelector[spanID]; ok {
				partition[sid.Uint32()] += values[i].Int64()
			}
		}
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

func mergeByLabels(ctx context.Context, profileSource Source, columnName string, rows iter.Iterator[Profile], m seriesByLabels, by ...string) (err error) {
	column, err := v1.ResolveColumnByPath(profileSource.Schema(), strings.Split(columnName, "."))
	if err != nil {
		return err
	}
	profiles := query.NewRepeatedRowIterator(rows, profileSource.RowGroups(), column.ColumnIndex)
	defer runutil.CloseWithErrCapture(&err, profiles, "failed to close profile stream")

	labelsByFingerprint := map[model.Fingerprint]string{}
	labelBuf := make([]byte, 0, 1024)

	for profiles.Next() {
		values := profiles.At()
		p := values.Row
		var total int64
		for _, e := range values.Values {
			total += e[0].Int64()
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
	return profiles.Err()
}

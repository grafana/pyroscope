package phlaredb

import (
	"context"
	"strings"

	"github.com/grafana/dskit/runutil"
	"github.com/opentracing/opentracing-go"
	"github.com/parquet-go/parquet-go"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb/query"
	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
)

func (b *singleBlockQuerier) MergeByStacktraces(ctx context.Context, rows iter.Iterator[Profile], maxNodes int64) (*phlaremodel.Tree, error) {
	sp, ctx := opentracing.StartSpanFromContext(ctx, "MergeByStacktraces - Block")
	defer sp.Finish()
	sp.SetTag("block ULID", b.meta.ULID.String())

	if err := b.Open(ctx); err != nil {
		return nil, err
	}
	b.queries.Add(1)
	defer b.queries.Done()

	ctx = query.AddMetricsToContext(ctx, b.metrics.query)
	r := symdb.NewResolver(ctx, b.symbols, symdb.WithResolverMaxNodes(maxNodes))
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
		r.AddSamplesWithSpanSelectorFromParquetRow(
			p.Row.StacktracePartition(),
			p.Values[0],
			p.Values[1],
			p.Values[2],
			spanSelector,
		)
	}
	return profiles.Err()
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

	// these columns might not be present
	annotationKeysColumn, _ := v1.ResolveColumnByPath(profileSource.Schema(), v1.AnnotationKeyColumnPath)
	annotationValuesColumn, _ := v1.ResolveColumnByPath(profileSource.Schema(), v1.AnnotationValueColumnPath)

	profiles := query.NewRepeatedRowIterator(
		ctx,
		rows,
		profileSource.RowGroups(),
		column.ColumnIndex,
		annotationKeysColumn.ColumnIndex,
		annotationValuesColumn.ColumnIndex,
	)
	defer runutil.CloseWithErrCapture(&err, profiles, "failed to close profile stream")

	seriesBuilder := phlaremodel.NewTimeSeriesBuilder(by...)

	for profiles.Next() {
		values := profiles.At()
		p := values.Row
		var total int64
		annotations := v1.Annotations{
			Keys:   make([]string, 0),
			Values: make([]string, 0),
		}
		for _, e := range values.Values {
			if e[0].Column() == column.ColumnIndex && e[0].Kind() == parquet.Int64 {
				total += e[0].Int64()
			} else if e[0].Column() == annotationKeysColumn.ColumnIndex && e[0].Kind() == parquet.ByteArray {
				annotations.Keys = append(annotations.Keys, e[0].String())
			} else if e[0].Column() == annotationValuesColumn.ColumnIndex && e[0].Kind() == parquet.ByteArray {
				annotations.Values = append(annotations.Values, e[0].String())
			}
		}
		seriesBuilder.Add(
			p.Fingerprint(),
			p.Labels(),
			int64(p.Timestamp()),
			float64(total),
			annotations,
			"",
		)
	}
	return seriesBuilder.Build(), profiles.Err()
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

	seriesBuilder := phlaremodel.TimeSeriesBuilder{}
	seriesBuilder.Init(by...)

	defer runutil.CloseWithErrCapture(&err, profiles, "failed to close profile stream")
	var v symdb.CallSiteValues
	for profiles.Next() {
		row := profiles.At()
		h := row.Row
		if err = r.CallSiteValuesParquet(&v, h.StacktracePartition(), row.Values[0], row.Values[1]); err != nil {
			return nil, err
		}
		// TODO aleks-p: add annotation support?
		seriesBuilder.Add(h.Fingerprint(), h.Labels(), int64(h.Timestamp()), float64(v.Total), v1.Annotations{}, "")
	}

	return seriesBuilder.Build(), profiles.Err()
}

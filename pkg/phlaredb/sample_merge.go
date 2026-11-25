package phlaredb

import (
	"context"
	"strings"

	"github.com/grafana/dskit/runutil"
	"github.com/grafana/dskit/tracing"
	"github.com/parquet-go/parquet-go"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/v2/pkg/iter"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/model/timeseries"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/query"
	v1 "github.com/grafana/pyroscope/v2/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/symdb"
)

func (b *singleBlockQuerier) MergeByStacktraces(ctx context.Context, rows iter.Iterator[Profile], maxNodes int64) (*phlaremodel.FunctionNameTree, error) {
	sp, ctx := tracing.StartSpanFromContext(ctx, "MergeByStacktraces - Block")
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
	sp, ctx := tracing.StartSpanFromContext(ctx, "MergePprof - Block")
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
	sp, ctx := tracing.StartSpanFromContext(ctx, "MergeByLabels - Block")
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

func (b *singleBlockQuerier) MergeBySpans(ctx context.Context, rows iter.Iterator[Profile], spanSelector phlaremodel.SpanSelector) (*phlaremodel.FunctionNameTree, error) {
	sp, _ := tracing.StartSpanFromContext(ctx, "MergeBySpans - Block")
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
	sp, ctx := tracing.StartSpanFromContext(ctx, "mergeByStacktraces")
	defer sp.Finish()
	var columns v1.SampleColumns
	if err = columns.Resolve(profileSource.Schema()); err != nil {
		return err
	}
	profiles := query.NewTypedRepeatedRowIterator(ctx, rows, profileSource.RowGroups())
	defer runutil.CloseWithErrCapture(&err, profiles, "failed to close profile stream")
	stacktraces := profiles.Uint32Column(columns.StacktraceID.ColumnIndex)
	values := profiles.Uint64Column(columns.Value.ColumnIndex)
	for profiles.Next() {
		p := profiles.At()
		r.AddSamples(p.StacktracePartition(), v1.Samples{
			StacktraceIDs: stacktraces.At(),
			Values:        values.At(),
		})
	}
	return profiles.Err()
}

func mergeBySpans[T interface{ StacktracePartition() uint64 }](ctx context.Context, profileSource Source, rows iter.Iterator[T], r *symdb.Resolver, spanSelector phlaremodel.SpanSelector) (err error) {
	sp, ctx := tracing.StartSpanFromContext(ctx, "mergeBySpans")
	defer sp.Finish()
	var columns v1.SampleColumns
	if err = columns.Resolve(profileSource.Schema()); err != nil {
		return err
	}
	if !columns.HasSpanID() {
		return nil
	}
	profiles := query.NewTypedRepeatedRowIterator(ctx, rows, profileSource.RowGroups())
	defer runutil.CloseWithErrCapture(&err, profiles, "failed to close profile stream")
	stacktraces := profiles.Uint32Column(columns.StacktraceID.ColumnIndex)
	values := profiles.Uint64Column(columns.Value.ColumnIndex)
	spans := profiles.Uint64Column(columns.SpanID.ColumnIndex)
	for profiles.Next() {
		p := profiles.At()
		r.AddSamplesWithSpanSelector(
			p.StacktracePartition(),
			v1.Samples{
				StacktraceIDs: stacktraces.At(),
				Values:        values.At(),
				Spans:         spans.At(),
			},
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
) (_ []*typesv1.Series, err error) {
	column, err := v1.ResolveColumnByPath(profileSource.Schema(), strings.Split(columnName, "."))
	if err != nil {
		return nil, err
	}

	// these columns might not be present
	annotationKeyColIndex := -1
	if annotationKeysColumn, err := v1.ResolveColumnByPath(profileSource.Schema(), v1.AnnotationKeyColumnPath); err == nil {
		annotationKeyColIndex = annotationKeysColumn.ColumnIndex
	}
	annotationValueColIndex := -1
	if annotationValuesColumn, err := v1.ResolveColumnByPath(profileSource.Schema(), v1.AnnotationValueColumnPath); err == nil {
		annotationValueColIndex = annotationValuesColumn.ColumnIndex
	}

	profiles := query.NewTypedRepeatedRowIterator(
		ctx,
		rows,
		profileSource.RowGroups(),
	)
	defer runutil.CloseWithErrCapture(&err, profiles, "failed to close profile stream")

	values := profiles.Int64Column(column.ColumnIndex)
	annotationKeys := profiles.StringColumn(annotationKeyColIndex)
	annotationValues := profiles.StringColumn(annotationValueColIndex)

	seriesBuilder := timeseries.NewBuilder(by...)

	for profiles.Next() {
		p := profiles.At()
		var total int64
		for _, v := range values.At() {
			total += v
		}
		annotations := v1.Annotations{
			Keys:   annotationKeys.At(),
			Values: annotationValues.At(),
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
	profiles := query.NewTypedRepeatedRowIterator(ctx, rows, profileSource.RowGroups())
	stacktraces := profiles.Uint32Column(columns.StacktraceID.ColumnIndex)
	values := profiles.Uint64Column(columns.Value.ColumnIndex)

	seriesBuilder := timeseries.Builder{}
	seriesBuilder.Init(by...)

	defer runutil.CloseWithErrCapture(&err, profiles, "failed to close profile stream")
	var v symdb.CallSiteValues
	for profiles.Next() {
		h := profiles.At()
		samples := v1.Samples{
			StacktraceIDs: stacktraces.At(),
			Values:        values.At(),
		}
		if err = r.CallSiteValues(&v, h.StacktracePartition(), samples); err != nil {
			return nil, err
		}
		// TODO aleks-p: add annotation support?
		seriesBuilder.Add(h.Fingerprint(), h.Labels(), int64(h.Timestamp()), float64(v.Total), v1.Annotations{}, "")
	}

	return seriesBuilder.Build(), profiles.Err()
}

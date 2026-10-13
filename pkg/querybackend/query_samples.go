package querybackend

import (
	"fmt"
	"strings"

	"github.com/grafana/dskit/runutil"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/grafana/pyroscope/v2/pkg/model"
	parquetquery "github.com/grafana/pyroscope/v2/pkg/phlaredb/query"
	v1 "github.com/grafana/pyroscope/v2/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/symdb"
)

func collectStacktraceSamples(q *queryContext, resolver *symdb.Resolver, profileIDs, spans, traces []string) (hasColumns bool, err error) {
	otelSpan := trace.SpanFromContext(q.ctx)

	profileOpts := []profileIteratorOption{withExcludeSampled()}
	if len(profileIDs) > 0 {
		opt, err := withProfileIDSelector(profileIDs...)
		if err != nil {
			return false, err
		}
		profileOpts = append(profileOpts, opt)
		otelSpan.SetAttributes(attribute.Int("profile_id_selector.count", len(profileIDs)))
		if len(profileIDs) <= maxProfileIDsToLog {
			otelSpan.SetAttributes(attribute.String("profile_ids", strings.Join(profileIDs, ",")))
		}
	}

	entries, err := profileEntryIterator(q, profileOpts...)
	if err != nil {
		return false, err
	}
	defer runutil.CloseWithErrCapture(&err, entries, "failed to close profile entry iterator")

	spanSelector, err := model.NewSpanSelector(spans)
	if err != nil {
		return false, err
	}

	traceSelector, err := model.NewTraceSelector(traces)
	if err != nil {
		return false, err
	}

	// Mutually exclusive: no public RPC sets both, so reject an internal query
	// plan that does rather than silently apply one and drop the other.
	if len(spanSelector) > 0 && len(traceSelector) > 0 {
		return false, fmt.Errorf("span_selector and trace_id_selector cannot be combined")
	}

	var columns v1.SampleColumns
	if err = columns.Resolve(q.ds.Profiles().Schema()); err != nil {
		return false, err
	}

	indices := []int{
		columns.StacktraceID.ColumnIndex,
		columns.Value.ColumnIndex,
	}
	switch {
	case len(spanSelector) > 0:
		if !columns.HasSpanID() {
			// Block has no SpanID column: no samples can match the span selector.
			return false, nil
		}
		indices = append(indices, columns.SpanID.ColumnIndex)
	case len(traceSelector) > 0:
		if !columns.HasTraceID() {
			// Block has no TraceID column: no samples can match the trace selector.
			return false, nil
		}
		indices = append(indices, columns.TraceID.ColumnIndex)
	}

	profiles := parquetquery.NewRepeatedRowIterator(q.ctx, entries, q.ds.Profiles().RowGroups(), indices...)
	defer runutil.CloseWithErrCapture(&err, profiles, "failed to close profile stream")

	switch {
	case len(spanSelector) > 0:
		for profiles.Next() {
			p := profiles.At()
			resolver.AddSamplesWithSpanSelectorFromParquetRow(
				p.Row.Partition,
				p.Values[0],
				p.Values[1],
				p.Values[2],
				spanSelector,
			)
		}
	case len(traceSelector) > 0:
		for profiles.Next() {
			p := profiles.At()
			resolver.AddSamplesWithTraceSelectorFromParquetRow(
				p.Row.Partition,
				p.Values[0],
				p.Values[1],
				p.Values[2],
				traceSelector,
			)
		}
	default:
		for profiles.Next() {
			p := profiles.At()
			resolver.AddSamplesFromParquetRow(p.Row.Partition, p.Values[0], p.Values[1])
		}
	}

	if err = profiles.Err(); err != nil {
		return false, err
	}

	return true, nil
}

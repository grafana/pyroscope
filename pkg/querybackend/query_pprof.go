package querybackend

import (
	"fmt"
	"strings"
	"sync"

	"github.com/grafana/dskit/runutil"
	"github.com/prometheus/prometheus/model/labels"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
	parquetquery "github.com/grafana/pyroscope/v2/pkg/phlaredb/query"
	v1 "github.com/grafana/pyroscope/v2/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/v2/pkg/pprof"
)

func init() {
	registerQueryType(
		queryv1.QueryType_QUERY_PPROF,
		queryv1.ReportType_REPORT_PPROF,
		queryPprof,
		newPprofAggregator,
		false,
		[]block.Section{
			block.SectionTSDB,
			block.SectionProfiles,
			block.SectionSymbols,
		}...,
	)
}

func queryPprof(q *queryContext, query *queryv1.Query) (*queryv1.Report, error) {
	otelSpan := trace.SpanFromContext(q.ctx)

	profileOpts := []profileIteratorOption{withExcludeSampled()}
	if len(query.Pprof.ProfileIdSelector) > 0 {
		opt, err := withProfileIDSelector(query.Pprof.ProfileIdSelector...)
		if err != nil {
			return nil, err
		}
		profileOpts = append(profileOpts, opt)
		otelSpan.SetAttributes(attribute.Int("profile_id_selector.count", len(query.Pprof.ProfileIdSelector)))
		if len(query.Pprof.ProfileIdSelector) <= maxProfileIDsToLog {
			otelSpan.SetAttributes(attribute.String("profile_ids", strings.Join(query.Pprof.ProfileIdSelector, ",")))
		}
	}

	entries, err := profileEntryIterator(q, profileOpts...)
	if err != nil {
		return nil, err
	}
	defer runutil.CloseWithErrCapture(&err, entries, "failed to close profile entry iterator")

	spanSelector, err := phlaremodel.NewSpanSelector(query.Pprof.SpanSelector)
	if err != nil {
		return nil, err
	}

	traceSelector, err := phlaremodel.NewTraceSelector(query.Pprof.TraceIdSelector)
	if err != nil {
		return nil, err
	}

	// Mutually exclusive: no public RPC sets both, so reject an internal query
	// plan that does rather than silently apply one and drop the other.
	if len(spanSelector) > 0 && len(traceSelector) > 0 {
		return nil, fmt.Errorf("span_selector and trace_id_selector cannot be combined")
	}

	var columns v1.SampleColumns
	if err = columns.Resolve(q.ds.Profiles().Schema()); err != nil {
		return nil, err
	}

	indices := []int{
		columns.StacktraceID.ColumnIndex,
		columns.Value.ColumnIndex,
	}
	switch {
	case len(spanSelector) > 0:
		if !columns.HasSpanID() {
			// Block has no SpanID column: no samples can match the span selector.
			return &queryv1.Report{Pprof: &queryv1.PprofReport{Query: query.Pprof.CloneVT()}}, nil
		}
		indices = append(indices, columns.SpanID.ColumnIndex)
	case len(traceSelector) > 0:
		if !columns.HasTraceID() {
			// Block has no TraceID column: no samples can match the trace selector.
			return &queryv1.Report{Pprof: &queryv1.PprofReport{Query: query.Pprof.CloneVT()}}, nil
		}
		indices = append(indices, columns.TraceID.ColumnIndex)
	}

	profiles := parquetquery.NewRepeatedRowIterator(q.ctx, entries, q.ds.Profiles().RowGroups(), indices...)
	defer runutil.CloseWithErrCapture(&err, profiles, "failed to close profile stream")

	resolverOptions := make([]symdb.ResolverOption, 0)
	resolverOptions = append(resolverOptions, symdb.WithResolverMaxNodes(query.Pprof.MaxNodes))
	if query.Pprof.StackTraceSelector != nil {
		resolverOptions = append(
			resolverOptions,
			symdb.WithResolverStackTraceSelector(query.Pprof.StackTraceSelector),
			symdb.WithResolverSanitizeOnMerge(q.req.src.Options.SanitizeOnMerge))
	}

	resolver := symdb.NewResolver(q.ctx, q.ds.Symbols(), resolverOptions...)
	defer resolver.Release()

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
		return nil, err
	}

	profile, err := resolver.Pprof()
	if err != nil {
		return nil, err
	}

	for _, m := range q.req.matchers {
		if m.Name == phlaremodel.LabelNameProfileType && m.Type == labels.MatchEqual {
			if t, err := phlaremodel.ParseProfileTypeSelector(m.Value); err == nil {
				pprof.SetProfileMetadata(profile, t, q.req.endTime, 0)
				break
			}
		}
	}

	resp := &queryv1.Report{
		Pprof: &queryv1.PprofReport{
			Query: query.Pprof.CloneVT(),
			Pprof: pprof.MustMarshal(profile, true),
		},
	}

	return resp, nil
}

type pprofAggregator struct {
	init     sync.Once
	query    *queryv1.PprofQuery
	profile  pprof.ProfileMerge
	sanitize bool
}

func newPprofAggregator(req *queryv1.InvokeRequest) aggregator {
	if req.Options != nil {
		return &pprofAggregator{
			sanitize: req.Options.SanitizeOnMerge,
		}
	}
	return &pprofAggregator{}
}

func (a *pprofAggregator) aggregate(report *queryv1.Report) error {
	r := report.Pprof
	a.init.Do(func() {
		a.query = r.Query.CloneVT()
	})
	return a.profile.MergeBytes(r.Pprof, a.sanitize)
}

func (a *pprofAggregator) build() *queryv1.Report {
	return &queryv1.Report{
		Pprof: &queryv1.PprofReport{
			Query: a.query,
			Pprof: pprof.MustMarshal(a.profile.Profile(), true),
		},
	}
}

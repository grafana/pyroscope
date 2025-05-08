package query_backend

import (
	"sync"

	"github.com/grafana/dskit/runutil"
	"github.com/prometheus/prometheus/model/labels"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/experiment/block"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	parquetquery "github.com/grafana/pyroscope/pkg/phlaredb/query"
	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/pkg/pprof"
)

func init() {
	registerQueryType(
		queryv1.QueryType_QUERY_PPROF,
		queryv1.ReportType_REPORT_PPROF,
		queryPprof,
		newPprofAggregator,
		[]block.Section{
			block.SectionTSDB,
			block.SectionProfiles,
			block.SectionSymbols,
		}...,
	)
}

func queryPprof(q *queryContext, query *queryv1.Query) (*queryv1.Report, error) {

	group := true
	if query.Pprof.DoOtelCheck {
		group = false
	}
	entries, err := profileEntryIterator(q, group)
	if err != nil {
		return nil, err
	}
	defer runutil.CloseWithErrCapture(&err, entries, "failed to close profile entry iterator")

	var columns v1.SampleColumns
	if err = columns.Resolve(q.ds.Profiles().Schema()); err != nil {
		return nil, err
	}

	profiles := parquetquery.NewRepeatedRowIterator(q.ctx, entries, q.ds.Profiles().RowGroups(),
		columns.StacktraceID.ColumnIndex,
		columns.Value.ColumnIndex)
	defer runutil.CloseWithErrCapture(&err, profiles, "failed to close profile stream")

	resolverOptions := make([]symdb.ResolverOption, 0)
	resolverOptions = append(resolverOptions, symdb.WithResolverMaxNodes(query.Pprof.MaxNodes))
	if query.Pprof.StackTraceSelector != nil {
		resolverOptions = append(resolverOptions, symdb.WithResolverStackTraceSelector(query.Pprof.StackTraceSelector))
	}

	resolver := symdb.NewResolver(q.ctx, q.ds.Symbols(), resolverOptions...)
	defer resolver.Release()

	otel := false

	for profiles.Next() {
		p := profiles.At()
		if query.Pprof.DoOtelCheck && p.Row.Labels.Get("__otel__") == "true" {
			otel = true
		}
		resolver.AddSamplesFromParquetRow(p.Row.Partition, p.Values[0], p.Values[1])
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
			Otel:  otel,
		},
	}

	return resp, nil
}

type pprofAggregator struct {
	init    sync.Once
	query   *queryv1.PprofQuery
	profile pprof.ProfileMerge
	otel    bool
}

func newPprofAggregator(*queryv1.InvokeRequest) aggregator { return new(pprofAggregator) }

func (a *pprofAggregator) aggregate(report *queryv1.Report) error {
	r := report.Pprof
	a.init.Do(func() {
		a.query = r.Query.CloneVT()
	})
	err := a.profile.MergeBytes(r.Pprof)
	if err != nil {
		return err
	}
	if r.Otel {
		a.otel = true
	}
	return err
}

func (a *pprofAggregator) build() *queryv1.Report {
	return &queryv1.Report{
		Pprof: &queryv1.PprofReport{
			Query: a.query,
			Pprof: pprof.MustMarshal(a.profile.Profile(), true),
			Otel:  a.otel,
		},
	}
}

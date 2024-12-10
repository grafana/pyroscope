package query_backend

import (
	"sync"

	"github.com/grafana/dskit/runutil"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/experiment/block"
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
	entries, err := profileEntryIterator(q)
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

	for profiles.Next() {
		p := profiles.At()
		resolver.AddSamplesFromParquetRow(p.Row.Partition, p.Values[0], p.Values[1])
	}
	if err = profiles.Err(); err != nil {
		return nil, err
	}

	profile, err := resolver.Pprof()
	if err != nil {
		return nil, err
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
	init    sync.Once
	query   *queryv1.PprofQuery
	profile pprof.ProfileMerge
}

func newPprofAggregator(*queryv1.InvokeRequest) aggregator { return new(pprofAggregator) }

func (a *pprofAggregator) aggregate(report *queryv1.Report) error {
	r := report.Pprof
	a.init.Do(func() {
		a.query = r.Query.CloneVT()
	})
	return a.profile.MergeBytes(r.Pprof)
}

func (a *pprofAggregator) build() *queryv1.Report {
	return &queryv1.Report{
		Pprof: &queryv1.PprofReport{
			Query: a.query,
			Pprof: pprof.MustMarshal(a.profile.Profile(), true),
		},
	}
}

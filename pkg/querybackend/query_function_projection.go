package querybackend

import (
	"errors"
	"sync"

	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/symdb"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

func init() {
	registerQueryType(
		queryv1.QueryType_QUERY_FUNCTIONS,
		queryv1.ReportType_REPORT_FUNCTIONS,
		queryFunctions,
		newFunctionsAggregator,
		false,
		[]block.Section{
			block.SectionTSDB,
			block.SectionProfiles,
			block.SectionSymbols,
		}...,
	)

}

func queryFunctions(q *queryContext, query *queryv1.Query) (*queryv1.Report, error) {
	req := query.Functions
	if req.StackTraceSelector.GetGoPgo() != nil {
		return nil, errors.New("function projections do not support go_pgo")
	}
	resolver := symdb.NewResolver(q.ctx, q.ds.Symbols(), symdb.WithResolverStackTraceSelector(req.StackTraceSelector))
	defer resolver.Release()
	if _, err := collectStacktraceSamples(q, resolver, req.ProfileIdSelector, req.SpanSelector, req.TraceIdSelector); err != nil {
		return nil, err
	}
	table, err := resolver.Functions()
	if err != nil {
		return nil, err
	}
	return &queryv1.Report{
		Functions: &queryv1.FunctionsReport{
			Query: req.CloneVT(),
			Table: table,
		},
	}, nil
}

type functionsAggregator struct {
	init      sync.Once
	query     *queryv1.FunctionsQuery
	functions *model.FunctionTableMerger
}

func newFunctionsAggregator(*queryv1.InvokeRequest) aggregator {
	return &functionsAggregator{
		functions: model.NewFunctionTableMerger(),
	}
}

func (a *functionsAggregator) aggregate(report *queryv1.Report) error {
	r := report.Functions
	a.init.Do(func() {
		a.query = r.Query.CloneVT()
	})
	a.functions.Merge(r.Table)
	return nil
}

func (a *functionsAggregator) build() *queryv1.Report {
	return &queryv1.Report{
		Functions: &queryv1.FunctionsReport{
			Query: a.query,
			Table: a.functions.Table(),
		},
	}
}

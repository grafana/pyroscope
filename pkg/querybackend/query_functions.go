package querybackend

import (
	"fmt"
	"sync"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/model"
)

func init() {
	registerQueryType(queryv1.QueryType_QUERY_FUNCTIONS, queryv1.ReportType_REPORT_FUNCTIONS,
		queryFunctions, newFunctionsAggregator, false,
		block.SectionTSDB, block.SectionProfiles, block.SectionSymbols)
}

func queryFunctions(q *queryContext, query *queryv1.Query) (*queryv1.Report, error) {
	f := query.Functions
	if f == nil {
		return nil, fmt.Errorf("functions query is required")
	}
	// Zero max_nodes disables truncation in both the symbol resolver and tree
	// builder. Aggregate before serialization, keeping all functions for merges.
	return queryTreeOrFunctions(q, &queryv1.Query{Tree: &queryv1.TreeQuery{
		SpanSelector:       f.SpanSelector,
		StackTraceSelector: f.StackTraceSelector,
		ProfileIdSelector:  f.ProfileIdSelector,
		TraceIdSelector:    f.TraceIdSelector,
	}}, f)
}

func emptyFunctionsReport(query *queryv1.FunctionsQuery) *queryv1.Report {
	return &queryv1.Report{Functions: &queryv1.FunctionsReport{
		Query: query.CloneVT(), Functions: new(querierv1.FunctionTable),
	}}
}

type functionsAggregator struct {
	init   sync.Once
	query  *queryv1.FunctionsQuery
	merger model.FunctionTableMerger
}

func newFunctionsAggregator(*queryv1.InvokeRequest) aggregator { return new(functionsAggregator) }

func (a *functionsAggregator) aggregate(report *queryv1.Report) error {
	r := report.Functions
	if r == nil || r.Functions == nil {
		return fmt.Errorf("functions report is required")
	}
	a.init.Do(func() { a.query = r.Query.CloneVT() })
	a.merger.Merge(r.Functions)
	return nil
}

func (a *functionsAggregator) build() *queryv1.Report {
	return &queryv1.Report{Functions: &queryv1.FunctionsReport{
		Query: a.query, Functions: a.merger.Table(),
	}}
}

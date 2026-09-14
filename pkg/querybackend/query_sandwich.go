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
	registerQueryType(queryv1.QueryType_QUERY_SANDWICH, queryv1.ReportType_REPORT_SANDWICH,
		querySandwich, newSandwichAggregator, false,
		block.SectionTSDB, block.SectionProfiles, block.SectionSymbols)
}

func querySandwich(q *queryContext, query *queryv1.Query) (*queryv1.Report, error) {
	s := query.Sandwich
	if s == nil {
		return nil, fmt.Errorf("sandwich query is required")
	}
	if s.Function == "" {
		return nil, fmt.Errorf("sandwich query requires a function")
	}
	// Zero max_nodes disables truncation in both the symbol resolver and the
	// tree builder. A half truncated here could not be corrected by a later
	// merge, so both halves stay whole until the frontend truncates.
	return queryTreeOrAggregate(q, &queryv1.Query{Tree: &queryv1.TreeQuery{
		SpanSelector:       s.SpanSelector,
		StackTraceSelector: s.StackTraceSelector,
		ProfileIdSelector:  s.ProfileIdSelector,
		TraceIdSelector:    s.TraceIdSelector,
	}}, nil, s)
}

func emptySandwichReport(query *queryv1.SandwichQuery) *queryv1.Report {
	return &queryv1.Report{Sandwich: &queryv1.SandwichReport{
		Query: query.CloneVT(), Sandwich: new(querierv1.SandwichReport),
	}}
}

type sandwichAggregator struct {
	init   sync.Once
	query  *queryv1.SandwichQuery
	merger model.SandwichMerger
}

func newSandwichAggregator(*queryv1.InvokeRequest) aggregator { return new(sandwichAggregator) }

func (a *sandwichAggregator) aggregate(report *queryv1.Report) error {
	r := report.Sandwich
	if r == nil || r.Sandwich == nil {
		return fmt.Errorf("sandwich report is required")
	}
	a.init.Do(func() { a.query = r.Query.CloneVT() })
	a.merger.Merge(r.Sandwich)
	return nil
}

func (a *sandwichAggregator) build() *queryv1.Report {
	return &queryv1.Report{Sandwich: &queryv1.SandwichReport{
		// Untruncated: the frontend applies the node budget once it has the
		// complete result.
		Query: a.query, Sandwich: a.merger.Sandwich(-1),
	}}
}

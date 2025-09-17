package querybackend

import (
	"sync"

	"github.com/grafana/dskit/runutil"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/block"
	"github.com/grafana/pyroscope/pkg/model"
	parquetquery "github.com/grafana/pyroscope/pkg/phlaredb/query"
	v1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
)

func init() {
	registerQueryType(
		queryv1.QueryType_QUERY_TREE,
		queryv1.ReportType_REPORT_TREE,
		queryTree,
		newTreeAggregator,
		[]block.Section{
			block.SectionTSDB,
			block.SectionProfiles,
			block.SectionSymbols,
		}...,
	)
}

func queryTree(q *queryContext, query *queryv1.Query) (*queryv1.Report, error) {
	entries, err := profileEntryIterator(q)
	if err != nil {
		return nil, err
	}
	defer runutil.CloseWithErrCapture(&err, entries, "failed to close profile entry iterator")

	spanSelector, err := model.NewSpanSelector(query.Tree.SpanSelector)
	if err != nil {
		return nil, err
	}

	var columns v1.SampleColumns
	if err = columns.Resolve(q.ds.Profiles().Schema()); err != nil {
		return nil, err
	}

	indices := []int{
		columns.StacktraceID.ColumnIndex,
		columns.Value.ColumnIndex,
	}
	if len(spanSelector) > 0 {
		indices = append(indices, columns.SpanID.ColumnIndex)
	}

	resolverOptions := []symdb.ResolverOption{
		symdb.WithResolverMaxNodes(query.Tree.MaxNodes),
	}
	if query.Tree.StackTraceSelector != nil {
		resolverOptions = append(resolverOptions, symdb.WithResolverStackTraceSelector(query.Tree.StackTraceSelector))
	}

	profiles := parquetquery.NewRepeatedRowIterator(q.ctx, entries, q.ds.Profiles().RowGroups(), indices...)
	defer runutil.CloseWithErrCapture(&err, profiles, "failed to close profile stream")

	resolver := symdb.NewResolver(q.ctx, q.ds.Symbols(), resolverOptions...)
	defer resolver.Release()

	if len(spanSelector) > 0 {
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
	} else {
		for profiles.Next() {
			p := profiles.At()
			resolver.AddSamplesFromParquetRow(p.Row.Partition, p.Values[0], p.Values[1])
		}
	}

	if err = profiles.Err(); err != nil {
		return nil, err
	}

	tree, err := resolver.Tree()
	if err != nil {
		return nil, err
	}

	resp := &queryv1.Report{
		Tree: &queryv1.TreeReport{
			Query: query.Tree.CloneVT(),
			Tree:  tree.Bytes(query.Tree.GetMaxNodes()),
		},
	}
	return resp, nil
}

type treeAggregator struct {
	init  sync.Once
	query *queryv1.TreeQuery
	tree  *model.TreeMerger
}

func newTreeAggregator(*queryv1.InvokeRequest) aggregator { return new(treeAggregator) }

func (a *treeAggregator) aggregate(report *queryv1.Report) error {
	r := report.Tree
	a.init.Do(func() {
		a.tree = model.NewTreeMerger()
		a.query = r.Query.CloneVT()
	})
	return a.tree.MergeTreeBytes(r.Tree)
}

func (a *treeAggregator) build() *queryv1.Report {
	return &queryv1.Report{
		Tree: &queryv1.TreeReport{
			Query: a.query,
			Tree:  a.tree.Tree().Bytes(a.query.GetMaxNodes()),
		},
	}
}

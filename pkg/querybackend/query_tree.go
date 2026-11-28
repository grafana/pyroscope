package querybackend

import (
	"sync"

	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/model"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/symdb"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

func init() {
	registerQueryType(
		queryv1.QueryType_QUERY_TREE,
		queryv1.ReportType_REPORT_TREE,
		queryTree,
		newTreeAggregator,
		false,
		[]block.Section{
			block.SectionTSDB,
			block.SectionProfiles,
			block.SectionSymbols,
		}...,
	)
}

func queryTree(q *queryContext, query *queryv1.Query) (*queryv1.Report, error) {
	resolver := symdb.NewResolver(q.ctx, q.ds.Symbols(),
		symdb.WithResolverMaxNodes(query.Tree.MaxNodes),
		symdb.WithResolverStackTraceSelector(query.Tree.StackTraceSelector))
	defer resolver.Release()
	hasColumns, err := collectStacktraceSamples(q, resolver, query.Tree.ProfileIdSelector, query.Tree.SpanSelector, query.Tree.TraceIdSelector)
	if err != nil {
		return nil, err
	}
	if !hasColumns {
		return &queryv1.Report{Tree: &queryv1.TreeReport{Query: query.Tree.CloneVT()}}, nil
	}

	// output full pprof tree if that's requested
	if query.Tree.FullSymbols {
		tree, symbolBuilder, err := resolver.LocationRefNameTree()
		if err != nil {
			return nil, err
		}
		resp := &queryv1.Report{
			Tree: &queryv1.TreeReport{
				Query:   query.Tree.CloneVT(),
				Tree:    tree.Bytes(query.Tree.GetMaxNodes(), symbolBuilder.KeepSymbol),
				Symbols: new(queryv1.TreeSymbols),
			},
		}
		symbolBuilder.Build(resp.Tree.Symbols)
		return resp, nil
	}

	tree, err := resolver.Tree()
	if err != nil {
		return nil, err
	}

	resp := &queryv1.Report{
		Tree: &queryv1.TreeReport{
			Query: query.Tree.CloneVT(),
			Tree:  tree.Bytes(query.Tree.GetMaxNodes(), nil),
		},
	}
	return resp, nil
}

type treeAggregator struct {
	init  sync.Once
	query *queryv1.TreeQuery
	tree  *model.TreeMerger[model.FunctionName, model.FunctionNameI]

	lrTree       *model.TreeMerger[model.LocationRefName, model.LocationRefNameI]
	symbolLock   sync.Mutex
	symbolMerger *symdb.SymbolMerger
}

func newTreeAggregator(*queryv1.InvokeRequest) aggregator { return new(treeAggregator) }

func (a *treeAggregator) aggregate(report *queryv1.Report) error {
	r := report.Tree
	if r.Query.FullSymbols {
		a.init.Do(func() {
			a.lrTree = model.NewTreeMerger[model.LocationRefName, model.LocationRefNameI]()
			a.query = r.Query.CloneVT()
			a.symbolMerger = symdb.NewSymbolMerger()
		})
		a.symbolLock.Lock()
		defer a.symbolLock.Unlock()
		adder, err := a.symbolMerger.Add(r.Symbols)
		if err != nil {
			return err
		}
		return a.lrTree.MergeTreeBytes(r.Tree, model.WithTreeMergeFormatNodeNames(adder))
	}

	a.init.Do(func() {
		a.tree = model.NewTreeMerger[model.FunctionName, model.FunctionNameI]()
		a.query = r.Query.CloneVT()
	})
	return a.tree.MergeTreeBytes(r.Tree)
}

func (a *treeAggregator) build() *queryv1.Report {
	result := &queryv1.Report{
		Tree: &queryv1.TreeReport{
			Query: a.query,
		},
	}

	if a.query.FullSymbols {
		builder := a.symbolMerger.ResultBuilder()
		result.Tree.Tree = a.lrTree.Tree().Bytes(a.query.GetMaxNodes(), builder.KeepSymbol)
		result.Tree.Symbols = new(queryv1.TreeSymbols)
		builder.Build(result.Tree.Symbols)
		return result
	}

	result.Tree.Tree = a.tree.Tree().Bytes(a.query.GetMaxNodes(), nil)
	return result
}

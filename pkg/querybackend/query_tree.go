package querybackend

import (
	"strings"
	"sync"

	"github.com/grafana/dskit/runutil"
	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"

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
		false,
		[]block.Section{
			block.SectionTSDB,
			block.SectionProfiles,
			block.SectionSymbols,
		}...,
	)
}

func queryTree(q *queryContext, query *queryv1.Query) (*queryv1.Report, error) {
	span := opentracing.SpanFromContext(q.ctx)

	var profileOpts []profileIteratorOption
	if len(query.Tree.ProfileIdSelector) > 0 {
		opt, err := withProfileIDSelector(query.Tree.ProfileIdSelector...)
		if err != nil {
			return nil, err
		}
		profileOpts = append(profileOpts, opt)
		span.SetTag("profile_id_selector.count", len(query.Tree.ProfileIdSelector))
		if len(query.Tree.ProfileIdSelector) <= maxProfileIDsToLog {
			span.LogFields(otlog.String("profile_ids", strings.Join(query.Tree.ProfileIdSelector, ",")))
		}
	}

	entries, err := profileEntryIterator(q, profileOpts...)
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
	tree  *model.TreeMerger[model.FuntionName, model.FuntionNameI]

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
		a.tree = model.NewTreeMerger[model.FuntionName, model.FuntionNameI]()
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

package querybackend

import (
	"fmt"
	"strings"
	"sync"

	"github.com/grafana/dskit/runutil"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/block"
	"github.com/grafana/pyroscope/v2/pkg/model"
	parquetquery "github.com/grafana/pyroscope/v2/pkg/phlaredb/query"
	v1 "github.com/grafana/pyroscope/v2/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/v2/pkg/phlaredb/symdb"
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

// treeSymbolMode resolves the symbol output requested by a tree query. When
// symbol_mode is unset it falls back to the deprecated full_symbols bool for
// wire compatibility; setting both is rejected.
func treeSymbolMode(t *queryv1.TreeQuery) (queryv1.SymbolMode, error) {
	mode := t.GetSymbolMode()
	if t.GetFullSymbols() { //nolint:staticcheck // bridges the deprecated full_symbols bool
		if mode != queryv1.SymbolMode_SYMBOL_MODE_UNSPECIFIED {
			return 0, fmt.Errorf("full_symbols must not be combined with symbol_mode")
		}
		return queryv1.SymbolMode_SYMBOL_MODE_FULL, nil
	}
	if mode == queryv1.SymbolMode_SYMBOL_MODE_UNSPECIFIED {
		return queryv1.SymbolMode_SYMBOL_MODE_NAME, nil
	}
	return mode, nil
}

func queryTree(q *queryContext, query *queryv1.Query) (*queryv1.Report, error) {
	mode, err := treeSymbolMode(query.Tree)
	if err != nil {
		return nil, err
	}

	otelSpan := trace.SpanFromContext(q.ctx)

	profileOpts := []profileIteratorOption{withExcludeSampled()}
	if len(query.Tree.ProfileIdSelector) > 0 {
		opt, err := withProfileIDSelector(query.Tree.ProfileIdSelector...)
		if err != nil {
			return nil, err
		}
		profileOpts = append(profileOpts, opt)
		otelSpan.SetAttributes(attribute.Int("profile_id_selector.count", len(query.Tree.ProfileIdSelector)))
		if len(query.Tree.ProfileIdSelector) <= maxProfileIDsToLog {
			otelSpan.SetAttributes(attribute.String("profile_ids", strings.Join(query.Tree.ProfileIdSelector, ",")))
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

	traceSelector, err := model.NewTraceSelector(query.Tree.TraceIdSelector)
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
			return &queryv1.Report{Tree: &queryv1.TreeReport{Query: query.Tree.CloneVT()}}, nil
		}
		indices = append(indices, columns.SpanID.ColumnIndex)
	case len(traceSelector) > 0:
		if !columns.HasTraceID() {
			// Block has no TraceID column: no samples can match the trace selector.
			return &queryv1.Report{Tree: &queryv1.TreeReport{Query: query.Tree.CloneVT()}}, nil
		}
		indices = append(indices, columns.TraceID.ColumnIndex)
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

	// output full pprof tree if that's requested
	if mode == queryv1.SymbolMode_SYMBOL_MODE_FULL {
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
	mode  queryv1.SymbolMode
	query *queryv1.TreeQuery
	tree  *model.TreeMerger[model.FunctionName, model.FunctionNameI]

	lrTree       *model.TreeMerger[model.LocationRefName, model.LocationRefNameI]
	symbolLock   sync.Mutex
	symbolMerger *symdb.SymbolMerger
}

func newTreeAggregator(*queryv1.InvokeRequest) aggregator { return new(treeAggregator) }

func (a *treeAggregator) aggregate(report *queryv1.Report) error {
	r := report.Tree
	mode, err := treeSymbolMode(r.Query)
	if err != nil {
		return err
	}
	if mode == queryv1.SymbolMode_SYMBOL_MODE_FULL {
		a.init.Do(func() {
			a.mode = mode
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
		a.mode = mode
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

	if a.mode == queryv1.SymbolMode_SYMBOL_MODE_FULL {
		builder := a.symbolMerger.ResultBuilder()
		result.Tree.Tree = a.lrTree.Tree().Bytes(a.query.GetMaxNodes(), builder.KeepSymbol)
		result.Tree.Symbols = new(queryv1.TreeSymbols)
		builder.Build(result.Tree.Symbols)
		return result
	}

	result.Tree.Tree = a.tree.Tree().Bytes(a.query.GetMaxNodes(), nil)
	return result
}

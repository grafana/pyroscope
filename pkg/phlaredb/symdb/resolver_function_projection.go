package symdb

import (
	"slices"

	"github.com/grafana/dskit/tracing"

	"github.com/grafana/pyroscope/v2/pkg/model"
	schemav1 "github.com/grafana/pyroscope/v2/pkg/phlaredb/schemas/v1"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

// FunctionTree resolves each stack once, sharing symbol resolution between directions.
func (r *Resolver) FunctionTree(options *typesv1.FunctionTreeOptions) (*typesv1.FunctionTree, error) {
	span, ctx := tracing.StartSpanFromContext(r.ctx, "Resolver.FunctionTree")
	defer span.Finish()

	trees := model.NewFunctionTreeMerger(options)
	err := r.withSymbols(ctx, func(symbols *Symbols, appender *SampleAppender) error {
		projection := newCallTreeSymbols(symbols, appender.Samples(), r.sts, options)
		if !projection.valid {
			return nil
		}
		if err := symbols.Stacktraces.ResolveStacktraceLocations(ctx, projection, projection.samples.StacktraceIDs); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		trees.Merge(projection.builder.Tree())
		return nil
	})
	if err != nil {
		return nil, err
	}
	return trees.Tree(), nil
}

func (r *Resolver) Functions() (*typesv1.FunctionTable, error) {
	span, ctx := tracing.StartSpanFromContext(r.ctx, "Resolver.Functions")
	defer span.Finish()

	functions := model.NewFunctionTableMerger()
	err := r.withSymbols(ctx, func(symbols *Symbols, appender *SampleAppender) error {
		projection := newFunctionTableSymbols(symbols, appender.Samples(), r.sts)
		if !projection.valid {
			return nil
		}
		if err := symbols.Stacktraces.ResolveStacktraceLocations(ctx, projection, projection.samples.StacktraceIDs); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		functions.Merge(projection.table())
		return nil
	})
	if err != nil {
		return nil, err
	}
	return functions.Table(), nil
}

type functionValue struct {
	total      int64
	self       int64
	lastSample int
}

type functionTableSymbols struct {
	*projectionSymbols
	total  int64
	values []functionValue
}

func newFunctionTableSymbols(symbols *Symbols, samples schemav1.Samples, selector *typesv1.StackTraceSelector) *functionTableSymbols {
	r := &functionTableSymbols{
		projectionSymbols: newProjectionSymbols(symbols, samples, selector),
	}
	r.values = make([]functionValue, len(r.names))
	for i := range r.values {
		r.values[i].lastSample = -1
	}
	return r
}

func (r *functionTableSymbols) InsertStacktrace(_ uint32, locations []int32) {
	value, sampleIndex := r.nextValue()
	if value <= 0 || !r.matchesRootPath(locations) {
		return
	}
	r.total += value
	leaf := true
	for _, location := range locations {
		for _, line := range r.symbols.Locations[location].Line {
			v := &r.values[r.functionIndex[line.FunctionId]]
			if v.lastSample != sampleIndex {
				v.total += value
				v.lastSample = sampleIndex
			}
			if leaf {
				v.self += value
				leaf = false
			}
		}
	}
}

func (r *functionTableSymbols) table() *typesv1.FunctionTable {
	t := &typesv1.FunctionTable{Total: r.total}
	for i := range r.values {
		v := &r.values[i]
		if v.total > 0 {
			t.Functions = append(t.Functions, &typesv1.FunctionStats{
				Name:  r.names[i],
				Total: v.total,
				Self:  v.self,
			})
		}
	}
	t.TotalFunctions = int64(len(t.Functions))
	return t
}

type callTreeSymbols struct {
	*projectionSymbols
	stack    []int32
	rootPath bool
	builder  *model.FunctionTreeBuilder[int32]
}

func newCallTreeSymbols(symbols *Symbols, samples schemav1.Samples, selector *typesv1.StackTraceSelector, options *typesv1.FunctionTreeOptions) *callTreeSymbols {
	index := newProjectionSymbols(symbols, samples, selector)
	return &callTreeSymbols{
		projectionSymbols: index,
		rootPath:          options.GetSelection() == typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_ROOT_PATH,
		builder:           model.NewFunctionTreeBuilder(index.callSite, options, index.lookup),
	}
}

func (r *callTreeSymbols) InsertStacktrace(_ uint32, locations []int32) {
	value, _ := r.nextValue()
	if value <= 0 || (r.rootPath && !r.matchesRootPath(locations)) {
		return
	}
	r.stack = r.stack[:0]
	// Iterate leaf-first locations and inline frames backwards to build root-first IDs.
	for _, location := range slices.Backward(locations) {
		lines := r.symbols.Locations[location].Line
		for _, line := range slices.Backward(lines) {
			r.stack = append(r.stack, r.functionIndex[line.FunctionId])
		}
	}
	r.builder.Insert(r.stack, value)
}

type projectionSymbols struct {
	symbols       *Symbols
	samples       schemav1.Samples
	cur           int
	functionIndex []int32
	names         []string
	callSite      []int32
	valid         bool
}

func newProjectionSymbols(symbols *Symbols, samples schemav1.Samples, selector *typesv1.StackTraceSelector) *projectionSymbols {
	selection := SelectStackTraces(symbols, selector)
	r := &projectionSymbols{
		symbols:       symbols,
		samples:       samples,
		functionIndex: make([]int32, len(selection.funcNames)),
		// Zero is reserved for the synthetic root, including when no symbols exist.
		names: make([]string, 1, len(selection.funcNames)+1),
		valid: selection.HasValidCallSite(),
	}
	byName := make(map[string]int32, len(selection.funcNames))
	for i, name := range selection.funcNames {
		id, ok := byName[name]
		if !ok {
			id = int32(len(r.names))
			byName[name] = id
			r.names = append(r.names, name)
		}
		r.functionIndex[i] = id
	}
	for _, name := range selection.callSite {
		id, ok := byName[name]
		if !ok {
			r.valid = false
		}
		r.callSite = append(r.callSite, id)
	}
	return r
}

func (r *projectionSymbols) lookup(id int32) string {
	return r.names[id]
}

func (r *projectionSymbols) matchesRootPath(locations []int32) bool {
	return r.valid && matchesCallSite(r.symbols, locations, r.functionIndex, r.callSite)
}

func (r *projectionSymbols) nextValue() (int64, int) {
	index := r.cur
	value := int64(r.samples.Values[index])
	r.cur++
	return value, index
}

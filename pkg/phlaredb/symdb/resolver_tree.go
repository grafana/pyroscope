package symdb

import (
	"bytes"
	"sync"

	"github.com/grafana/pyroscope/pkg/model"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

type treeSymbols struct {
	symbols  *Symbols
	samples  *schemav1.Samples
	tree     *model.StacktraceTree
	maxNodes int64
	lines    []int32
	cur      int
}

var treeSymbolsPool = sync.Pool{
	New: func() any { return new(treeSymbols) },
}

func treeSymbolsFromPool() *treeSymbols {
	return treeSymbolsPool.Get().(*treeSymbols)
}

func (r *treeSymbols) reset() {
	r.symbols = nil
	r.samples = nil
	r.tree.Reset()
	r.lines = r.lines[:0]
	r.cur = 0
	treeSymbolsPool.Put(r)
}

func (r *treeSymbols) init(symbols *Symbols, samples schemav1.Samples) {
	r.symbols = symbols
	r.samples = &samples
	if r.tree == nil {
		// Branching factor.
		r.tree = model.NewStacktraceTree(samples.Len() * 2)
	}
}

func (r *treeSymbols) InsertStacktrace(_ uint32, locations []int32) {
	r.lines = r.lines[:0]
	for i := 0; i < len(locations); i++ {
		lines := r.symbols.Locations[locations[i]].Line
		for j := 0; j < len(lines); j++ {
			f := r.symbols.Functions[lines[j].FunctionId]
			r.lines = append(r.lines, int32(f.Name))
		}
	}
	r.tree.Insert(r.lines, int64(r.samples.Values[r.cur]))
	r.cur++
}

func (r *treeSymbols) buildTree() *model.Tree {
	// TODO(kolesnikovae): Eliminate intermediate serialization.
	var buf bytes.Buffer
	r.tree.Bytes(&buf, r.maxNodes, r.symbols.Strings)
	t, err := model.UnmarshalTree(buf.Bytes())
	if err != nil {
		panic(err)
	}
	return t
}

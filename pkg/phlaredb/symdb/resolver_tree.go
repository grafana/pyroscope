package symdb

import (
	"sync"

	"github.com/grafana/pyroscope/pkg/model"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

type treeSymbols struct {
	symbols *Symbols
	samples *schemav1.Samples
	tree    *model.Tree
	lines   []string
	cur     int
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
	r.tree = nil
	r.lines = r.lines[:0]
	r.cur = 0
	treeSymbolsPool.Put(r)
}

func (r *treeSymbols) init(symbols *Symbols, samples schemav1.Samples) {
	r.symbols = symbols
	r.samples = &samples
	r.tree = new(model.Tree)
}

func (r *treeSymbols) InsertStacktrace(_ uint32, locations []int32) {
	r.lines = r.lines[:0]
	for i := len(locations) - 1; i >= 0; i-- {
		lines := r.symbols.Locations[locations[i]].Line
		for j := len(lines) - 1; j >= 0; j-- {
			f := r.symbols.Functions[lines[j].FunctionId]
			r.lines = append(r.lines, r.symbols.Strings[f.Name])
		}
	}
	r.tree.InsertStack(int64(r.samples.Values[r.cur]), r.lines...)
	r.cur++
}

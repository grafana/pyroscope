package symdb

import (
	"context"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/grafana/pyroscope/pkg/model"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/grafana/pyroscope/pkg/util/minheap"
)

func buildTree(
	ctx context.Context,
	symbols *Symbols,
	appender *SampleAppender,
	maxNodes int64,
) (*model.Tree, error) {
	// If the number of samples is large and the StacktraceResolver
	// implements the range iterator, we will be building the tree
	// based on the parent pointer tree of the partition.
	// This is significantly more efficient than building the tree
	// from the ground up by inserting each stack trace.
	iterator, ok := symbols.Stacktraces.(StacktraceIDRangeIterator)
	if ok && appender.Len() > 1<<20 {
		m := model.NewTreeMerger()
		g, _ := errgroup.WithContext(ctx)
		ranges := iterator.SplitStacktraceIDRanges(appender)
		for ranges.Next() {
			sr := ranges.At()
			g.Go(util.RecoverPanic(func() error {
				m.MergeTree(buildTreeForStacktraceIDRange(sr, symbols, maxNodes))
				return nil
			}))
		}
		if err := g.Wait(); err != nil {
			return nil, err
		}
		return m.Tree(), nil
	}
	// Otherwise, use the basic approach: resolve each stack trace
	// and insert them into the new tree one by one.
	samples := appender.Samples()
	t := treeSymbolsFromPool()
	defer t.reset()
	t.init(symbols, samples)
	if err := symbols.Stacktraces.ResolveStacktraceLocations(ctx, t, samples.StacktraceIDs); err != nil {
		return nil, err
	}
	return t.tree.Tree(maxNodes, t.symbols.Strings), nil
}

type treeSymbols struct {
	symbols *Symbols
	samples *schemav1.Samples
	tree    *model.StacktraceTree
	lines   []int32
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

func buildTreeForStacktraceIDRange(
	stacktraces *StacktraceIDRange,
	symbols *Symbols,
	maxNodes int64,
) *model.Tree {
	nodes := stacktraces.Nodes()
	stacktraces.SetNodeValues(nodes)
	propagateNodeValues(nodes)
	// We preserve more nodes than requested to preserve more
	// locations with inlined functions. The multiplier is
	// chosen empirically; it should be roughly equal to the
	// ratio of nodes in the location tree to the nodes in the
	// function tree (after truncation).
	markNodesForTruncation(nodes, maxNodes*4)
	t := model.NewStacktraceTree(int(maxNodes))
	l := int32(len(nodes))
	s := make([]int32, 0, 64)
	for i := int32(1); i < l; i++ {
		p := nodes[i].Parent
		v := nodes[i].Value
		if v > 0 && nodes[p].Location&truncationMark == 0 {
			s = resolveStack(s, nodes, i, symbols)
			t.Insert(s, v)
		}
	}
	return t.Tree(maxNodes, symbols.Strings)
}

func propagateNodeValues(nodes []Node) {
	// Step 1: Set leaf values.
	// This is already done in SetNodeValues.
	// Presence of the value does not indicate
	// that the node is a leaf: it may have its
	// own value and children.

	// Step 2: Propagate values to the direct
	// parent. We iterate the nodes in reverse
	// order to ensure that all the descendants
	// have been processed before the parent,
	// and its value includes all the children.
	for i := len(nodes) - 1; i >= 1; i-- {
		if p := nodes[i].Parent; p > 0 {
			nodes[p].Value += nodes[i].Value
		}
	}
	// Step 3: Find edge nodes: ones that have
	// children, but do not have a value.
	// Sum up the values of the children and
	// mark the node – their values are to
	// be propagated to the parent chain at
	// the next step.
	const mark = 1 << 30
	for i := len(nodes) - 1; i >= 1; i-- {
		if p := nodes[i].Parent; p > 0 && nodes[p].Value == 0 {
			nodes[p].Value += nodes[i].Value
			nodes[p].Location |= mark
		}
	}
	// Step 4: Propagate the edge node values
	// to the parent chain. Propagation stops
	// once we find another edge node in the
	// chain: we add own value to it, which will
	// be propagated further, after all the
	// downstream edges converge.
	for i := len(nodes) - 1; i >= 1; i-- {
		if nodes[i].Location&mark > 0 {
			nodes[i].Location &= ^mark
			v := nodes[i].Value
			j := nodes[i].Parent
			for j >= 0 {
				nodes[j].Value += v
				if nodes[j].Location&mark != 0 {
					break
				}
				j = nodes[j].Parent
			}
		}
	}
}

func markNodesForTruncation(dst []Node, maxNodes int64) {
	m := minValue(dst, maxNodes)
	for i := 1; i < len(dst); i++ {
		if dst[i].Value < m {
			dst[i].Location |= truncationMark
			// Preserve value of the truncated location on leaves.
			if dst[dst[i].Parent].Location&truncationMark != 0 {
				continue
			}
		}
		dst[dst[i].Parent].Value -= dst[i].Value
	}
}

func resolveStack(dst []int32, nodes []Node, i int32, symbols *Symbols) []int32 {
	dst = dst[:0]
	for i > 0 {
		j := nodes[i].Location
		if j&truncationMark > 0 {
			dst = append(dst, sentinel)
		} else {
			loc := symbols.Locations[j]
			for l := 0; l < len(loc.Line); l++ {
				dst = append(dst, int32(symbols.Functions[loc.Line[l].FunctionId].Name))
			}
		}
		i = nodes[i].Parent
	}
	return dst
}

func minValue(nodes []Node, maxNodes int64) int64 {
	if maxNodes < 1 || maxNodes >= int64(len(nodes)) {
		return 0
	}
	h := make([]int64, 0, maxNodes)
	for i := range nodes {
		v := nodes[i].Value
		if len(h) >= int(maxNodes) {
			if v > h[0] {
				h = minheap.Pop(h)
			} else {
				continue
			}
		}
		h = minheap.Push(h, v)
	}
	if len(h) < int(maxNodes) {
		return 0
	}
	return h[0]
}

package symdb

import (
	"context"
	"strconv"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/grafana/pyroscope/pkg/iter"
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
	selection *SelectedStackTraces,
) (*model.Tree, error) {
	if !selection.HasValidCallSite() {
		// TODO(bryan) Maybe return an error here? buildPprof returns a blank
		// profile. So mimicking that behavior for now.
		return &model.Tree{}, nil
	}

	// If the number of samples is large (> 128K) and the StacktraceResolver
	// implements the range iterator, we will be building the tree based on
	// the parent pointer tree of the partition (a copy of). The only exception
	// is when the number of nodes is not limited, or is close to the number of
	// nodes in the original tree: the optimization is still beneficial in terms
	// of CPU, but is very expensive in terms of memory.
	iterator, ok := symbols.Stacktraces.(StacktraceIDRangeIterator)
	if ok && shouldCopyTree(appender, maxNodes) {
		ranges := iterator.SplitStacktraceIDRanges(appender)
		return buildTreeFromParentPointerTrees(ctx, ranges, symbols, maxNodes, selection)
	}
	// Otherwise, use the basic approach: resolve each stack trace
	// and insert them into the new tree one by one. The method
	// performs best on small sample sets.
	samples := appender.Samples()
	t := treeSymbolsFromPool()
	defer t.reset()
	t.init(symbols, samples, selection)
	if err := symbols.Stacktraces.ResolveStacktraceLocations(ctx, t, samples.StacktraceIDs); err != nil {
		return nil, err
	}
	return t.tree.Tree(maxNodes, t.symbols.Strings), nil
}

func shouldCopyTree(appender *SampleAppender, maxNodes int64) bool {
	const copyThreshold = 128 << 10
	expensiveTruncation := maxNodes <= 0 || maxNodes > int64(appender.Len())
	return appender.Len() > copyThreshold && !expensiveTruncation
}

type treeSymbols struct {
	symbols *Symbols
	samples *schemav1.Samples
	tree    *model.StacktraceTree
	lines   []int32
	cur     int

	selection        *SelectedStackTraces
	funcNamesMatcher func(funcNames []int32) bool
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

func (r *treeSymbols) init(symbols *Symbols, samples schemav1.Samples, selection *SelectedStackTraces) {
	r.symbols = symbols
	r.samples = &samples
	r.selection = selection

	if r.tree == nil {
		// Branching factor.
		r.tree = model.NewStacktraceTree(samples.Len() * 2)
	}
	if r.selection != nil && len(r.selection.callSite) > 0 {
		r.funcNamesMatcher = r.funcNamesMatchSelection
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
	if r.funcNamesMatcher == nil || r.funcNamesMatcher(r.lines) {
		r.tree.Insert(r.lines, int64(r.samples.Values[r.cur]))
	}
	r.cur++
}

// funcNamesMatchSelection checks if the funcNames match the selection.
// Note funcNames is a slice of function name references and is reversed. The first item is the last function in the stack trace.
func (r *treeSymbols) funcNamesMatchSelection(funcNames []int32) bool {
	if len(funcNames) < int(r.selection.depth) {
		return false
	}

	for i := 0; i < int(r.selection.depth); i++ {
		if r.symbols.Strings[funcNames[len(funcNames)-1-i]] != r.selection.callSite[i] {
			return false
		}
	}
	return true
}

func buildTreeFromParentPointerTrees(
	ctx context.Context,
	ranges iter.Iterator[*StacktraceIDRange],
	symbols *Symbols,
	maxNodes int64,
	selection *SelectedStackTraces,
) (*model.Tree, error) {
	m := model.NewTreeMerger()
	g, _ := errgroup.WithContext(ctx)
	for ranges.Next() {
		sr := ranges.At()
		g.Go(util.RecoverPanic(func() error {
			m.MergeTree(buildTreeForStacktraceIDRange(sr, symbols, maxNodes, selection))
			return nil
		}))
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return m.Tree(), nil
}

type nodeResult int64

const (
	nodeResultUnknown nodeResult = iota
	nodeResultMatch
	nodeResultDescendant
	nodeResultAncestor
	nodeResultNoMatch
)

func nodeMatch(n *Node, selectionIdx int, symbols *Symbols, selection *SelectedStackTraces) int {
	resetPos := int(selection.depth - 1)
	for _, l := range symbols.Locations[n.Location].Line {
		if selectionIdx < 0 {
			return resetPos
		}
		if selection.callSite[selectionIdx] != selection.funcNames[l.FunctionId] {
			return resetPos
		}
		selectionIdx--
	}
	return selectionIdx
}

func markAncestors(idx int, nodes []Node, result nodeResult) {
	markNAncestors(idx, nodes, result, -1)
}

func markNAncestors(idx int, nodes []Node, result nodeResult, depth int) {
	count := 0
	for idx != sentinel {
		if depth > 0 && count >= depth {
			break
		}
		if nodes[idx].Value != int64(nodeResultUnknown) {
			break
		}
		nodes[idx].Value = int64(result)
		idx = int(nodes[idx].Parent)
		count++
	}
}

func selectNodes(
	symbols *Symbols,
	selection *SelectedStackTraces,
	nodes []Node,
) []Node {

	var (
		current      int
		depth        int
		selectionIdx int
	)

	// iterate over all nodes and check if they or their descendants match the selection
	for idx := range nodes {
		current = idx
		selectionIdx = int(selection.depth) - 1
		depth = 0
		for {
			// if node result is know, we can mark nodes right away
			currentResult := nodeResult(nodes[current].Value)
			if currentResult != nodeResultUnknown {
				switch currentResult {
				case nodeResultDescendant, nodeResultMatch:
					markAncestors(idx, nodes, nodeResultDescendant)
				case nodeResultAncestor, nodeResultNoMatch:
					markAncestors(idx, nodes, nodeResultNoMatch)
				default:
					panic("unhandled node result: " + strconv.Itoa(int(currentResult)))
				}
				break
			}

			// check if the strings match on this node
			selectionIdx = nodeMatch(&nodes[current], selectionIdx, symbols, selection)

			// if the next node is the root or we are on the root node already break
			if next := nodes[current].Parent; next == sentinel || nodes[next].Parent == sentinel {
				if selectionIdx == -1 {
					// we found the match
					matchNode := idx
					for i := 0; i < depth-int(selection.depth); i++ {
						matchNode = int(nodes[matchNode].Parent)
					}
					markAncestors(int(nodes[matchNode].Parent), nodes, nodeResultAncestor)
					nodes[matchNode].Value = int64(nodeResultMatch)
					markAncestors(idx, nodes, nodeResultDescendant)
					break
				}

				// mark everything that is deepeer than the selection as no match
				if depth > int(selection.depth) {
					markNAncestors(idx, nodes, nodeResultNoMatch, depth-int(selection.depth))
				}
				break
			}

			current = int(nodes[current].Parent)
			depth++
		}
	}

	// iterate once again over all nodes and mark the nodes that are not matched as truncated
	for idx := range nodes {
		if nodes[idx].Value != int64(nodeResultDescendant) && nodes[idx].Value != int64(nodeResultMatch) {
			// mark them as truncated
			nodes[idx].Location |= truncationMark
		}
		// reset the value
		nodes[idx].Value = 0
	}

	return nodes

}

func buildTreeForStacktraceIDRange(
	stacktraces *StacktraceIDRange,
	symbols *Symbols,
	maxNodes int64,
	selection *SelectedStackTraces,
) *model.Tree {
	// Get the parent pointer tree for the range. The tree is
	// not specific to the samples we've collected and includes
	// all the stack traces.
	nodes := stacktraces.Nodes()
	// Filter stacktrace filter
	if selection != nil && len(selection.callSite) > 0 {
		nodes = selectNodes(symbols, selection, nodes)
	}

	// SetNodeValues sets values to the nodes that match the
	// samples we've collected; those are not always leaves:
	// a node may have its own value (self) and children.
	stacktraces.SetNodeValues(nodes)
	// Propagate the values to the parent nodes. This is required
	// to identify the nodes that should be removed from the tree.
	// For each node, the value should be a sum of all the child
	// nodes (total).
	propagateNodeValues(nodes)
	// Next step is truncation: we need to mark leaf nodes of the
	// stack traces we want to keep, and ensure that their values
	// reflect their own weight (total for truncated leaves, self
	// for the true leaves).
	// We preserve more nodes than requested to preserve more
	// locations with inlined functions. The multiplier is
	// chosen empirically; it should be roughly equal to the
	// ratio of nodes in the location tree to the nodes in the
	// function tree (after truncation).
	markNodesForTruncation(nodes, maxNodes*4)
	// We now build an intermediate tree from the marked stack
	// traces. The reason is that the intermediate tree is
	// substantially bigger than the final one. The intermediate
	// tree is optimized for inserts and lookups, while the output
	// tree is optimized for merge operations.
	t := model.NewStacktraceTree(int(maxNodes))
	insertStacktraces(t, nodes, symbols)
	// Finally, we convert the stack trace tree into the function
	// tree, dropping insignificant functions, and symbolizing the
	// nodes (function names).
	return t.Tree(maxNodes, symbols.Strings)
}

func propagateNodeValues(nodes []Node) {
	for i := len(nodes) - 1; i >= 1; i-- {
		if p := nodes[i].Parent; p > 0 {
			nodes[p].Value += nodes[i].Value
		}
	}
}

func markNodesForTruncation(nodes []Node, maxNodes int64) {
	m := minValue(nodes, maxNodes)
	for i := 1; i < len(nodes); i++ {
		p := nodes[i].Parent
		v := nodes[i].Value
		// Remove previous truncation mark, potential set by the stacktrace filter
		nodes[i].Location &= ^truncationMark
		if v < m {
			nodes[i].Location |= truncationMark
			// Preserve values of truncated locations. The weight
			// of the truncated chain is accounted in the parent.
			if p >= 0 && nodes[p].Location&truncationMark != 0 {
				continue
			}
		}
		// Subtract the value of the location from the parent:
		// by doing so we ensure that the transient nodes have zero
		// weight, and then will be ignored by the tree builder.
		if p >= 0 {
			nodes[p].Value -= v
		}
	}
}

func insertStacktraces(t *model.StacktraceTree, nodes []Node, symbols *Symbols) {
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

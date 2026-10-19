package symdb

import (
	"runtime"
	"slices"
	"sync"

	"github.com/grafana/pyroscope/v2/pkg/model"
)

// Parent pointer trees with fewer nodes are mapped by a single goroutine.
var parallelCallTreeMinNodes = 256 << 10

// buildLocationCallTree builds the call tree of function names from the
// parent pointer tree of the stack traces, node by node. Each location node
// with sampled descendants is mapped to the call tree node of its parent,
// extended with the location's lines (inlined functions), outermost first.
//
// If truncate is set, the stack traces are truncated as by
// buildTreeForStacktraceIDRange before building the function tree:
// locations with subtree totals below the maxNodes*4-th largest are dropped,
// and the weight of each topmost dropped subtree is kept in a
// model.CallTreeTruncated leaf of the parent. Otherwise, the tree is that
// of buildCallTree.
//
// Large trees are mapped concurrently: location subtrees are split by the
// outermost function of their topmost location, so the call trees built by
// the workers have no top-level functions in common, and are concatenated.
//
// The function reports false if the stack traces are not backed by a single
// parent pointer tree.
func buildLocationCallTree(symbols *Symbols, appender *SampleAppender, maxNodes int64, truncate bool) (*model.CallTree, bool) {
	iterator, ok := symbols.Stacktraces.(StacktraceIDRangeIterator)
	if !ok || appender.Len() == 0 {
		return nil, false
	}
	it := iterator.SplitStacktraceIDRanges(appender)
	if !it.Next() {
		return nil, false
	}
	r := it.At()
	ppt, ok := r.ParentPointerTree.(*parentPointerTree)
	if !ok || it.Next() {
		return nil, false
	}
	nodes := ppt.nodes
	n := len(nodes)
	if n < 2 || nodes[0].p >= 0 {
		return nil, false
	}

	// Sample values, and totals of location subtrees. As in
	// propagateNodeValues, totals do not propagate to the root.
	self := make([]int64, n)
	for i, sid := range r.IDs {
		if v := int64(r.Values[i]); sid > 0 && int(sid) < n && v > 0 {
			self[sid] = v
		}
	}
	total := make([]int64, n)
	copy(total, self)
	for i := n - 1; i >= 1; i-- {
		if p := nodes[i].p; p > 0 {
			total[p] += total[i]
		}
	}
	var minValue int64
	if m := maxNodes * 4; truncate && m >= 1 && m < int64(n) {
		minValue = model.KthLargest(slices.Clone(total), int(m))
	}

	m := locationCallTreeMapper{
		symbols:   symbols,
		nodes:     nodes,
		self:      self,
		total:     total,
		minValue:  minValue,
		fn:        make([]int32, n),
		truncated: make([]bool, n),
	}
	workers := 1
	if n >= parallelCallTreeMinNodes {
		workers = min(runtime.GOMAXPROCS(0), 8)
	}
	owner, workers := m.split(workers)
	trees := make([]*functionTree, workers)
	rootTruncated := make([]int64, workers)
	var wg sync.WaitGroup
	for w := range trees {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			trees[w], rootTruncated[w] = m.mapNodes(owner, uint8(w), n/workers)
		}(w)
	}
	wg.Wait()
	return concatCallTrees(trees, rootTruncated, symbols.Strings), true
}

type locationCallTreeMapper struct {
	symbols   *Symbols
	nodes     []pptNode
	self      []int64
	total     []int64
	minValue  int64
	fn        []int32 // Call tree node of the location, in the worker's tree.
	truncated []bool
}

// split assigns location nodes to workers by the outermost function of
// their topmost location, balancing the number of nodes. It returns nil if
// all the nodes are assigned to a single worker.
func (m *locationCallTreeMapper) split(workers int) ([]uint8, int) {
	if workers < 2 {
		return nil, 1
	}
	n := len(m.nodes)
	top := make([]int32, n)
	size := make([]int32, n)
	for i := 1; i < n; i++ {
		if p := m.nodes[i].p; p == 0 {
			top[i] = int32(i)
		} else {
			top[i] = top[p]
		}
		size[top[i]]++
	}
	// Topmost locations without lines put their children at the top
	// level of the call tree: keep it simple and map them in one go.
	groups := make(map[uint32]int)
	for i := 1; i < n; i++ {
		if m.nodes[i].p != 0 {
			continue
		}
		lines := m.symbols.Locations[m.nodes[i].r].Line
		if len(lines) == 0 {
			return nil, 1
		}
		groups[m.symbols.Functions[lines[len(lines)-1].FunctionId].Name] += int(size[i])
	}
	names := make([]uint32, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	slices.SortFunc(names, func(a, b uint32) int {
		if d := groups[b] - groups[a]; d != 0 {
			return d
		}
		return int(a) - int(b)
	})
	load := make([]int, workers)
	worker := make(map[uint32]uint8, len(names))
	for _, name := range names {
		w := slices.Index(load, slices.Min(load))
		worker[name] = uint8(w)
		load[w] += groups[name]
	}
	owner := make([]uint8, n)
	for i := 1; i < n; i++ {
		if p := m.nodes[i].p; p == 0 {
			lines := m.symbols.Locations[m.nodes[i].r].Line
			owner[i] = worker[m.symbols.Functions[lines[len(lines)-1].FunctionId].Name]
		} else {
			owner[i] = owner[p]
		}
	}
	return owner, workers
}

// mapNodes maps the nodes owned by the worker. The weight of topmost
// locations that are truncated is returned separately: workers would
// otherwise add truncated leaves to the root independently.
func (m *locationCallTreeMapper) mapNodes(owner []uint8, w uint8, sizeHint int) (*functionTree, int64) {
	t := newFunctionTree(sizeHint)
	var rootTruncated int64
	for i := 1; i < len(m.nodes); i++ {
		if owner != nil && owner[i] != w {
			continue
		}
		p := m.nodes[i].p
		if m.total[i] < m.minValue {
			m.truncated[i] = true
			if !m.truncated[p] && m.total[i] > 0 {
				// The topmost truncated location keeps the weight of
				// its subtree.
				if p == 0 {
					rootTruncated += m.total[i]
				} else {
					x := t.child(m.fn[p], model.CallTreeTruncated)
					t.self[x] += m.total[i]
				}
			}
			continue
		}
		if m.total[i] == 0 {
			// The location is not sampled.
			continue
		}
		f := m.fn[p]
		lines := m.symbols.Locations[m.nodes[i].r].Line
		for l := len(lines) - 1; l >= 0; l-- {
			f = t.child(f, int32(m.symbols.Functions[lines[l].FunctionId].Name))
		}
		m.fn[i] = f
		t.self[f] += m.self[i]
	}
	return t, rootTruncated
}

// concatCallTrees concatenates call trees whose root children are distinct.
func concatCallTrees(trees []*functionTree, rootTruncated []int64, names []string) *model.CallTree {
	n := 1
	var truncated int64
	for i, t := range trees {
		n += len(t.parent) - 1
		truncated += rootTruncated[i]
	}
	if truncated > 0 {
		n++
	}
	c := &model.CallTree{
		Parent: make([]int32, 1, n),
		Name:   make([]int32, 1, n),
		Self:   make([]int64, 1, n),
		Names:  names,
	}
	c.Parent[0] = sentinel
	for _, t := range trees {
		base := int32(len(c.Parent)) - 1
		for j := 1; j < len(t.parent); j++ {
			p := t.parent[j]
			if p > 0 {
				p += base
			}
			c.Parent = append(c.Parent, p)
		}
		c.Name = append(c.Name, t.name[1:]...)
		c.Self = append(c.Self, t.self[1:]...)
		c.Self[0] += t.self[0]
	}
	if truncated > 0 {
		c.Parent = append(c.Parent, 0)
		c.Name = append(c.Name, model.CallTreeTruncated)
		c.Self = append(c.Self, truncated)
	}
	c.Total = slices.Clone(c.Self)
	for i := len(c.Total) - 1; i > 0; i-- {
		c.Total[c.Parent[i]] += c.Total[i]
	}
	c.Total[0] = 0
	return c
}

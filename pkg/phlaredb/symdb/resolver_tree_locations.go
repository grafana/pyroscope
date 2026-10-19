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

	b := getCallTreeBuffers()
	defer putCallTreeBuffers(b)

	// Sample values, and totals of location subtrees. As in
	// propagateNodeValues, totals do not propagate to the root.
	b.self = zeroed(b.self, n)
	for i, sid := range r.IDs {
		if v := int64(r.Values[i]); sid > 0 && int(sid) < n && v > 0 {
			b.self[sid] = v
		}
	}
	b.total = zeroed(b.total, n)
	copy(b.total, b.self)
	for i := n - 1; i >= 1; i-- {
		if p := nodes[i].p; p > 0 {
			b.total[p] += b.total[i]
		}
	}
	var minValue int64
	if m := maxNodes * 4; truncate && m >= 1 && m < int64(n) {
		b.scratch = zeroed(b.scratch, n)
		copy(b.scratch, b.total)
		minValue = model.KthLargest(b.scratch, int(m))
	}

	m := locationCallTreeMapper{
		symbols:  symbols,
		nodes:    nodes,
		self:     b.self,
		total:    b.total,
		minValue: minValue,
	}
	b.fn = zeroed(b.fn, n)
	m.fn = b.fn
	workers := 1
	if n >= parallelCallTreeMinNodes {
		workers = min(runtime.GOMAXPROCS(0), 8)
	}
	order, bounds := m.split(b, workers)
	workers = len(bounds) - 1
	trees := make([]*functionTree, workers)
	rootTruncated := make([]int64, workers)
	var wg sync.WaitGroup
	for w := range trees {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			var own []int32
			if order != nil {
				own = order[bounds[w]:bounds[w+1]]
			}
			trees[w], rootTruncated[w] = m.mapNodes(own, n/workers)
		}(w)
	}
	wg.Wait()
	return concatCallTrees(trees, rootTruncated, symbols.Strings), true
}

// truncatedLocation marks locations dropped by truncation in fn.
const truncatedLocation = -1

type locationCallTreeMapper struct {
	symbols  *Symbols
	nodes    []pptNode
	self     []int64
	total    []int64
	minValue int64
	// Call tree node of the location, in the tree of the worker
	// the location is assigned to, or truncatedLocation.
	fn []int32
}

// split assigns location nodes to workers by the outermost function of
// their topmost location, balancing the number of nodes. Nodes of worker w
// are order[bounds[w]:bounds[w+1]], in ascending order. If order is nil,
// a single worker maps all the nodes.
func (m *locationCallTreeMapper) split(b *callTreeBuffers, workers int) (order []int32, bounds []int) {
	if workers < 2 {
		return nil, []int{0, 0}
	}
	n := len(m.nodes)
	// Size of the subtree of each topmost location.
	b.top = zeroed(b.top, n)
	b.size = zeroed(b.size, n)
	top, size := b.top, b.size
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
			return nil, []int{0, 0}
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
	worker := make(map[uint32]int32, len(names))
	for _, name := range names {
		w := slices.Index(load, slices.Min(load))
		worker[name] = int32(w)
		load[w] += groups[name]
	}
	// The worker of a topmost location replaces its size: the worker of
	// node i is size[top[i]]. Nodes are then bucketed by worker.
	for i := 1; i < n; i++ {
		if m.nodes[i].p == 0 {
			lines := m.symbols.Locations[m.nodes[i].r].Line
			size[i] = worker[m.symbols.Functions[lines[len(lines)-1].FunctionId].Name]
		}
	}
	bounds = make([]int, workers+1)
	for i := 1; i < n; i++ {
		bounds[size[top[i]]+1]++
	}
	for w := 1; w <= workers; w++ {
		bounds[w] += bounds[w-1]
	}
	b.order = zeroed(b.order, n-1)
	order = b.order
	pos := slices.Clone(bounds[:workers])
	for i := 1; i < n; i++ {
		w := size[top[i]]
		order[pos[w]] = int32(i)
		pos[w]++
	}
	return order, bounds
}

// mapNodes maps the given nodes, or all of them if nodes is nil. The weight
// of topmost locations that are truncated is returned separately: workers
// would otherwise add truncated leaves to the root independently.
func (m *locationCallTreeMapper) mapNodes(nodes []int32, sizeHint int) (*functionTree, int64) {
	t := newFunctionTree(sizeHint)
	var rootTruncated int64
	visit := func(i int32) {
		p := m.nodes[i].p
		if m.total[i] < m.minValue {
			if m.fn[p] != truncatedLocation && m.total[i] > 0 {
				// The topmost truncated location keeps the weight of
				// its subtree.
				if p == 0 {
					rootTruncated += m.total[i]
				} else {
					x := t.child(m.fn[p], model.CallTreeTruncated)
					t.self[x] += m.total[i]
				}
			}
			m.fn[i] = truncatedLocation
			return
		}
		if m.total[i] == 0 {
			// The location is not sampled.
			return
		}
		f := m.fn[p]
		lines := m.symbols.Locations[m.nodes[i].r].Line
		for l := len(lines) - 1; l >= 0; l-- {
			f = t.child(f, int32(m.symbols.Functions[lines[l].FunctionId].Name))
		}
		m.fn[i] = f
		t.self[f] += m.self[i]
	}
	if nodes == nil {
		for i := 1; i < len(m.nodes); i++ {
			visit(int32(i))
		}
	} else {
		for _, i := range nodes {
			visit(i)
		}
	}
	return t, rootTruncated
}

// callTreeBuffers are the transient buffers of buildLocationCallTree, sized
// by the number of locations of a block: they are reused across calls.
type callTreeBuffers struct {
	self, total, scratch []int64
	fn, top, size, order []int32
}

var callTreeBuffersPool sync.Pool

func getCallTreeBuffers() *callTreeBuffers {
	if b, ok := callTreeBuffersPool.Get().(*callTreeBuffers); ok {
		return b
	}
	return new(callTreeBuffers)
}

func putCallTreeBuffers(b *callTreeBuffers) { callTreeBuffersPool.Put(b) }

// zeroed returns a zeroed slice of length n, reusing the capacity of s.
func zeroed[T any](s []T, n int) []T {
	if cap(s) < n {
		return make([]T, n)
	}
	s = s[:n]
	clear(s)
	return s
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

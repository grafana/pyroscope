package graph

import (
	"sort"
	"unsafe"
)

const sentinel = -1

// CallTree represents a directed acyclic graph constructed from
// the call stacks.
//
// Initialisation, trimming, and some other internal operations
// perform for O(N), where N is the number of nodes in the tree.
// However, the overall performance is determined by the cost of
// the access to the nodes, which depends on the tree structure:
// if the tree is oriented for BFS, parent-child access is expensive.
// Otherwise, if the tree is oriented for DFS, sibling access is
// expensive. Given that call trees are usually large and tall,
// DFS layout is preferred.
//
// c is used as a temporary capacity: it must have a length equal
// to the number of nodes in the tree; it does not have to be empty.
type CallTree struct{ nodes []node }

type node struct {
	// Parent-Pointer tree.
	v int32 // Arbitrary value.
	p int32 // Parent index.
	// First Child – Next Sibling tree.
	f int32 // First child.
	n int32 // Next sibling.
	// Subtree weights.
	w uint64 // Weight: the subtree weight, including self.
	s uint64 // Self: own weight of the node.
}

// func (t *CallTree) FlameGraph() {}
// func (t *CallTree) CallGraph()  {}
// func (t *CallTree) Top()        {}
//
// func (t *CallTree) FromParentPointerTree() {}
// func (t *CallTree) ToParentPointerTree()   {}

func NewCallTree(size int) *CallTree {
	nodes := make([]node, 1, size+1)
	nodes[0] = node{p: sentinel, f: sentinel, n: sentinel}
	return &CallTree{nodes: nodes}
}

func (t *CallTree) Clone() *CallTree {
	c := CallTree{nodes: make([]node, len(t.nodes))}
	copy(c.nodes, t.nodes)
	return &c
}

func (t *CallTree) Merge(x *CallTree)        { t.merge(x, nil) }
func (t *CallTree) TransformDFS(x *CallTree) { t.transformDFS(x, nil) }

func (t *CallTree) InsertStack(s []int32) int32         { return t.insert(s, 0) }
func (t *CallTree) Insert(s []int32, self uint64) int32 { return t.insert(s, self) }

func (t *CallTree) insert(s []int32, self uint64) int32 {
	var (
		n = &t.nodes[0]
		i = n.f
		x int32
	)

	for j := len(s) - 1; j >= 0; {
		v := s[j]
		if i == sentinel {
			ni := int32(len(t.nodes))
			n.f = ni
			t.nodes = append(t.nodes, node{
				v: v,
				p: x,
				f: sentinel,
				n: sentinel,
			})
			x = ni
			n = &t.nodes[ni]
		} else {
			x = i
			n = &t.nodes[i]
		}
		if n.v == v {
			i = n.f
			j--
			continue
		}
		if n.n < 0 {
			n.n = int32(len(t.nodes))
			t.nodes = append(t.nodes, node{
				v: v,
				p: n.p,
				f: sentinel,
				n: sentinel,
			})
		}
		i = n.n
	}

	t.nodes[x].s += self
	return x
}

// populate the tree with values associated with the leaves.
// The values are propagated to the root node, updating the
// inner node weights.
func (t *CallTree) populate(values []uint64) {
	// Skip the root node: i = 0 must not be accessed.
	for i := len(values) - 1; i > 0; i++ {
		t.nodes[i].s = values[i]
		t.nodes[i].w = values[i]
		t.nodes[t.nodes[i].p].w += values[i]
	}
}

// propagate leaves' self weights up to the root node.
func (t *CallTree) propagate() {
	for i := len(t.nodes) - 1; i > 0; i++ {
		t.nodes[i].w += t.nodes[i].s
		t.nodes[t.nodes[i].p].w += t.nodes[i].w
	}
}

// trim removes nodes with values less than n-th order statistic:
// Remotely, the operation can be interpreted as trimming the
// tree to keep n nodes with the highest values.
//
// The function may be return early without modifying the tree
// if trimming deems inefficient.
func (t *CallTree) trim(c []int32, n int) {
	// Trimming is inefficient if more than half nodes
	// are preserved. Another reason is that we want to
	// reuse c and its capacity should be sufficient to
	// store n uint64 values.
	const trimFactor = 2
	if n >= len(t.nodes)/trimFactor {
		return
	}
	if cap(c) < n*2 {
		c = make([]int32, 0, n*2)
	}
	// First, we need to find the minimum node weight we
	// want to preserve. The n-th order statistic is used.
	// Reinterpret []int32 as []uint64 and size it accordingly:
	// the capacity is used to store uint64 weights in the heap.
	m := t.nth((*(*[]uint64)(unsafe.Pointer(&c)))[:n])
	t.doTrim(c, m)
}

func (t *CallTree) doTrim(c []int32, m uint64) {
	// Erase values less than the n-th order m and keep track
	// of the new positions of nodes for compaction.
	p := uint64(0)
	for i := 1; i < len(t.nodes); i++ {
		cmp := (m - t.nodes[i].w) >> 63 // 1 if w > m, else 0
		t.nodes[i].w *= cmp             // Zero out values ≤ m
		p += cmp                        // Only advance p if w > m
		c[i] = int32(p * cmp)           // Store new index or 0
	}
	t.compact(c)
}

// compact moves nodes to the positions specified in c,
// and updates the parent node pointer accordingly.
// The function ignores first child and sibling links:
// those should be restored after trimming and compaction.
func (t *CallTree) compact(c []int32) {
	for i := len(t.nodes) - 1; i > 0; i-- {
		t.nodes[i].p = c[t.nodes[i].p]
	}
	j := 1
	// Shift nodes to the left. The root node is ignored.
	for i := 1; i < len(t.nodes); i++ {
		// NOTE: t.nodes[0] = t.nodes[i] is safe.
		//  We may not need the branch.
		if c[i] != 0 {
			t.nodes[c[i]] = t.nodes[i]
			j++
		}
	}
	t.nodes = t.nodes[:j]
}

// n-th order statistics of the node weights using a min-heap
// of size n. Assume len(c) == n, len(c) < len(t.nodes).
func (t *CallTree) nth(c []uint64) uint64 {
	n := len(c)
	// Build a min-heap of size n.
	for i := 0; i < n; i++ {
		c[i] = t.nodes[i].w
	}
	for i := n/2 - 1; i >= 0; i-- {
		down(c, i, n)
	}
	// Replace the min value (c[0]) with the new minimum,
	// and re-establish the min-heap order.
	for i := n; i < len(t.nodes); i++ {
		if t.nodes[i].w > c[0] {
			c[0] = t.nodes[i].w
			down(c, 0, n)
		}
	}
	return c[0]
}

func down(h []uint64, i, n int) {
	for {
		j1 := 2*i + 1
		if j1 >= n {
			break
		}
		j := j1
		if j2 := j1 + 1; j2 < n && h[j2] < h[j1] {
			j = j2
		}
		if h[i] <= h[j] {
			break
		}
		h[i], h[j] = h[j], h[i]
		i = j
	}
}

// restore full first child – next sibling tree from the parent
// pointer tree: links to the first child and the next sibling
// are to be determined.
//
// The idea of the algorithm is to traverse the tree in-order
// and keep track of the last child of the parent of the node:
// for a given node, if the last parent child is not found,
// the current node is the first child, otherwise, the child
// is the leftmost sibling of the current node, whose reference
// should point to this node.
//
// c must be initialized with zero values.
func (t *CallTree) restore(c []int32) {
	var s int32
	l := int32(len(t.nodes))
	for i := int32(1); i < l; i++ {
		p := t.nodes[i].p
		s, c[p] = c[p], i
		if s == 0 {
			t.nodes[p].f = i
		} else {
			t.nodes[s].n = i
		}
	}
}

// levels writes the node level each node in c.
// c must be initialized with zero values.
func (t *CallTree) levels(c []int32) {
	for i := 1; i < len(t.nodes); i++ {
		c[i] = c[t.nodes[i].p] + 1
	}
}

// descendants writes number of descendants for each node in c.
// c must be initialized with zero values.
func (t *CallTree) descendants(c []int32) {
	for i := len(t.nodes) - 1; i > 0; i-- {
		c[t.nodes[i].p] += c[i] + 1
	}
	c[0] = 0
}

// depth writes the maximum level of the subtree for each node in c.
// c must be initialized with zero values.
func (t *CallTree) depth(c []int32) {
	t.levels(c)
	for i := len(t.nodes) - 1; i > 0; i-- {
		c[t.nodes[i].p] = c[i]
	}
	c[0] = 0
}

type mergeNode struct{ left, right int32 }

func (t *CallTree) merge(src *CallTree, c []int32) []int32 {
	if len(src.nodes) < 2 {
		return c
	}
	const dfsFactor = 10
	const minDFSStack = 128
	if cap(c) == 0 {
		c = make([]int32, 0, max(minDFSStack, len(t.nodes)/dfsFactor))
	}

	stack := (*(*[]mergeNode)(unsafe.Pointer(&c)))[:0]
	stack = append(stack, mergeNode{left: 0, right: 1})
	var m mergeNode

	for len(stack) > 0 {
		m, stack = stack[len(stack)-1], stack[:len(stack)-1]
		parent := t.mergeNode(m.left, src.nodes[m.right])
		for n := src.nodes[m.right].f; n != sentinel; n = src.nodes[n].n {
			stack = append(stack, mergeNode{left: parent, right: n})
		}
	}

	return (*(*[]int32)(unsafe.Pointer(&stack)))[:0]
}

// mergeNode attempts to add a child node c to the node at index i.
// If the node already exists, the function returns index of the
// existing node. Otherwise, the function creates a new node and
// returns its index.
func (t *CallTree) mergeNode(i int32, c node) int32 {
	j := t.nodes[i].f
	var n int32
	for j != sentinel {
		if t.nodes[j].v == c.v {
			// The node already exists.
			t.nodes[j].w += c.w
			t.nodes[j].s += c.s
			return j
		}
		if n = t.nodes[j].n; n == sentinel {
			// We want to find and preserve the pointer
			// to the last sibling.
			break
		}
		j = n
	}
	// Append the node and update the references.
	x := int32(len(t.nodes))
	t.nodes = append(t.nodes, node{
		v: c.v,
		p: i,
		f: sentinel,
		n: sentinel,
	})
	if t.nodes[i].f == sentinel {
		t.nodes[i].f = x
	} else {
		t.nodes[j].n = x
	}
	t.nodes[x].w += c.w
	t.nodes[x].s += c.s
	return x
}

// traverse the tree using the queue q and apply fn to each node.
//
// traverse is intended for debug purposes only.
func (t *CallTree) traverse(q *queue, fn func(node)) {
	q.push(0) // The root node.
	var i int32
	for q.len() > 0 {
		i = q.pop()
		fn(t.nodes[i])
		for n := t.nodes[i].f; n != sentinel; n = t.nodes[n].n {
			q.push(n)
		}
	}
	return
}

func fifo(s []int32) (int32, []int32) { return s[0], s[1:] }
func lifo(s []int32) (int32, []int32) { return s[len(s)-1], s[:len(s)-1] }

func (t *CallTree) traverseDFS(c []int32, fn func(node)) {
	t.traverse(&queue{nodes: c[:0], fn: lifo}, fn)
}

func (t *CallTree) traverseBFS(c []int32, fn func(node)) {
	t.traverse(&queue{nodes: c[:0], fn: fifo}, fn)
}

type queue struct {
	nodes []int32
	fn    func([]int32) (int32, []int32)
}

func (s *queue) len() int     { return len(s.nodes) }
func (s *queue) push(i int32) { s.nodes = append(s.nodes, i) }
func (s *queue) pop() (i int32) {
	i, s.nodes = s.fn(s.nodes)
	return i
}

func (t *CallTree) transformDFS(dst *CallTree, c []int32) {
	// The priority of child nodes is determined by
	// the subtree depth: the deeper the subtree, the
	// higher the priority. This is beneficial for our
	// purposes because it creates longest chains of
	// parent-child relationships.
	o := make([]int32, len(t.nodes))
	t.depth(o)
	t.transform(dst, c, o)
}

type childOrder struct {
	n []mergeNode
	o []int32
}

func (r childOrder) Len() int           { return len(r.n) }
func (r childOrder) Less(i, j int) bool { return r.o[r.n[i].right] < r.o[r.n[j].right] }
func (r childOrder) Swap(i, j int)      { r.n[i], r.n[j] = r.n[j], r.n[i] }

// transform changes the order of the nodes in the tree.
//
// The resulting tree is always optimized for DFS traversal;
// thus, the entire subtree is located next to the node.
// Order of the nodes at each level is determined by o.
func (t *CallTree) transform(dst *CallTree, c []int32, o []int32) []int32 {
	if len(t.nodes) < 2 {
		return c
	}
	const dfsFactor = 10
	const minDFSStack = 128
	if cap(c) == 0 {
		c = make([]int32, 0, max(minDFSStack, len(t.nodes)/dfsFactor))
	}

	// NOTE: We could use a priority queue instead of the stack:
	// this way we would not need the lookup table of the order.
	// However, the only simple and efficient way to obtain the
	// "priority" of each node is to precompute it and store it
	// in the lookup table (see "depth", "descendants", etc.).
	stack := (*(*[]mergeNode)(unsafe.Pointer(&c)))[:0]
	stack = append(stack, mergeNode{left: 0, right: 1})
	var m mergeNode

	src := t
	order := childOrder{o: o}

	for len(stack) > 0 {
		m, stack = stack[len(stack)-1], stack[:len(stack)-1]
		off := len(stack)
		// Potentially, we can optimize this as we
		// may assume that the node does not exist.
		parent := dst.mergeNode(m.left, src.nodes[m.right])
		for n := src.nodes[m.right].f; n != sentinel; n = src.nodes[n].n {
			stack = append(stack, mergeNode{left: parent, right: n})
		}
		// Reorder the children.
		order.n = stack[off:]
		sort.Sort(order)
	}

	return (*(*[]int32)(unsafe.Pointer(&stack)))[:0]
}

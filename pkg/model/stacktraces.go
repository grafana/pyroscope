package model

import (
	"bytes"
	"io"
	"sync"
	"unsafe"

	ingestv1 "github.com/grafana/pyroscope/api/gen/proto/go/ingester/v1"
	"github.com/grafana/pyroscope/pkg/og/util/varint"
	"github.com/grafana/pyroscope/pkg/util/minheap"
)

// TODO(kolesnikovae): Remove support for StacktracesMergeFormat_MERGE_FORMAT_STACKTRACES.

type StacktraceMerger struct {
	mu sync.Mutex
	s  *StacktraceTree
	r  *functionsRewriter
}

// NewStackTraceMerger merges collections of StacktraceSamples.
// The result is a byte tree representation of the merged samples.
func NewStackTraceMerger() *StacktraceMerger {
	return new(StacktraceMerger)
}

// MergeStackTraces adds the stack traces to the resulting tree.
// The call is thread-safe, but the resulting tree bytes should
// be only build after all the samples are merged.
// Note that the function may reuse the capacity of the names slice.
func (m *StacktraceMerger) MergeStackTraces(stacks []*ingestv1.StacktraceSample, names []string) {
	m.mu.Lock()
	if m.s == nil {
		// Estimate resulting tree size: it's likely that the first
		// batch is small, therefore we can safely assume the tree
		// will grow (factor of 2 is quite conservative).
		// 4 here is the branching factor: how many new nodes per
		// a stack on average we expect.
		m.s = NewStacktraceTree(len(stacks) * 4 * 2)
		// We can use function IDs as is for the first batch.
		m.r = newFunctionsRewriter(names)
		for _, s := range stacks {
			m.s.Insert(s.FunctionIds, s.Value)
		}
		m.mu.Unlock()
		return
	}
	m.r.union(names)
	for _, s := range stacks {
		m.r.rewrite(s.FunctionIds)
		m.s.Insert(s.FunctionIds, s.Value)
	}
	m.mu.Unlock()
}

func (m *StacktraceMerger) TreeBytes(maxNodes int64) []byte {
	if m.s == nil || len(m.s.Nodes) == 0 {
		return nil
	}
	// Reuse of the slice (or the whole StacktraceMerger) is possible,
	// but it is unlikely that the performance will be impacted.
	size := len(m.s.Nodes)
	if mn := int(maxNodes); maxNodes > 0 && mn < size {
		size = mn
	}
	buf := make([]byte, 0, size*estimateBytesPerNode)
	b := bytes.NewBuffer(buf)
	m.s.Bytes(b, maxNodes, m.r.names)
	return b.Bytes()
}

func (m *StacktraceMerger) Size() int {
	if m.s != nil {
		return len(m.s.Nodes)
	}
	return 0
}

func newFunctionsRewriter(names []string) *functionsRewriter {
	p := make(map[string]int, 2*len(names))
	for i, v := range names {
		p[v] = i
	}
	return &functionsRewriter{
		positions: p,
		names:     names,
	}
}

type functionsRewriter struct {
	positions map[string]int
	names     []string
	tmp       []int32
}

func (r *functionsRewriter) union(names []string) {
	if cap(r.tmp) > len(names) {
		r.tmp = r.tmp[:len(names)]
	} else {
		r.tmp = make([]int32, len(names))
	}
	for i, name := range names {
		position, found := r.positions[name]
		if !found {
			position = len(r.names)
			r.names = append(r.names, name)
			r.positions[name] = position
		}
		r.tmp[i] = int32(position)
	}
}

func (r *functionsRewriter) rewrite(stack []int32) {
	for i := range stack {
		stack[i] = r.tmp[stack[i]]
	}
}

type StacktraceTree struct{ Nodes []StacktraceNode }

type StacktraceNode struct {
	FirstChild  int32
	NextSibling int32
	Parent      int32
	Location    int32
	Value       int64
	Total       int64
}

func NewStacktraceTree(size int) *StacktraceTree {
	if size < 1 {
		size = 1
	}
	t := StacktraceTree{Nodes: make([]StacktraceNode, 1, size)}
	t.Nodes[0] = StacktraceNode{
		FirstChild:  sentinel,
		NextSibling: sentinel,
	}
	return &t
}

func (t *StacktraceTree) Reset() {
	if cap(t.Nodes) < 1 {
		*t = *(NewStacktraceTree(0))
		return
	}
	t.Nodes = t.Nodes[:1]
	t.Nodes[0] = StacktraceNode{
		FirstChild:  sentinel,
		NextSibling: sentinel,
	}
}

const sentinel = -1

func (t *StacktraceTree) Insert(locations []int32, value int64) int32 {
	var (
		n    = &t.Nodes[0]
		next = n.FirstChild
		cur  int32
	)

	for j := len(locations) - 1; j >= 0; {
		r := locations[j]
		if next == sentinel {
			ni := int32(len(t.Nodes))
			n.FirstChild = ni
			t.Nodes = append(t.Nodes, StacktraceNode{
				Parent:      cur,
				FirstChild:  sentinel,
				NextSibling: sentinel,
				Location:    r,
			})
			cur = ni
			n = &t.Nodes[ni]
		} else {
			cur = next
			n = &t.Nodes[next]
		}
		if n.Location == r {
			n.Total += value
			next = n.FirstChild
			j--
			continue
		}
		if n.NextSibling < 0 {
			n.NextSibling = int32(len(t.Nodes))
			t.Nodes = append(t.Nodes, StacktraceNode{
				Parent:      n.Parent,
				FirstChild:  sentinel,
				NextSibling: sentinel,
				Location:    r,
			})
		}
		next = n.NextSibling
	}

	t.Nodes[cur].Value += value
	return cur
}

func (t *StacktraceTree) LookupLocations(dst []uint64, idx int32) []uint64 {
	dst = dst[:0]
	if idx >= int32(len(t.Nodes)) {
		return dst
	}
	for i := idx; i > 0; i = t.Nodes[i].Parent {
		dst = append(dst, uint64(t.Nodes[i].Location))
	}
	return dst
}

// MinValue returns the minimum "total" value a node in a tree has to have.
func (t *StacktraceTree) MinValue(maxNodes int64) int64 {
	if maxNodes < 1 || maxNodes >= int64(len(t.Nodes)) {
		return 0
	}
	h := make([]int64, 0, maxNodes)
	for _, n := range t.Nodes {
		if len(h) >= int(maxNodes) {
			if n.Total > h[0] {
				h = minheap.Pop(h)
			} else {
				continue
			}
		}
		h = minheap.Push(h, n.Total)
	}
	if len(h) < int(maxNodes) {
		return 0
	}
	return h[0]
}

type StacktraceTreeTraverseFn = func(index int32, children []int32) error

func (t *StacktraceTree) Traverse(maxNodes int64, fn StacktraceTreeTraverseFn) error {
	minValue := t.MinValue(maxNodes)
	children := make([]int32, 0, 128) // Children per node.
	nodesSize := maxNodes             // Depth search buffer.
	if nodesSize < 1 || nodesSize > 10<<10 {
		nodesSize = 1 << 10 // Sane default.
	}
	nodes := make([]int32, 1, nodesSize)
	var current int32
	for len(nodes) > 0 {
		current, nodes, children = nodes[len(nodes)-1], nodes[:len(nodes)-1], children[:0]
		var truncated int64
		n := &t.Nodes[current]
		if n.Location == sentinel {
			goto call
		}

		for x := n.FirstChild; x > 0; {
			child := &t.Nodes[x]
			if child.Total >= minValue && child.Location != sentinel {
				children = append(children, x)
			} else {
				truncated += child.Total
			}
			x = child.NextSibling
		}

		if truncated > 0 {
			// Create a stub for removed nodes.
			i := len(t.Nodes)
			t.Nodes = append(t.Nodes, StacktraceNode{
				Location: sentinel,
				Value:    truncated,
			})
			children = append(children, int32(i))
		}

		if len(children) > 0 {
			nodes = append(nodes, children...)
		}

	call:
		if err := fn(current, children); err != nil {
			return err
		}
	}

	return nil
}

func (t *StacktraceTree) Bytes(dst io.Writer, maxNodes int64, funcs []string) {
	if len(t.Nodes) == 0 || len(funcs) == 0 {
		return
	}
	vw := varint.NewWriter()
	_ = t.Traverse(maxNodes, func(index int32, children []int32) error {
		n := t.Nodes[index]
		var name []byte
		switch n.Location {
		default:
			// It is guaranteed that funcs slice and its contents are immutable,
			// and the byte slice backing capacity is managed by GC.
			name = unsafeStringBytes(funcs[n.Location])
		case sentinel:
			name = truncatedNodeNameBytes
		}
		_, _ = vw.Write(dst, uint64(len(name)))
		_, _ = dst.Write(name)
		_, _ = vw.Write(dst, uint64(n.Value))
		_, _ = vw.Write(dst, uint64(len(children)))
		return nil
	})
}

func unsafeStringBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

func (t *StacktraceTree) Tree(maxNodes int64, names []string) *Tree {
	if len(t.Nodes) < 2 || len(names) == 0 {
		// stack trace tree has root at 0: trees with less
		// than 2 nodes are considered empty.
		return new(Tree)
	}

	nodesSize := maxNodes
	if nodesSize < 1 || nodesSize > 10<<10 {
		nodesSize = 1 << 10 // Sane default.
	}
	root := new(node) // Virtual root node.
	nodes := make([]*node, 1, nodesSize)
	nodes[0] = root
	var current *node

	_ = t.Traverse(maxNodes, func(index int32, children []int32) error {
		current, nodes = nodes[len(nodes)-1], nodes[:len(nodes)-1]
		sn := &t.Nodes[index]
		var name string
		if sn.Location < 0 {
			name = truncatedNodeName
			sn.Total = sn.Value
		} else {
			name = names[sn.Location]
		}
		n := current.insert(name)
		n.self = sn.Value
		n.total = sn.Total
		n.children = make([]*node, 0, len(children))
		for i := 0; i < len(children); i++ {
			nodes = append(nodes, n)
		}
		return nil
	})

	// Roots should not have parents.
	s := root.children[0].children
	for _, n := range s {
		n.parent.parent = nil
	}

	return &Tree{root: s}
}

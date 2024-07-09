package queryplan

import (
	"math"
	"unsafe"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	"github.com/grafana/pyroscope/pkg/iter"
)

// QueryPlan is a query physical plan. The plan is represented by a DAG, where
// each node might be either a "merge" or a "read" (leaves). A node references
// a range: merge nodes refer to others, while read nodes refer to the blocks.
type QueryPlan struct {
	nodes  []node
	blocks []*metastorev1.BlockMeta
}

type Node struct {
	Type NodeType

	p *QueryPlan
	n node
}

type NodeType uint32

const (
	_ NodeType = iota
	NodeRead
	NodeMerge
)

var typeNames = [...]string{"invalid", "read", "merge"}

func (t NodeType) String() (s string) {
	if int(t) >= len(typeNames) {
		return typeNames[0]
	}
	return typeNames[t]
}

type node struct {
	typ NodeType
	// Node of merge type refers to nodes.
	// Node of read type refers to blocks.
	off uint32
	len uint32
}

func Open(p *querybackendv1.QueryPlan) *QueryPlan {
	if len(p.Graph) == 0 || len(p.Blocks) == 0 || len(p.Graph)%3 > 0 {
		return new(QueryPlan)
	}
	return &QueryPlan{
		nodes:  *(*[]node)(unsafe.Pointer(&p.Graph)),
		blocks: p.Blocks,
	}
}

// Build creates a query plan from the list of block metadata.
//
// NOTE(kolesnikovae): At this point it only groups blocks into uniform ranges,
// and builds a DAG of reads and merges. In practice, however, we may want to
// implement more sophisticated strategies. For example, it would be beneficial
// to group blocks based on the tenant services to ensure that a single read
// covers exactly one service, and does not have to deal with stack trace
// cardinality issues. Another example is grouping by shards to minimize the
// number of unique series (assuming the shards are still built based on the
// series labels) a reader or merger should handle. In general, the strategy
// should depend on the query type.
func Build(
	blocks []*metastorev1.BlockMeta,
	maxReads, maxMerges int64,
) *QueryPlan {
	if len(blocks) == 0 {
		return new(QueryPlan)
	}
	// First, we create leaves: the entire range of blocks
	// is split into smaller uniform ranges, which will be
	// fetched by workers.
	s := int(math.Max(float64(maxReads), float64(maxMerges)))
	ranges := uniformSplit(make([][2]uint32, s), len(blocks), maxReads)
	nodes := make([]node, len(ranges))
	for i, b := range ranges {
		nodes[i] = node{
			typ: NodeRead,
			// Block range.
			off: b[0],
			len: b[1],
		}
	}
	// Next we create merge nodes.
	var off int
	for {
		// Split should not be applied to the same (sub-)range
		// twice, therefore we keep track of the offset within
		// the nodes slice.
		length := len(nodes)
		ranges = uniformSplit(ranges, len(nodes)-off, maxMerges)
		for _, n := range ranges {
			// Range offset does not account for the offset within
			// the nodes slice, therefore we add it here.
			nodes = append(nodes, node{
				typ: NodeMerge,
				off: n[0] + uint32(off),
				len: n[1],
			})
		}
		if len(ranges) == 1 {
			// The root node has been added.
			break
		} else if len(ranges) == 0 {
			// Create a virtual root, that will be a parent of all the
			// top level nodes. We find the offset of child nodes based
			// on the last node: its children is the last range of nodes
			// that have a parent.
			n := nodes[len(nodes)-1]
			o := n.off + n.len
			l := uint32(len(nodes)) - o
			nodes = append(nodes, node{
				typ: NodeMerge,
				off: o,
				len: l,
			})
			break
		}
		off += length
	}
	return &QueryPlan{
		blocks: blocks,
		nodes:  nodes,
	}
}

func (p *QueryPlan) Root() *Node {
	if len(p.nodes) == 0 {
		// A stub node.
		return &Node{Type: NodeRead, p: p}
	}
	n := Node{p: p}
	x := len(p.nodes) - 1
	n.n, p.nodes = p.nodes[x], p.nodes[:x]
	n.Type = n.n.typ
	return &n
}

// Plan returns the query plan scoped to the node.
// The plan is nil, if the node is a leaf.
func (n *Node) Plan() *querybackendv1.QueryPlan {
	if n.n.typ == NodeRead {
		return new(querybackendv1.QueryPlan)
	}
	t := make([]node, 0, 32)
	traverseBFS(n.p, n.n, func(n node) {
		t = append(t, n)
	})
	// The node itself is not included into the plan,
	// as it considered executed.
	t = t[1:]
	// Offset correction by node type: as the plan is a subtree,
	// the offsets of the nodes are shifted. The shift size is
	// the offset of the first node (by type).
	var b uint32 // Number of blocks referenced by read nodes.
	var p uint32 // Previous node type.
	c := [3]int{0, -1, -1}
	for i := range t {
		if c[t[i].typ] < 0 {
			p = uint32(i)
			c[t[i].typ] = int(t[i].off)
		}
		t[i].off -= uint32(c[t[i].typ])
		if t[i].typ == NodeRead {
			b += t[i].len
		}
	}
	// Swap merge and read nodes as their order
	// is changed during the traversal.
	tmp := make([]node, p)
	copy(tmp, t[:p])
	copy(t, t[p:])
	copy(t[len(t)-int(p):], tmp)
	// Get blocks by the offset we subtracted
	// from the read node offsets.
	off := c[NodeRead]
	return &querybackendv1.QueryPlan{
		Graph:  *(*[]uint32)(unsafe.Pointer(&t)),
		Blocks: n.p.blocks[off : off+int(b)],
	}
}

func (n *Node) Children() iter.Iterator[*Node] {
	if n.n.typ != NodeMerge {
		return iter.NewEmptyIterator[*Node]()
	}
	return &nodeIterator{n: n}
}

func (n *Node) Blocks() iter.Iterator[*metastorev1.BlockMeta] {
	if n.n.typ != NodeRead {
		return iter.NewEmptyIterator[*metastorev1.BlockMeta]()
	}
	return &blockIterator{n: n}
}

type nodeIterator struct {
	n *Node
	i int
}

func (i *nodeIterator) Err() error   { return nil }
func (i *nodeIterator) Close() error { return nil }

func (i *nodeIterator) Next() bool {
	if i.i >= int(i.n.n.len) {
		return false
	}
	i.i++
	return true
}

func (i *nodeIterator) At() *Node {
	n := i.n.p.nodes[int(i.n.n.off)+i.i-1]
	return &Node{
		Type: i.n.Type,
		p:    i.n.p,
		n:    n,
	}
}

type blockIterator struct {
	n *Node
	i int
}

func (i *blockIterator) Err() error   { return nil }
func (i *blockIterator) Close() error { return nil }

func (i *blockIterator) Next() bool {
	if i.i >= int(i.n.n.len) {
		return false
	}
	i.i++
	return true
}

func (i *blockIterator) At() *metastorev1.BlockMeta {
	return i.n.p.blocks[int(i.n.n.off)+i.i-1]
}

//nolint:unused
func traverseDFS(p *QueryPlan, n node, fn func(node)) {
	stack := make([]node, 0, 32)
	stack = append(stack, n)
	for len(stack) > 0 {
		x := len(stack) - 1
		n, stack = stack[x], stack[:x]
		if n.typ == NodeMerge {
			stack = append(stack, p.nodes[n.off:n.off+n.len]...)
		}
		fn(n)
	}
}

func traverseBFS(p *QueryPlan, n node, fn func(node)) {
	stack := make([]node, 0, 32)
	stack = append(stack, n)
	for len(stack) > 0 {
		n, stack = stack[0], stack[1:]
		if n.typ == NodeMerge {
			stack = append(stack, p.nodes[n.off:n.off+n.len]...)
		}
		fn(n)
	}
}

// uniformSplit splits a slice of length s into
// uniform ranges not exceeding the size max.
func uniformSplit(ret [][2]uint32, s int, max int64) [][2]uint32 {
	ret = ret[:0]
	n := math.Ceil(float64(s) / float64(max)) // Find number of parts.
	o := int(math.Ceil(float64(s) / n))       // Find optimal part size.
	for i := 0; i < s; i += o {
		r := i + o
		if r > s {
			r = s
		}
		ret = append(ret, [2]uint32{uint32(i), uint32(r - i)})
	}
	return ret
}

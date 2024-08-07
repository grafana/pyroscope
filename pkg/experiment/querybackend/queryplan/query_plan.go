package queryplan

import (
	"fmt"
	"io"
	"math"
	"math/rand"
	"slices"
	"strings"
	"unsafe"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	"github.com/grafana/pyroscope/pkg/iter"
)

var xrand = rand.New(rand.NewSource(4349676827832284783))

// QueryPlan represents a physical query plan structured as a DAG.
// Each node in the graph can either be a "merge" or a "read" operation (leaves).
// Merge nodes reference other nodes, while read nodes reference data blocks.
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

func (t NodeType) String() string {
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
	if len(p.Blocks) == 0 {
		return new(QueryPlan)
	}
	qp := QueryPlan{blocks: p.Blocks}
	if len(p.Graph) != 0 || len(p.Graph)%3 == 0 {
		qp.nodes = unsafe.Slice((*node)(unsafe.Pointer(unsafe.SliceData(p.Graph))), len(p.Graph)/3)
	}
	return &qp
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
	xrand.Shuffle(len(blocks), func(i, j int) {
		blocks[i], blocks[j] = blocks[j], blocks[i]
	})
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
	if len(nodes) < 2 {
		return &QueryPlan{
			blocks: blocks,
			nodes:  nodes,
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

// Root returns the root node of the query plan.
func (p *QueryPlan) Root() *Node {
	if len(p.nodes) == 0 {
		return &Node{Type: NodeRead, p: p}
	}
	n := Node{p: p}
	n.n = p.nodes[len(p.nodes)-1]
	n.Type = n.n.typ
	return &n
}

// Plan returns the query plan scoped to the node.
// The plan references the parent plan blocks.
func (n *Node) Plan() *QueryPlan {
	// BFS traversal. Our goal is to preserve the order of
	// nodes as in the original plan, and only shift offsets.
	nodes := make([]node, 0, 32)
	stack := make([]node, 0, 32)
	stack = append(stack, n.n)
	var x node
	for len(stack) > 0 {
		x, stack = stack[0], stack[1:]
		if x.typ == NodeMerge {
			// Add child nodes in the reverse order, to honour
			// the order of the original plan, compensating the
			// stack LIFO at this level. We do it after append
			// to stack to not modify the original plan.
			s := len(stack)
			stack = append(stack, n.p.nodes[x.off:x.off+x.len]...)
			slices.Reverse(stack[s:])
		}
		nodes = append(nodes, x)
	}
	if len(nodes) == 0 {
		return new(QueryPlan)
	}
	// Swap merge and read nodes as their order is changed
	// during the traversal. The order of nodes within the
	// same type is revered as well (order within the level
	// is fixed at traversal).
	var p NodeType // Previous node type.
	var s int
	for i, c := range nodes {
		if p != 0 && c.typ != p {
			s = i
			break
		}
		p = c.typ
	}
	slices.Reverse(nodes[:s]) // Merge nodes.
	slices.Reverse(nodes[s:]) // Read nodes.
	tmp := make([]node, s)
	copy(tmp, nodes[:s])
	copy(nodes, nodes[s:])
	copy(nodes[len(nodes)-s:], tmp)
	if nodes[0].typ != NodeRead {
		panic("bug: first node must be a read node")
	}

	// Offset correction by node type: as the plan is a subtree,
	// the offsets of the child nodes are shifted.
	o := [3]int{0, -1, -1} // Table of offsets by type.
	bs := nodes[0].off     // Offset of the first referenced block.
	for i, c := range nodes {
		// Correct the offset.
		if o[c.typ] < 0 {
			// This is the first node of the type: we remember
			// the offset and reset it to zero, as children of
			// the first node are _always_ placed at the very
			// beginning (for both read and merge nodes).
			nodes[i].off = 0
			o[c.typ] = int(c.len)
		} else {
			nodes[i].off = uint32(o[c.typ])
			o[c.typ] += int(c.len)
		}
	}

	// Update references to the blocks.
	be := bs + uint32(o[NodeRead])
	blocks := n.p.blocks[bs:be]

	return &QueryPlan{
		nodes:  nodes,
		blocks: blocks,
	}
}

func (p *QueryPlan) Proto() *querybackendv1.QueryPlan {
	return &querybackendv1.QueryPlan{
		Graph:  unsafe.Slice((*uint32)(unsafe.Pointer(unsafe.SliceData(p.nodes))), len(p.nodes)*3),
		Blocks: p.blocks,
	}
}

func (p *QueryPlan) String() string {
	var b strings.Builder
	printPlan(&b, "", p, false)
	return b.String()
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

func printPlan(w io.Writer, pad string, p *QueryPlan, debug bool) {
	r := p.Root()
	if debug {
		_, _ = fmt.Fprintf(w, pad+"%s {children: %d, nodes: %d, blocks: %d}\n",
			r.Type, r.n.len, len(r.p.nodes), len(r.p.blocks))
	} else {
		_, _ = fmt.Fprintf(w, pad+"%s (%d)\n", r.Type, r.n.len)
	}

	switch r.Type {
	case NodeMerge:
		c := r.Children()
		for c.Next() {
			printPlan(w, pad+"\t", c.At().Plan(), debug)
		}

	case NodeRead:
		b := r.Blocks()
		for b.Next() {
			_, _ = fmt.Fprintf(w, pad+"\t"+"%+v\n", b.At())
		}

	default:
		panic("unknown type")
	}
}

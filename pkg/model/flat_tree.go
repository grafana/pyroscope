package model

import (
	"encoding/binary"
	"slices"
	"strings"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/v2/pkg/og/util/varint"
)

// FlatTree is a function name tree optimized for merging large trees. It
// produces the same serialized trees and flame graphs as FunctionNameTree,
// but keeps no pointers: nodes are stored in parallel slices and node names
// are interned. Node 0 is the virtual root, and a parent always precedes its
// children.
//
// The children of a node form a list sorted by name in descending order:
// the order in which serialized trees list them. Trees are therefore merged
// by walking the lists of both trees side by side, mostly sequentially.
//
// FlatTree is not safe for concurrent use.
type FlatTree struct {
	names nameDict

	name   []int32
	self   []int64
	parent []int32
	child  []int32 // First child: the one with the greatest name.
	next   []int32 // Next sibling, with a lesser name.

	// inputs is the number of trees merged so far.
	inputs int
}

const noNode = -1

func NewFlatTree() *FlatTree {
	t := &FlatTree{
		name:   []int32{noNode},
		self:   []int64{0},
		parent: []int32{noNode},
		child:  []int32{noNode},
		next:   []int32{noNode},
	}
	t.names.ids = make(map[string]int32)
	return t
}

// Inputs returns the number of trees merged into t.
func (t *FlatTree) Inputs() int { return t.inputs }

// Len returns the number of nodes, excluding the virtual root.
func (t *FlatTree) Len() int { return len(t.parent) - 1 }

func (t *FlatTree) reserve(n int) {
	if n <= cap(t.parent) {
		return
	}
	n = max(n, 2*cap(t.parent))
	g := n - len(t.parent)
	t.name = slices.Grow(t.name, g)
	t.self = slices.Grow(t.self, g)
	t.parent = slices.Grow(t.parent, g)
	t.child = slices.Grow(t.child, g)
	t.next = slices.Grow(t.next, g)
}

// cursor is a position in the children list of a node: new children
// are inserted between prev and cur.
type cursor struct {
	node, prev, cur int32
}

// find positions the cursor at the child with the given name, inserting the
// child if it is not present, and returns the child. Lookups of names in
// descending order take a single pass over the list.
func (t *FlatTree) find(c *cursor, name string, id func() int32) int32 {
	for c.cur != noNode {
		s := t.names.names[t.name[c.cur]]
		if s == name {
			x := c.cur
			c.prev, c.cur = x, t.next[x]
			return x
		}
		if s < name {
			break
		}
		c.prev, c.cur = c.cur, t.next[c.cur]
	}
	if c.prev != noNode && t.names.names[t.name[c.prev]] <= name {
		// Out of order: the child may precede the cursor.
		c.prev, c.cur = noNode, t.child[c.node]
		return t.find(c, name, id)
	}
	return t.insert(c, id())
}

// findID is find for a name of the tree's dictionary.
func (t *FlatTree) findID(c *cursor, id int32) int32 {
	name := t.names.names[id]
	for c.cur != noNode {
		if t.name[c.cur] == id {
			x := c.cur
			c.prev, c.cur = x, t.next[x]
			return x
		}
		if t.names.names[t.name[c.cur]] < name {
			break
		}
		c.prev, c.cur = c.cur, t.next[c.cur]
	}
	if c.prev != noNode && t.names.names[t.name[c.prev]] <= name {
		c.prev, c.cur = noNode, t.child[c.node]
		return t.findID(c, id)
	}
	return t.insert(c, id)
}

// insert inserts a child at the cursor.
func (t *FlatTree) insert(c *cursor, id int32) int32 {
	x := int32(len(t.parent))
	if len(t.parent) == cap(t.parent) {
		t.reserve(len(t.parent) + 1)
	}
	t.name = append(t.name, id)
	t.self = append(t.self, 0)
	t.parent = append(t.parent, c.node)
	t.child = append(t.child, noNode)
	t.next = append(t.next, c.cur)
	if c.prev == noNode {
		t.child[c.node] = x
	} else {
		t.next[c.prev] = x
	}
	c.prev = x
	return x
}

type mergeFrame struct {
	cursor
	left uint64 // Children left to merge.
}

// MergeBytes merges a tree serialized with FunctionNameTree.Bytes or
// FlatTree.Bytes. On a malformed input an error is returned and t may
// include a partially merged prefix of the input.
func (t *FlatTree) MergeBytes(b []byte) error {
	t.inputs++
	if len(b) < 2 {
		return nil
	}
	t.reserve(len(t.parent) + len(b)/32)
	// The virtual root record: only the children count matters.
	_, _, offset, err := readFlatTreeRecord(b, 0)
	if err != nil {
		return err
	}
	childrenLen, o := varint.Uvarint(b[offset:])
	if o < 0 {
		return errMalformedTreeBytes
	}
	offset += o
	stack := make([]mergeFrame, 0, 256)
	if childrenLen > 0 {
		stack = append(stack, mergeFrame{cursor{0, noNode, t.child[0]}, childrenLen})
	}
	var nameBytes []byte
	var value uint64
	id := func() int32 { return t.names.idBytes(nameBytes) }
	for len(stack) > 0 {
		f := &stack[len(stack)-1]
		if f.left == 0 {
			stack = stack[:len(stack)-1]
			continue
		}
		f.left--
		if nameBytes, value, offset, err = readFlatTreeRecord(b, offset); err != nil {
			return err
		}
		if childrenLen, o = varint.Uvarint(b[offset:]); o < 0 {
			return errMalformedTreeBytes
		}
		offset += o
		x := t.find(&f.cursor, unsafeString(nameBytes), id)
		t.self[x] += int64(value)
		if childrenLen > 0 {
			stack = append(stack, mergeFrame{cursor{x, noNode, t.child[x]}, childrenLen})
		}
	}
	return nil
}

func readFlatTreeRecord(b []byte, offset int) (name []byte, value uint64, next int, err error) {
	if offset >= len(b) {
		return nil, 0, 0, errMalformedTreeBytes
	}
	nameLen, o := varint.Uvarint(b[offset:])
	if o < 0 {
		return nil, 0, 0, errMalformedTreeBytes
	}
	offset += o
	if nameLen > uint64(len(b)-offset) {
		return nil, 0, 0, errMalformedTreeBytes
	}
	name = b[offset : offset+int(nameLen)]
	offset += int(nameLen)
	if offset >= len(b) {
		return nil, 0, 0, errMalformedTreeBytes
	}
	value, o = varint.Uvarint(b[offset:])
	if o < 0 {
		return nil, 0, 0, errMalformedTreeBytes
	}
	return name, value, offset + o, nil
}

// CallTree is a function name tree as built from stack trace samples:
// node 0 is the root, a parent always precedes its children, and the
// names of a node's children are unique. Total is the node's self value
// plus the totals of its children, except for the root, whose total is
// zero.
type CallTree struct {
	Parent []int32
	Name   []int32 // Index into Names, or CallTreeTruncated.
	Self   []int64
	Total  []int64
	Names  []string
}

// CallTreeTruncated is the name of a leaf of truncated stack traces, the
// sentinel location of StacktraceTree: it is always replaced by "other".
const CallTreeTruncated = -2

// KthLargest returns the k-th largest value of s (1-based), or 0 if s has
// fewer values: the root of a min-heap of the k largest values. The values
// are reordered.
func KthLargest(s []int64, k int) int64 { return kthLargest(s, k) }

// MergeCallTree merges the call tree truncated to maxNodes as if it was
// converted to FunctionNameTree with TreeFromStacktraceTree and serialized
// with Bytes(maxNodes). The names of the call tree are retained.
//
// The caller must ensure that no node is named "other", see
// HasOtherFunctionName.
func (t *FlatTree) MergeCallTree(c *CallTree, maxNodes int64) {
	t.MergePreparedCallTree(PrepareCallTree(c, maxNodes))
}

// PreparedCallTree is a call tree truncated and ordered for merging.
type PreparedCallTree struct {
	c        *CallTree
	off      []int32 // The children of node i are kids[off[i]:off[i+1]].
	kids     []int32 // Node n+i is the "other" node of node i.
	otherSum []int64
}

// PrepareCallTree truncates the call tree for MergePreparedCallTree, which
// is then equivalent to MergeCallTree(c, maxNodes). Preparation takes most
// of the time of the merge, and does not need access to the tree.
func PrepareCallTree(c *CallTree, maxNodes int64) *PreparedCallTree {
	n := len(c.Parent)
	if n < 2 {
		return new(PreparedCallTree)
	}
	minValue := callTreeMinValue(c, maxNodes)

	// Children of the kept nodes, plus the "other" nodes (n+i is the "other"
	// child of node i), sorted by name in descending order.
	var otherSum []int64
	kept := make([]bool, n)
	kept[0] = true
	off := make([]int32, n+1)
	for i := 1; i < n; i++ {
		p := c.Parent[i]
		if !kept[p] {
			continue
		}
		if c.Total[i] >= minValue && c.Name[i] != CallTreeTruncated {
			kept[i] = true
			off[p+1]++
			continue
		}
		if otherSum == nil {
			otherSum = make([]int64, n)
		}
		otherSum[p] += c.Total[i]
	}
	for i, v := range otherSum {
		if v > 0 {
			off[i+1]++
		}
	}
	for i := 1; i <= n; i++ {
		off[i] += off[i-1]
	}
	kids := make([]int32, off[n])
	pos := slices.Clone(off[:n])
	for i := 1; i < n; i++ {
		if kept[i] {
			p := c.Parent[i]
			kids[pos[p]] = int32(i)
			pos[p]++
		}
	}
	for i, v := range otherSum {
		if v > 0 {
			kids[pos[i]] = int32(n + i)
			pos[i]++
		}
	}
	p := &PreparedCallTree{c: c, off: off, kids: kids, otherSum: otherSum}
	for i := 0; i < n; i++ {
		if s := kids[off[i]:off[i+1]]; len(s) > 1 {
			slices.SortFunc(s, func(a, b int32) int { return strings.Compare(p.nameOf(b), p.nameOf(a)) })
		}
	}
	return p
}

func (p *PreparedCallTree) nameOf(x int32) string {
	if x >= int32(len(p.c.Parent)) {
		return truncatedNodeName
	}
	return p.c.Names[p.c.Name[x]]
}

func (p *PreparedCallTree) subtreeSizes(roots []int32) []int64 {
	n := int32(len(p.c.Parent))
	sizes := make([]int64, len(roots))
	stack := make([]int32, 0, 256)
	for i, r := range roots {
		stack = append(stack[:0], r)
		for len(stack) > 0 {
			x := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			sizes[i]++
			if x < n {
				stack = append(stack, p.kids[p.off[x]:p.off[x+1]]...)
			}
		}
	}
	return sizes
}

// MergePreparedCallTree merges the call tree prepared with PrepareCallTree.
func (t *FlatTree) MergePreparedCallTree(p *PreparedCallTree) {
	t.inputs++
	if p.c == nil {
		return
	}
	t.reserve(len(t.parent) + len(p.kids))
	t.mergePreparedRoots(p, p.kids[p.off[0]:p.off[1]])
}

func (t *FlatTree) mergePreparedRoots(p *PreparedCallTree, roots []int32) {
	c, off, kids := p.c, p.off, p.kids
	n := int32(len(c.Parent))
	ids := make([]int32, len(c.Names))
	for i := range ids {
		ids[i] = noNode
	}
	other := t.names.id(truncatedNodeName)
	if len(t.parent) == 1 {
		t.buildPreparedCallTree(p, roots, ids, other)
		return
	}
	type frame struct {
		cursor
		kids []int32
	}
	stack := []frame{{cursor{0, noNode, t.child[0]}, roots}}
	for len(stack) > 0 {
		f := &stack[len(stack)-1]
		if len(f.kids) == 0 {
			stack = stack[:len(stack)-1]
			continue
		}
		var src int32
		src, f.kids = f.kids[0], f.kids[1:]
		if src >= n {
			x := t.findID(&f.cursor, other)
			t.self[x] += p.otherSum[src-n]
			continue
		}
		id := ids[c.Name[src]]
		if id == noNode {
			id = t.names.id(c.Names[c.Name[src]])
			ids[c.Name[src]] = id
		}
		x := t.findID(&f.cursor, id)
		t.self[x] += c.Self[src]
		if s := kids[off[src]:off[src+1]]; len(s) > 0 {
			stack = append(stack, frame{cursor{x, noNode, t.child[x]}, s})
		}
	}
}

// buildPreparedCallTree copies the prepared call tree into the empty tree:
// the children are already in order, so no lookups are needed.
func (t *FlatTree) buildPreparedCallTree(p *PreparedCallTree, roots, ids []int32, other int32) {
	c, off, kids := p.c, p.off, p.kids
	n := int32(len(c.Parent))
	type frame struct {
		node, prev int32
		kids       []int32
	}
	stack := []frame{{0, noNode, roots}}
	for len(stack) > 0 {
		f := &stack[len(stack)-1]
		if len(f.kids) == 0 {
			stack = stack[:len(stack)-1]
			continue
		}
		var src int32
		src, f.kids = f.kids[0], f.kids[1:]
		id, self := other, int64(0)
		if src >= n {
			self = p.otherSum[src-n]
		} else {
			if id = ids[c.Name[src]]; id == noNode {
				id = t.names.id(c.Names[c.Name[src]])
				ids[c.Name[src]] = id
			}
			self = c.Self[src]
		}
		x := int32(len(t.parent))
		t.name = append(t.name, id)
		t.self = append(t.self, self)
		t.parent = append(t.parent, f.node)
		t.child = append(t.child, noNode)
		t.next = append(t.next, noNode)
		if f.prev == noNode {
			t.child[f.node] = x
		} else {
			t.next[f.prev] = x
		}
		f.prev = x
		if src < n {
			if s := kids[off[src]:off[src+1]]; len(s) > 0 {
				stack = append(stack, frame{x, noNode, s})
			}
		}
	}
}

// MergeTree merges src into t. src must not be used afterwards: t may take
// over its contents.
func (t *FlatTree) MergeTree(src *FlatTree) {
	if len(t.parent) == 1 {
		inputs := t.inputs
		*t = *src
		t.inputs += inputs
		return
	}
	t.inputs += src.inputs
	if len(src.parent) == 1 {
		return
	}
	t.reserve(len(t.parent) + len(src.parent)/4)
	t.mergeTreeChildren(src, t.sourceIDs(src), 0, src.child[0])
}

func (t *FlatTree) sourceIDs(src *FlatTree) []int32 {
	ids := make([]int32, len(src.names.names))
	for i := range ids {
		ids[i] = noNode
	}
	return ids
}

func (t *FlatTree) sourceID(src *FlatTree, ids []int32, s int32) int32 {
	id := ids[src.name[s]]
	if id == noNode {
		id = t.names.id(src.names.names[src.name[s]])
		ids[src.name[s]] = id
	}
	return id
}

func (t *FlatTree) mergeTreeRoots(src *FlatTree, roots []int32) {
	ids := t.sourceIDs(src)
	c := cursor{0, noNode, t.child[0]}
	for _, s := range roots {
		x := t.findID(&c, t.sourceID(src, ids, s))
		t.self[x] += src.self[s]
		if first := src.child[s]; first != noNode {
			t.mergeTreeChildren(src, ids, x, first)
		}
	}
}

func (t *FlatTree) mergeTreeChildren(src *FlatTree, ids []int32, node, first int32) {
	type frame struct {
		cursor
		src int32 // Next child of the source node to merge.
	}
	stack := []frame{{cursor{node, noNode, t.child[node]}, first}}
	for len(stack) > 0 {
		f := &stack[len(stack)-1]
		if f.src == noNode {
			stack = stack[:len(stack)-1]
			continue
		}
		var s int32
		s, f.src = f.src, src.next[f.src]
		x := t.findID(&f.cursor, t.sourceID(src, ids, s))
		t.self[x] += src.self[s]
		if c := src.child[s]; c != noNode {
			stack = append(stack, frame{cursor{x, noNode, t.child[x]}, c})
		}
	}
}

// callTreeMinValue returns the minimal total of the call tree nodes that are
// kept when the tree is converted with TreeFromStacktraceTree(maxNodes), and
// then serialized with Bytes(maxNodes): each step only keeps nodes with
// totals not less than the maxNodes-th largest total of its input, and
// replaces the others with a single "other" node per parent.
func callTreeMinValue(c *CallTree, maxNodes int64) int64 {
	n := len(c.Parent)
	if maxNodes < 1 || maxNodes >= int64(n) {
		// The conversion keeps all nodes, and so does serialization:
		// the tree has fewer nodes than the root and the limit.
		return 0
	}
	// TreeFromStacktraceTree: the root's total is zero and it takes part.
	m1 := kthLargest(slices.Clone(c.Total), int(maxNodes))
	// Totals of the converted tree: kept nodes and "other" nodes that
	// replace the truncated children of a kept node or the root.
	totals := make([]int64, 0, n)
	var other []int64
	for i := 1; i < n; i++ {
		p := c.Parent[i]
		if c.Total[i] >= m1 && c.Name[i] != CallTreeTruncated {
			totals = append(totals, c.Total[i])
		} else if p == 0 || c.Total[p] >= m1 {
			if other == nil {
				other = make([]int64, n)
			}
			other[p] += c.Total[i]
		}
	}
	for _, v := range other {
		if v > 0 {
			totals = append(totals, v)
		}
	}
	if int64(len(totals)) <= maxNodes {
		return m1
	}
	return max(m1, kthLargest(totals, int(maxNodes)))
}

// HasOtherFunctionName reports whether any of the names equals the name of
// the node that replaces truncated nodes.
func HasOtherFunctionName(names []string) bool {
	return slices.Contains(names, truncatedNodeName)
}

// totals returns the totals of the nodes: self plus the totals of children.
func (t *FlatTree) totals() []int64 {
	total := slices.Clone(t.self)
	for i := len(total) - 1; i > 0; i-- {
		total[t.parent[i]] += total[i]
	}
	return total
}

// minValue is FunctionNameTree.minValue.
func (t *FlatTree) minValue(total []int64, maxNodes int64) int64 {
	n := int64(len(total) - 1)
	if maxNodes < 1 || n <= maxNodes {
		return 0
	}
	return kthLargest(slices.Clone(total[1:]), int(maxNodes))
}

// Bytes serializes the tree as FunctionNameTree.Bytes does. If the tree is
// the only tree merged, it is serialized as is, without truncation: in the
// same way as a single report is passed through by an aggregator.
func (t *FlatTree) Bytes(maxNodes int64) []byte {
	if t.inputs == 1 {
		maxNodes = 0
	}
	return t.MarshalTruncate(maxNodes)
}

// MarshalTruncate serializes the tree truncated to maxNodes as
// FunctionNameTree.MarshalTruncate does.
func (t *FlatTree) MarshalTruncate(maxNodes int64) []byte {
	b, _ := t.marshal(maxNodes, false)
	return b
}

// MarshalTruncateCompact is MarshalTruncate in the compact format: node
// names are written as indices into the returned names, in the order of
// first use. The record of the virtual root refers to no name.
func (t *FlatTree) MarshalTruncateCompact(maxNodes int64) ([]byte, []string) {
	return t.marshal(maxNodes, true)
}

// BytesCompact is Bytes in the compact format.
func (t *FlatTree) BytesCompact(maxNodes int64) ([]byte, []string) {
	if t.inputs == 1 {
		maxNodes = 0
	}
	return t.marshal(maxNodes, true)
}

func (t *FlatTree) marshal(maxNodes int64, compact bool) ([]byte, []string) {
	if len(t.parent) < 2 {
		return nil, nil
	}
	total := t.totals()
	m := newTreeMarshaler(t, total, t.minValue(total, maxNodes), compact)
	m.write(0)
	return m.buf, m.table
}

type treeMarshaler struct {
	t        *FlatTree
	n        int32
	total    []int64
	minValue int64
	compact  bool

	names    []string
	other    int32
	hasOther bool

	// Nodes with ids >= n are "other" nodes that replace truncated children,
	// synthetic holds their weight. Existing "other" nodes take the weight
	// of truncated siblings in extra.
	extra     []int64
	synthetic []int64

	buf        []byte
	table      []string
	index      []int32 // Position of the name in the table, by name id.
	otherIndex int32

	stack    []int32
	children []int32
}

func newTreeMarshaler(t *FlatTree, total []int64, minValue int64, compact bool) *treeMarshaler {
	n := len(t.parent)
	m := &treeMarshaler{
		t:          t,
		n:          int32(n),
		total:      total,
		minValue:   minValue,
		compact:    compact,
		names:      t.names.names,
		otherIndex: -1,
		stack:      make([]int32, 0, 1024),
		children:   make([]int32, 0, 128),
	}
	m.other, m.hasOther = t.names.ids[truncatedNodeName]
	if compact {
		m.buf = make([]byte, 0, n*6+64)
		m.index = make([]int32, len(m.names))
		for i := range m.index {
			m.index[i] = -1
		}
	} else {
		size := 0
		for i := 1; i < n; i++ {
			size += len(m.names[t.name[i]]) + 4
		}
		m.buf = make([]byte, 0, size+64)
	}
	return m
}

func (m *treeMarshaler) writeName(id int32) {
	if !m.compact {
		name := truncatedNodeName
		if id >= 0 {
			name = m.names[id]
		}
		m.buf = binary.AppendUvarint(m.buf, uint64(len(name)))
		m.buf = append(m.buf, name...)
		return
	}
	if id < 0 {
		if m.otherIndex >= 0 {
			m.buf = binary.AppendUvarint(m.buf, uint64(m.otherIndex))
			return
		}
		if !m.hasOther {
			m.other, m.hasOther = m.t.names.id(truncatedNodeName), true
			m.names = m.t.names.names
			m.index = append(m.index, -1)
		}
		id = m.other
	}
	if m.index[id] < 0 {
		m.index[id] = int32(len(m.table))
		m.table = append(m.table, m.names[id])
	}
	m.buf = binary.AppendUvarint(m.buf, uint64(m.index[id]))
}

func (m *treeMarshaler) write(x int32) {
	t := m.t
	m.stack = append(m.stack[:0], x)
	for len(m.stack) > 0 {
		x := m.stack[len(m.stack)-1]
		m.stack = m.stack[:len(m.stack)-1]
		if x >= m.n {
			m.writeName(-1)
			m.buf = binary.AppendUvarint(m.buf, uint64(m.synthetic[x-m.n]))
			m.buf = binary.AppendUvarint(m.buf, 0)
			continue
		}
		var self int64
		if x > 0 {
			m.writeName(t.name[x])
			self = t.self[x]
			if m.extra != nil {
				self += m.extra[x]
			}
		} else {
			// The virtual root.
			m.buf = binary.AppendUvarint(m.buf, 0)
		}
		m.buf = binary.AppendUvarint(m.buf, uint64(self))
		children := m.keptChildren(x)
		// Push in ascending order, to pop in descending.
		for i := len(children) - 1; i >= 0; i-- {
			m.stack = append(m.stack, children[i])
		}
		m.buf = binary.AppendUvarint(m.buf, uint64(len(children)))
	}
}

func (m *treeMarshaler) keptChildren(x int32) []int32 {
	// Kept children in descending name order; the "other" node
	// is placed according to its name.
	t := m.t
	children := m.children[:0]
	var truncated int64
	otherAt := -1
	for c := t.child[x]; c != noNode; c = t.next[c] {
		isOther := m.hasOther && t.name[c] == m.other
		if m.total[c] >= m.minValue || isOther {
			if isOther {
				otherAt = len(children)
			}
			children = append(children, c)
		} else {
			truncated += m.total[c]
		}
	}
	if truncated > 0 {
		if otherAt >= 0 {
			m.addExtra(children[otherAt], truncated)
		} else {
			i := 0
			for i < len(children) && m.names[t.name[children[i]]] > truncatedNodeName {
				i++
			}
			children = slices.Insert(children, i, m.n+int32(len(m.synthetic)))
			m.synthetic = append(m.synthetic, truncated)
		}
	}
	m.children = children
	return children
}

func (m *treeMarshaler) addExtra(x int32, v int64) {
	if m.extra == nil {
		m.extra = make([]int64, m.n)
	}
	m.extra[x] += v
}

// readCompactRecord reads the name index, the value, and the number of
// children of a node record.
func readCompactRecord(b []byte, offset int) (name, value, children uint64, next int, err error) {
	var o int
	if offset >= len(b) {
		return 0, 0, 0, 0, errMalformedTreeBytes
	}
	if name, o = varint.Uvarint(b[offset:]); o < 0 {
		return 0, 0, 0, 0, errMalformedTreeBytes
	}
	offset += o
	if offset >= len(b) {
		return 0, 0, 0, 0, errMalformedTreeBytes
	}
	if value, o = varint.Uvarint(b[offset:]); o < 0 {
		return 0, 0, 0, 0, errMalformedTreeBytes
	}
	offset += o
	if offset >= len(b) {
		return 0, 0, 0, 0, errMalformedTreeBytes
	}
	if children, o = varint.Uvarint(b[offset:]); o < 0 {
		return 0, 0, 0, 0, errMalformedTreeBytes
	}
	return name, value, children, offset + o, nil
}

// MergeCompactBytes merges a tree serialized with MarshalTruncateCompact.
// The names are retained.
func (t *FlatTree) MergeCompactBytes(b []byte, names []string) error {
	t.inputs++
	if len(b) < 2 {
		return nil
	}
	t.reserve(len(t.parent) + len(b)/6)
	_, _, childrenLen, offset, err := readCompactRecord(b, 0)
	if err != nil {
		return err
	}
	_, err = t.mergeCompactRecords(b, offset, newCompactNames(names), 0, childrenLen)
	return err
}

type compactNames struct {
	names []string
	ids   []int32
}

func newCompactNames(names []string) *compactNames {
	ids := make([]int32, len(names))
	for i := range ids {
		ids[i] = noNode
	}
	return &compactNames{names: names, ids: ids}
}

func (n *compactNames) id(t *FlatTree, i uint64) (int32, error) {
	if i >= uint64(len(n.ids)) {
		return 0, errMalformedTreeBytes
	}
	id := n.ids[i]
	if id == noNode {
		id = t.names.id(n.names[i])
		n.ids[i] = id
	}
	return id, nil
}

func (t *FlatTree) mergeCompactRecords(b []byte, offset int, names *compactNames, node int32, count uint64) (int, error) {
	if count == 0 {
		return offset, nil
	}
	stack := make([]mergeFrame, 1, 256)
	stack[0] = mergeFrame{cursor{node, noNode, t.child[node]}, count}
	for len(stack) > 0 {
		f := &stack[len(stack)-1]
		if f.left == 0 {
			stack = stack[:len(stack)-1]
			continue
		}
		f.left--
		name, value, children, next, err := readCompactRecord(b, offset)
		if err != nil {
			return 0, err
		}
		offset = next
		id, err := names.id(t, name)
		if err != nil {
			return 0, err
		}
		x := t.findID(&f.cursor, id)
		t.self[x] += int64(value)
		if children > 0 {
			stack = append(stack, mergeFrame{cursor{x, noNode, t.child[x]}, children})
		}
	}
	return offset, nil
}

func (t *FlatTree) mergeCompactRoots(b []byte, names []string, offsets []int) error {
	n := newCompactNames(names)
	c := cursor{0, noNode, t.child[0]}
	for _, offset := range offsets {
		name, value, children, next, err := readCompactRecord(b, offset)
		if err != nil {
			return err
		}
		id, err := n.id(t, name)
		if err != nil {
			return err
		}
		x := t.findID(&c, id)
		t.self[x] += int64(value)
		if _, err = t.mergeCompactRecords(b, next, n, x, children); err != nil {
			return err
		}
	}
	return nil
}

func skipCompactRecords(b []byte, offset int, count uint64) (int, error) {
	for count > 0 {
		_, _, children, next, err := readCompactRecord(b, offset)
		if err != nil {
			return 0, err
		}
		offset, count = next, count-1+children
	}
	return offset, nil
}

// FlameGraph builds the flame graph as NewFlameGraph does for the tree.
func (t *FlatTree) FlameGraph(maxNodes int64) *querierv1.FlameGraph {
	total := t.totals()
	var sum int64
	for c := t.child[0]; c != noNode; c = t.next[c] {
		sum += total[c]
	}
	total[0] = sum
	minValue := t.minValue(total, maxNodes)
	names := t.names.names
	other, hasOther := t.names.ids[truncatedNodeName]

	// Name positions in the output; the root is named "total". Nodes
	// named "other" are never visited: they are folded into the "other"
	// nodes that replace truncated nodes.
	position := make([]int32, len(names))
	for i := range position {
		position[i] = -1
	}
	out := []string{"total"}
	if id, ok := t.names.ids["total"]; ok {
		position[id] = 0
	}
	otherPosition := int32(-1)

	type item struct {
		x       int32 // Node, or -1 for "other" nodes.
		level   int32
		xOffset int64
		self    int64 // For "other" nodes.
	}
	var levels [][]int64
	var maxSelf int64
	stack := make([]item, 1, 1024)
	stack[0] = item{x: 0}
	children := make([]int32, 0, 128)
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		var self, tot int64
		var pos int32
		switch {
		case cur.x < 0:
			self, tot = cur.self, cur.self
			if otherPosition < 0 {
				otherPosition = int32(len(out))
				out = append(out, truncatedNodeName)
			}
			pos = otherPosition
		case cur.x == 0:
			tot = total[0]
		default:
			self, tot = t.self[cur.x], total[cur.x]
			id := t.name[cur.x]
			if pos = position[id]; pos < 0 {
				pos = int32(len(out))
				out = append(out, names[id])
				position[id] = pos
			}
		}
		maxSelf = max(maxSelf, self)
		if int(cur.level) == len(levels) {
			levels = append(levels, nil)
		}
		levels[cur.level] = append(levels[cur.level], int64(pos), self, tot, cur.xOffset)
		if cur.x < 0 {
			continue
		}

		children = children[:0]
		for c := t.child[cur.x]; c != noNode; c = t.next[c] {
			children = append(children, c)
		}
		xOffset := cur.xOffset + self
		var truncated int64
		for i := len(children) - 1; i >= 0; i-- {
			c := children[i]
			if total[c] >= minValue && !(hasOther && t.name[c] == other) {
				stack = append(stack, item{x: c, level: cur.level + 1, xOffset: xOffset})
				xOffset += total[c]
			} else {
				truncated += total[c]
			}
		}
		if truncated != 0 {
			stack = append(stack, item{x: -1, level: cur.level + 1, xOffset: xOffset, self: truncated})
		}
	}

	result := make([]*querierv1.Level, len(levels))
	for i, l := range levels {
		// Nodes are listed in reverse visiting order, each as
		// [x offset delta, total, self, name position].
		values := make([]int64, len(l))
		var prev int64
		for j, k := 0, len(l)-4; k >= 0; j, k = j+4, k-4 {
			values[j] = l[k+3] - prev
			values[j+1] = l[k+2]
			values[j+2] = l[k+1]
			values[j+3] = l[k]
			prev += values[j] + values[j+1]
		}
		result[i] = &querierv1.Level{Values: values}
	}
	return &querierv1.FlameGraph{
		Names:   out,
		Levels:  result,
		Total:   sum,
		MaxSelf: maxSelf,
	}
}

type nameDict struct {
	ids   map[string]int32
	names []string
}

// id returns the identifier of the name; s is retained if not present.
func (d *nameDict) id(s string) int32 {
	if i, ok := d.ids[s]; ok {
		return i
	}
	i := int32(len(d.names))
	d.names = append(d.names, s)
	d.ids[s] = i
	return i
}

func (d *nameDict) idBytes(b []byte) int32 {
	if i, ok := d.ids[string(b)]; ok {
		return i
	}
	return d.id(string(b))
}

// kthLargest returns the k-th largest value (1-based); s is reordered.
// It is the value the root of a min-heap of the k largest values holds.
func kthLargest(s []int64, k int) int64 {
	if k < 1 || k > len(s) {
		return 0
	}
	// Select the element at index k-1 in descending order,
	// with a three-way partition: totals have many duplicates.
	target := k - 1
	lo, hi := 0, len(s)-1
	for lo < hi {
		a, b, c := s[lo], s[lo+(hi-lo)/2], s[hi]
		pivot := max(min(a, b), min(max(a, b), c))
		lt, i, gt := lo, lo, hi
		for i <= gt {
			switch v := s[i]; {
			case v > pivot:
				s[lt], s[i] = s[i], s[lt]
				lt++
				i++
			case v < pivot:
				s[i], s[gt] = s[gt], s[i]
				gt--
			default:
				i++
			}
		}
		switch {
		case target < lt:
			hi = lt - 1
		case target > gt:
			lo = gt + 1
		default:
			return pivot
		}
	}
	return s[target]
}

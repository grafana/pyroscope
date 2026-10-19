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
	if c.prev != noNode && t.names.names[t.name[c.prev]] <= name {
		// Out of order: start over.
		c.prev, c.cur = noNode, t.child[c.node]
	}
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
	x := int32(len(t.parent))
	t.name = append(t.name, id())
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
	Name   []int32 // Index into Names.
	Self   []int64
	Total  []int64
	Names  []string
}

// MergeCallTree merges the call tree truncated to maxNodes as if it was
// converted to FunctionNameTree with TreeFromStacktraceTree and serialized
// with Bytes(maxNodes). The names of the call tree are copied if retained.
//
// The caller must ensure that no node is named "other", see
// HasOtherFunctionName.
func (t *FlatTree) MergeCallTree(c *CallTree, maxNodes int64) {
	t.inputs++
	n := len(c.Parent)
	if n < 2 {
		return
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
		if c.Total[i] >= minValue {
			kept[i] = true
			off[p+1]++
			continue
		}
		if otherSum == nil {
			otherSum = make([]int64, n)
		}
		if otherSum[p] == 0 {
			off[p+1]++
		}
		otherSum[p] += c.Total[i]
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
	nameOf := func(x int32) string {
		if x >= int32(n) {
			return truncatedNodeName
		}
		return c.Names[c.Name[x]]
	}
	for i := 0; i < n; i++ {
		if s := kids[off[i]:off[i+1]]; len(s) > 1 {
			slices.SortFunc(s, func(a, b int32) int { return strings.Compare(nameOf(b), nameOf(a)) })
		}
	}

	t.reserve(len(t.parent) + int(off[n]))
	ids := make([]int32, len(c.Names))
	for i := range ids {
		ids[i] = noNode
	}
	var src int32
	id := func() int32 {
		if src >= int32(n) {
			return t.names.id(truncatedNodeName)
		}
		x := c.Name[src]
		if ids[x] == noNode {
			ids[x] = t.names.id(strings.Clone(c.Names[x]))
		}
		return ids[x]
	}
	type frame struct {
		cursor
		kids []int32
	}
	stack := []frame{{cursor{0, noNode, t.child[0]}, kids[off[0]:off[1]]}}
	for len(stack) > 0 {
		f := &stack[len(stack)-1]
		if len(f.kids) == 0 {
			stack = stack[:len(stack)-1]
			continue
		}
		src, f.kids = f.kids[0], f.kids[1:]
		x := t.find(&f.cursor, nameOf(src), id)
		if src >= int32(n) {
			t.self[x] += otherSum[src-int32(n)]
			continue
		}
		t.self[x] += c.Self[src]
		if s := kids[off[src]:off[src+1]]; len(s) > 0 {
			stack = append(stack, frame{cursor{x, noNode, t.child[x]}, s})
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
		if c.Total[i] >= m1 {
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
	n := len(t.parent)
	if n < 2 {
		return nil
	}
	total := t.totals()
	minValue := t.minValue(total, maxNodes)
	names := t.names.names
	other, hasOther := t.names.ids[truncatedNodeName]

	// Nodes with ids >= n are "other" nodes that replace truncated children,
	// synthetic holds their weight. Existing "other" nodes take the weight
	// of truncated siblings in extra.
	var extra []int64
	var synthetic []int64

	size := 0
	for i := 1; i < n; i++ {
		size += len(names[t.name[i]]) + 4
	}
	buf := make([]byte, 0, size+64)
	stack := make([]int32, 1, 1024)
	children := make([]int32, 0, 128)
	for len(stack) > 0 {
		x := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if x >= int32(n) {
			buf = binary.AppendUvarint(buf, uint64(len(truncatedNodeName)))
			buf = append(buf, truncatedNodeName...)
			buf = binary.AppendUvarint(buf, uint64(synthetic[x-int32(n)]))
			buf = binary.AppendUvarint(buf, 0)
			continue
		}
		var self int64
		var name string
		if x > 0 {
			name = names[t.name[x]]
			self = t.self[x]
			if extra != nil {
				self += extra[x]
			}
		}
		buf = binary.AppendUvarint(buf, uint64(len(name)))
		buf = append(buf, name...)
		buf = binary.AppendUvarint(buf, uint64(self))

		// Kept children in descending name order; the "other" node
		// is placed according to its name.
		children = children[:0]
		var truncated int64
		otherAt := -1
		for c := t.child[x]; c != noNode; c = t.next[c] {
			isOther := hasOther && t.name[c] == other
			if total[c] >= minValue || isOther {
				if isOther {
					otherAt = len(children)
				}
				children = append(children, c)
			} else {
				truncated += total[c]
			}
		}
		if truncated > 0 {
			if otherAt >= 0 {
				if extra == nil {
					extra = make([]int64, n)
				}
				extra[children[otherAt]] += truncated
			} else {
				i := 0
				for i < len(children) && names[t.name[children[i]]] > truncatedNodeName {
					i++
				}
				children = slices.Insert(children, i, int32(n+len(synthetic)))
				synthetic = append(synthetic, truncated)
			}
		}
		// Push in ascending order, to pop in descending.
		for i := len(children) - 1; i >= 0; i-- {
			stack = append(stack, children[i])
		}
		buf = binary.AppendUvarint(buf, uint64(len(children)))
	}
	return buf
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

package model

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unsafe"

	profilev1 "github.com/grafana/pyroscope/api/gen/proto/go/google/v1"

	"github.com/xlab/treeprint"

	"github.com/grafana/pyroscope/v2/pkg/og/util/varint"
	"github.com/grafana/pyroscope/v2/pkg/slices"
	"github.com/grafana/pyroscope/v2/pkg/util/minheap"
)

const OtherFunctionName = FunctionName(truncatedNodeName)

type FunctionName string

type FunctionNameI struct {
}

func (FunctionNameI) IsLocationTree() bool {
	return false
}

func (FunctionNameI) newOther() FunctionName { //nolint:unused
	return OtherFunctionName
}

func (FunctionNameI) marshalNode(w io.Writer, vw varint.Writer, n *node[FunctionName], _ func(FunctionName) FunctionName) error { //nolint:unused
	if _, err := vw.Write(w, uint64(len(n.name))); err != nil {
		return err
	}
	if _, err := w.Write(unsafeStringBytes(string(n.name))); err != nil {
		return err
	}
	_, err := vw.Write(w, uint64(n.self))
	return err
}

func (FunctionNameI) unmarshalNode(b []byte, offset int) (FunctionName, int64, int, error) { //nolint:unused
	nameLen, o := varint.Uvarint(b[offset:])
	if o < 0 {
		return "", 0, 0, errMalformedTreeBytes
	}
	offset += o
	// Note that we allocate a string, instead of referencing b's capacity.
	name := FunctionName(b[offset : offset+int(nameLen)])
	offset += int(nameLen)
	value, o := varint.Uvarint(b[offset:])
	if o < 0 {
		return "", 0, 0, errMalformedTreeBytes
	}
	offset += o
	return name, int64(value), offset, nil
}

func (FunctionNameI) mergeNode(parent *node[FunctionName], b []byte, offset int, format func(FunctionName) FunctionName, newNode func() *node[FunctionName]) (*node[FunctionName], int, error) { //nolint:unused
	nameLen, o := varint.Uvarint(b[offset:])
	if o < 0 {
		return nil, 0, errMalformedTreeBytes
	}
	offset += o
	if int(nameLen) > len(b)-offset {
		return nil, 0, errMalformedTreeBytes
	}
	nameBytes := b[offset : offset+int(nameLen)]
	offset += int(nameLen)
	value, o := varint.Uvarint(b[offset:])
	if o < 0 {
		return nil, 0, errMalformedTreeBytes
	}
	offset += o

	var n *node[FunctionName]
	if format != nil {
		// The format function may retain the name: hand it an owned copy.
		n = parent.insert(format(FunctionName(nameBytes)), newNode)
	} else {
		// Look up the child through a zero-copy view of the name: an owned
		// copy is only allocated when a new node is actually inserted.
		name := FunctionName(unsafeString(nameBytes))
		i, found := parent.find(name)
		if found == nil {
			found = parent.insertAt(i, FunctionName(nameBytes), newNode)
		}
		n = found
	}
	n.self += int64(value)
	return n, offset, nil
}

const OtherLocationRef = LocationRefName(-1)

type LocationRefName int

type LocationRefNameI struct {
}

func (LocationRefNameI) IsLocationTree() bool {
	return true
}

func (LocationRefNameI) newOther() LocationRefName { //nolint:unused
	return OtherLocationRef
}

func (LocationRefNameI) marshalNode(w io.Writer, vw varint.Writer, n *node[LocationRefName], keepName func(LocationRefName) LocationRefName) error { //nolint:unused
	if _, err := vw.Write(w, uint64(keepName(n.name))); err != nil {
		return err
	}
	_, err := vw.Write(w, uint64(n.self))
	return err
}

func (LocationRefNameI) unmarshalNode(b []byte, offset int) (LocationRefName, int64, int, error) { //nolint:unused
	name, o := varint.Uvarint(b[offset:])
	if o < 0 {
		return 0, 0, 0, errMalformedTreeBytes
	}
	offset += o

	value, o := varint.Uvarint(b[offset:])
	if o < 0 {
		return 0, 0, 0, errMalformedTreeBytes
	}
	offset += o

	return LocationRefName(name), int64(value), offset, nil
}

func (LocationRefNameI) mergeNode(parent *node[LocationRefName], b []byte, offset int, format func(LocationRefName) LocationRefName, newNode func() *node[LocationRefName]) (*node[LocationRefName], int, error) { //nolint:unused
	name64, o := varint.Uvarint(b[offset:])
	if o < 0 {
		return nil, 0, errMalformedTreeBytes
	}
	offset += o
	value, o := varint.Uvarint(b[offset:])
	if o < 0 {
		return nil, 0, errMalformedTreeBytes
	}
	offset += o

	name := LocationRefName(name64)
	if format != nil {
		name = format(name)
	}
	n := parent.insert(name, newNode)
	n.self += int64(value)
	return n, offset, nil
}

type NodeName interface {
	~string | ~int
}

type NodeNameI[N ~string | ~int] interface {
	IsLocationTree() bool
	newOther() N
	marshalNode(io.Writer, varint.Writer, *node[N], func(N) N) error
	unmarshalNode([]byte, int) (N, int64, int, error)
	// mergeNode parses the node record at the given offset and merges it
	// into the parent's children: the node's self value is added to the
	// existing child with the same (optionally formatted) name, or a new
	// child is inserted. Returns the merged node and the next offset.
	mergeNode(*node[N], []byte, int, func(N) N, func() *node[N]) (*node[N], int, error)
}

func unsafeString(b []byte) string { //nolint:unused
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(&b[0], len(b))
}

type LocationRefNameTree = Tree[LocationRefName, LocationRefNameI]

type FunctionNameTree = Tree[FunctionName, FunctionNameI]

type Tree[N NodeName, I NodeNameI[N]] struct {
	root []*node[N]
}

type node[N NodeName] struct {
	parent      *node[N]
	children    []*node[N]
	self, total int64
	name        N
}

func (t *Tree[N, I]) String() string {
	type branch struct {
		nodes []*node[N]
		treeprint.Tree
	}
	tree := treeprint.New()
	for _, n := range t.root {
		b := tree.AddBranch(fmt.Sprintf("%v: self %d total %d", n.name, n.self, n.total))
		remaining := append([]*branch{}, &branch{nodes: n.children, Tree: b})
		for len(remaining) > 0 {
			current := remaining[0]
			remaining = remaining[1:]
			for _, n := range current.nodes {
				if len(n.children) > 0 {
					remaining = append(remaining, &branch{nodes: n.children, Tree: current.AddBranch(fmt.Sprintf("%v: self %d total %d", n.name, n.self, n.total))})
				} else {
					current.AddNode(fmt.Sprintf("%v: self %d total %d", n.name, n.self, n.total))
				}
			}
		}
	}
	return tree.String()
}

func (t *Tree[N, I]) Total() (v int64) {
	for _, n := range t.root {
		v += n.total
	}
	return v
}

func (t *Tree[N, I]) InsertStack(v int64, stack ...N) {
	if v <= 0 {
		return
	}
	r := &node[N]{children: t.root}
	n := r
	for s := range stack {
		name := stack[s]
		n.total += v
		// Inlined node.insert
		i, j := 0, len(n.children)
		for i < j {
			h := int(uint(i+j) >> 1)
			if n.children[h].name < name {
				i = h + 1
			} else {
				j = h
			}
		}
		if i < len(n.children) && n.children[i].name == name {
			n = n.children[i]
		} else {
			child := &node[N]{parent: n, name: name}
			n.children = append(n.children, child)
			copy(n.children[i+1:], n.children[i:])
			n.children[i] = child
			n = child
		}
	}
	// Leaf.
	n.total += v
	n.self += v
	t.root = r.children
}

func (t *Tree[N, I]) WriteCollapsed(dst io.Writer) {
	t.IterateStacks(func(_ N, self int64, stack []N) {
		slices.Reverse(stack)
		stackStrs := make([]string, len(stack))
		for i, v := range stack {
			stackStrs[i] = fmt.Sprint(v)
		}
		_, _ = fmt.Fprintf(dst, "%s %d\n", strings.Join(stackStrs, ";"), self)
	})
}

func (t *Tree[N, I]) IterateStacks(cb func(name N, self int64, stack []N)) {
	s := 1024
	if s < len(t.root) {
		s += len(t.root)
	}
	nodes := make([]*node[N], len(t.root), s)
	stack := make([]N, 0, 64)
	copy(nodes, t.root)
	for len(nodes) > 0 {
		n := nodes[0]
		self := n.self
		label := n.name
		if self > 0 {
			current := n
			stack = stack[:0]
			for current != nil && current.parent != nil {
				stack = append(stack, current.name)
				current = current.parent
			}
			cb(label, self, stack)
		}
		nodes = nodes[1:]
		nodes = append(nodes, n.children...)
	}
}

// Default Depth First Search slice capacity. The value should be equal
// to the number of all the siblings of the tree leaf ascendants.
//
// Chosen empirically. For very deep stacks (>128), it's likely that the
// slice will grow to 1-4K nodes, depending on the trace branching.
const defaultDFSSize = 128

func (t *Tree[N, I]) Merge(src *Tree[N, I]) {
	if t.Total() == 0 && src.Total() > 0 {
		*t = *src
		return
	}
	if src.Total() == 0 {
		return
	}

	nodeBuffer := newNodeBuffer[N](defaultDFSSize)

	srcNodes := make([]*node[N], 0, defaultDFSSize)
	srcRoot := &node[N]{children: src.root}
	srcNodes = append(srcNodes, srcRoot)

	dstNodes := make([]*node[N], 0, defaultDFSSize)
	dstRoot := &node[N]{children: t.root}
	dstNodes = append(dstNodes, dstRoot)

	var st, dt *node[N]
	for len(srcNodes) > 0 {
		st, srcNodes = srcNodes[len(srcNodes)-1], srcNodes[:len(srcNodes)-1]
		dt, dstNodes = dstNodes[len(dstNodes)-1], dstNodes[:len(dstNodes)-1]

		dt.self += st.self
		dt.total += st.total

		for _, srcChildNode := range st.children {
			// Note that we don't copy the name, but reference it.
			dstChildNode := dt.insert(srcChildNode.name, nodeBuffer.newNode)
			srcNodes = append(srcNodes, srcChildNode)
			dstNodes = append(dstNodes, dstChildNode)
		}
	}

	t.root = dstRoot.children
}

func (t *Tree[N, I]) FormatNodeNames(fn func(N) N) {
	nodes := make([]*node[N], 0, defaultDFSSize)
	nodes = append(nodes, &node[N]{children: t.root})
	var n *node[N]
	var fix bool
	for len(nodes) > 0 {
		n, nodes = nodes[len(nodes)-1], nodes[:len(nodes)-1]
		m := n.name
		n.name = fn(m)
		if m != n.name {
			fix = true
		}
		nodes = append(nodes, n.children...)
	}
	if !fix {
		return
	}
	t.Fix()
}

// Fix re-establishes order of nodes and merges duplicates.
func (t *Tree[N, I]) Fix() {
	if len(t.root) == 0 {
		return
	}
	r := &node[N]{children: t.root}
	for _, n := range r.children {
		n.parent = r
	}
	nodes := make([][]*node[N], 0, defaultDFSSize)
	nodes = append(nodes, r.children)
	var n []*node[N]
	for len(nodes) > 0 {
		n, nodes = nodes[len(nodes)-1], nodes[:len(nodes)-1]
		if len(n) == 0 {
			continue
		}
		sort.Slice(n, func(i, j int) bool {
			return n[i].name < n[j].name
		})
		p := n[0]
		j := 1
		for _, c := range n[1:] {
			if p.name == c.name {
				for _, x := range c.children {
					x.parent = p
				}
				p.children = append(p.children, c.children...)
				p.total += c.total
				p.self += c.self
				continue
			}
			p = c
			n[j] = c
			j++
		}
		n = n[:j]
		for _, c := range n {
			c.parent.children = n
			nodes = append(nodes, c.children)
		}
	}
	t.root = r.children
}

func (n *node[N]) String() string {
	return fmt.Sprintf("{%v: self %d total %d}", n.name, n.self, n.total)
}

func (n *node[N]) insert(name N, newNode func() *node[N]) *node[N] {
	i, child := n.find(name)
	if child != nil {
		return child
	}
	return n.insertAt(i, name, newNode)
}

// find locates the child with the given name, returning the position where
// it is, or where it would be inserted, and the child itself (nil if absent).
func (n *node[N]) find(name N) (int, *node[N]) {
	i := sort.Search(len(n.children), func(i int) bool {
		return n.children[i].name >= name
	})
	if i < len(n.children) && n.children[i].name == name {
		return i, n.children[i]
	}
	return i, nil
}

func (n *node[N]) insertAt(i int, name N, newNode func() *node[N]) *node[N] {
	// We don't clone the name: it is caller responsibility
	// to maintain the memory ownership.
	var child *node[N]
	if newNode == nil {
		child = &node[N]{}
	} else {
		child = newNode()
	}
	child.parent = n
	child.name = name
	n.children = append(n.children, child)
	copy(n.children[i+1:], n.children[i:])
	n.children[i] = child
	return child
}

// minValue returns the minimum "total" value a node in a tree has to have to show up in
// the resulting flamegraph
func (t *Tree[N, I]) minValue(maxNodes int64) int64 {
	if maxNodes < 1 {
		return 0
	}
	nodes := make([]*node[N], 0, max(int64(len(t.root)), defaultDFSSize))
	treeSize := t.size(nodes)
	if treeSize <= maxNodes {
		return 0
	}

	h := make([]int64, 0, maxNodes)

	nodes = append(nodes[:0], t.root...)
	var n *node[N]
	for len(nodes) > 0 {
		last := len(nodes) - 1
		n, nodes = nodes[last], nodes[:last]
		if len(h) >= int(maxNodes) {
			if n.total > h[0] {
				h = minheap.Pop(h)
			} else {
				continue
			}
		}
		h = minheap.Push(h, n.total)
		nodes = append(nodes, n.children...)
	}

	if len(h) < int(maxNodes) {
		return 0
	}

	return h[0]
}

// size reports number of nodes the tree consists of.
// Provided buffer used for DFS traversal.
func (t *Tree[N, I]) size(buf []*node[N]) int64 {
	nodes := append(buf, t.root...)
	var s int64
	var n *node[N]
	for len(nodes) > 0 {
		last := len(nodes) - 1
		n, nodes = nodes[last], nodes[:last]
		nodes = append(nodes, n.children...)
		s++
	}
	return s
}

const truncatedNodeName = "other"

var truncatedNodeNameBytes = []byte(truncatedNodeName)

// Bytes returns marshaled tree byte representation; the number of nodes
// is limited to maxNodes. The function modifies the tree: truncated nodes
// are removed from the tree in place.
func (t *Tree[N, I]) Bytes(maxNodes int64, keepName func(N) N) []byte {
	var buf bytes.Buffer
	_ = t.MarshalTruncate(&buf, maxNodes, keepName)
	return buf.Bytes()
}

// MarshalTruncate writes tree byte representation to the writer provider,
// the number of nodes is limited to maxNodes. The function modifies
// the tree: truncated nodes are removed from the tree.
func (t *Tree[N, I]) MarshalTruncate(w io.Writer, maxNodes int64, keepName func(N) N) (err error) {
	if len(t.root) == 0 {
		return nil
	}

	var initializer I
	otherName := initializer.newOther()

	vw := varint.NewWriter()
	minVal := t.minValue(maxNodes)
	nodes := make([]*node[N], 1, defaultDFSSize)
	nodes[0] = &node[N]{children: t.root} // Virtual root node.
	var n *node[N]

	for len(nodes) > 0 {
		last := len(nodes) - 1
		n, nodes = nodes[last], nodes[:last]

		if err := initializer.marshalNode(w, vw, n, keepName); err != nil {
			return err
		}

		var other int64
		var j int
		for _, cn := range n.children {
			if cn.total >= minVal || cn.name == otherName {
				n.children[j] = cn
				j++
			} else {
				other += cn.total
			}
		}

		n.children = n.children[:j]
		if other > 0 {
			o := n.insert(otherName, nil)
			o.total += other
			o.self += other
		}

		if len(n.children) > 0 {
			nodes = append(nodes, n.children...)
		}
		if _, err = vw.Write(w, uint64(len(n.children))); err != nil {
			return err
		}
	}

	return nil
}

var errMalformedTreeBytes = fmt.Errorf("malformed tree bytes")

const estimateBytesPerNode = 16 // Chosen empirically.

func MustUnmarshalTree[N NodeName, I NodeNameI[N]](b []byte) *Tree[N, I] {
	if len(b) == 0 {
		return new(Tree[N, I])
	}
	t, err := UnmarshalTree[N, I](b)
	if err != nil {
		panic(err)
	}
	return t
}

// nodeBuffer arena-allocates nodes in chunks. Chunks grow geometrically from
// the initial size hint and are capped: the hint is derived from the input
// size and may overestimate the node count severalfold, so allocating (and
// zeroing) it all upfront wastes more memory than it saves in malloc calls.
const maxNodeBufferChunk = 8 << 10

type nodeBuffer[N NodeName] struct {
	chunk int
	nodes []node[N]
}

func newNodeBuffer[N NodeName](size int) *nodeBuffer[N] {
	return &nodeBuffer[N]{
		chunk: min(max(size, 64), maxNodeBufferChunk),
	}
}

func (nb *nodeBuffer[N]) newNode() *node[N] {
	if len(nb.nodes) == 0 {
		nb.nodes = make([]node[N], nb.chunk)
		nb.chunk = min(nb.chunk*2, maxNodeBufferChunk)
	}
	n := &nb.nodes[0]
	nb.nodes = nb.nodes[1:]
	return n
}

func UnmarshalTree[N NodeName, I NodeNameI[N]](b []byte) (*Tree[N, I], error) {
	var initializer I
	t := new(Tree[N, I])
	if len(b) < 2 {
		return t, nil
	}
	size := estimateBytesPerNode
	if e := len(b) / estimateBytesPerNode; e > estimateBytesPerNode {
		size = e
	}
	parents := make([]*node[N], 1, min(size, 4<<10))
	// Virtual root node.
	root := new(node[N])
	parents[0] = root
	var parent *node[N]
	var offset int

	nodeBuffer := newNodeBuffer[N](size)

	var records int
	for len(parents) > 0 {
		parent, parents = parents[len(parents)-1], parents[:len(parents)-1]
		// specific start

		name, value, o, err := initializer.unmarshalNode(b, offset)
		if err != nil {
			return nil, err
		}
		offset = o

		// specific end
		childrenLen, o := varint.Uvarint(b[offset:])
		if o < 0 {
			return nil, errMalformedTreeBytes
		}
		offset += o

		n := parent.insert(name, nodeBuffer.newNode)
		n.children = make([]*node[N], 0, childrenLen)
		n.self = value
		records++

		for i := uint64(0); i < childrenLen; i++ {
			parents = append(parents, n)
		}
	}

	// Remove the virtual root.
	t.root = root.children[0].children
	t.recomputeTotals(records)

	return t, nil
}

// mergeBytes merges a marshaled tree directly into t, without materializing
// an intermediate tree. Node totals are NOT maintained: the caller must
// recompute them (see recomputeTotals) before the totals are read.
// Returns the number of node records merged, which bounds the tree growth.
// On a malformed input an error is returned and t may be partially merged.
func (t *Tree[N, I]) mergeBytes(b []byte, format func(N) N) (int, error) {
	var initializer I
	if len(b) < 2 {
		return 0, nil
	}
	// The stream starts with a virtual root record; its name and value
	// are ignored, only the children count matters.
	_, _, offset, err := initializer.unmarshalNode(b, 0)
	if err != nil {
		return 0, err
	}
	childrenLen, o := varint.Uvarint(b[offset:])
	if o < 0 {
		return 0, errMalformedTreeBytes
	}
	offset += o

	root := &node[N]{children: t.root}
	parents := make([]*node[N], 0, defaultDFSSize)
	for i := uint64(0); i < childrenLen; i++ {
		parents = append(parents, root)
	}

	// Most records of a merged tree are expected to hit existing nodes,
	// so the arena starts small and grows on demand.
	nodeBuffer := newNodeBuffer[N](256)

	var records int
	var parent *node[N]
	for len(parents) > 0 {
		parent, parents = parents[len(parents)-1], parents[:len(parents)-1]

		n, next, err := initializer.mergeNode(parent, b, offset, format, nodeBuffer.newNode)
		if err != nil {
			return records, err
		}
		offset = next
		records++

		childrenLen, o = varint.Uvarint(b[offset:])
		if o < 0 {
			return records, errMalformedTreeBytes
		}
		offset += o

		if childrenLen > 0 {
			if cap(n.children) == 0 {
				n.children = make([]*node[N], 0, childrenLen)
			}
			for i := uint64(0); i < childrenLen; i++ {
				parents = append(parents, n)
			}
		}
	}

	t.root = root.children
	return records, nil
}

// recomputeTotals derives every node's total from the self values of its
// subtree in a single pass: in reverse level order all children of a node
// are visited before the node itself. The size hint pre-allocates the
// traversal buffer; it does not have to be exact.
func (t *Tree[N, I]) recomputeTotals(sizeHint int) {
	if len(t.root) == 0 {
		return
	}
	order := make([]*node[N], 0, max(sizeHint, 1024))
	order = append(order, t.root...)
	for i := 0; i < len(order); i++ {
		order = append(order, order[i].children...)
	}
	for i := len(order) - 1; i >= 0; i-- {
		n := order[i]
		n.total = n.self
		for _, c := range n.children {
			n.total += c.total
		}
	}
}

// TreeFromBackendProfile is a wrapper...
func TreeFromBackendProfile(profile *profilev1.Profile, maxNodes int64) ([]byte, error) {
	return TreeFromBackendProfileSampleType(profile, maxNodes, 0)
}

// TreeFromBackendProfileSampleType converts a pprof profile to a tree format with maxNodes limit
func TreeFromBackendProfileSampleType(profile *profilev1.Profile, maxNodes int64, sampleType int) ([]byte, error) {
	t := NewStacktraceTree(int(maxNodes * 2))
	stack := make([]int32, 0, 64)
	m := make(map[uint64]int32)

	for i := range profile.Sample {
		stack = stack[:0]
		for j := range profile.Sample[i].LocationId {
			locIdx := int(profile.Sample[i].LocationId[j]) - 1
			if locIdx < 0 || len(profile.Location) <= locIdx {
				return nil, fmt.Errorf("invalid location ID %d in sample %d", profile.Sample[i].LocationId[j], i)
			}

			loc := profile.Location[locIdx]
			if len(loc.Line) > 0 {
				for l := range loc.Line {
					stack = append(stack, int32(profile.Function[loc.Line[l].FunctionId-1].Name))
				}
				continue
			}
			addr, ok := m[loc.Address]
			if !ok {
				addr = int32(len(profile.StringTable))
				profile.StringTable = append(profile.StringTable, strconv.FormatInt(int64(loc.Address), 16))
				m[loc.Address] = addr
			}
			stack = append(stack, addr)
		}

		if sampleType < 0 || sampleType >= len(profile.Sample[i].Value) {
			return nil, fmt.Errorf("invalid sampleType index %d for sample %d (len=%d)", sampleType, i, len(profile.Sample[i].Value))
		}

		t.Insert(stack, profile.Sample[i].Value[sampleType])
	}

	b := bytes.NewBuffer(nil)
	b.Grow(100 << 10)
	t.Bytes(b, maxNodes, profile.StringTable)
	return b.Bytes(), nil
}

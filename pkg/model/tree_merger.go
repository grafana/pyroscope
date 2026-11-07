package model

import (
	"cmp"
	"slices"
	"sync"

	"github.com/grafana/pyroscope/v2/pkg/og/util/varint"
)

type TreeMerger[N NodeName, I NodeNameI[N]] struct {
	mu sync.Mutex
	t  *Tree[N, I]
}

func NewTreeMerger[N NodeName, I NodeNameI[N]]() *TreeMerger[N, I] {
	return new(TreeMerger[N, I])
}

func (m *TreeMerger[N, I]) MergeTree(t *Tree[N, I]) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.t != nil {
		m.t.Merge(t)
	} else {
		m.t = t
	}
}

type treeMergeOption[N any] struct {
	formatNodeNames func(N) N
}

type TreeMergeOption[N any] func(o *treeMergeOption[N])

func WithTreeMergeFormatNodeNames[N any](f func(N) N) TreeMergeOption[N] {
	return func(o *treeMergeOption[N]) {
		o.formatNodeNames = f
	}
}

// MergeTreeBytes prepares a flat source tree outside the merge lock, then
// inserts its nodes without constructing an intermediate pointer tree.
func (m *TreeMerger[N, I]) MergeTreeBytes(b []byte, opts ...TreeMergeOption[N]) error {
	var o treeMergeOption[N]
	for _, f := range opts {
		f(&o)
	}
	nodes, compatible, err := prepareTreeBytes[N, I](b, o.formatNodeNames != nil)
	if err != nil {
		return err
	}
	if !compatible {
		// Preserve the decoder's behavior for duplicate siblings, and the
		// formatter's sorted traversal order for out-of-order encodings.
		t, err := UnmarshalTree[N, I](b)
		if err != nil {
			return err
		}
		if o.formatNodeNames != nil {
			t.FormatNodeNames(o.formatNodeNames)
		}
		m.MergeTree(t)
		return nil
	}
	if o.formatNodeNames != nil {
		// FormatNodeNames also visits its synthetic root. Its formatted name is
		// discarded; preserve the callback for callers using the existing option.
		var rootName N
		o.formatNodeNames(rootName)
		for i := 1; i < len(nodes); i++ {
			nodes[i].name = o.formatNodeNames(nodes[i].name)
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(nodes) == 0 {
		if m.t == nil {
			m.t = new(Tree[N, I])
		}
		return nil
	}
	total := nodes[0].total
	if m.t != nil {
		if total == 0 {
			return nil
		}
		if m.t.Total() == 0 && total > 0 {
			m.t = nil
		}
	}
	if m.t == nil {
		m.t = new(Tree[N, I])
	}
	root := &node[N]{children: m.t.root}
	type frame struct {
		parent    *node[N]
		remaining int
	}
	parents := make([]frame, 0, min(len(nodes), defaultDFSSize))
	if nodes[0].children > 0 {
		parents = append(parents, frame{root, nodes[0].children})
	}
	buffer := newNodeBuffer[N](defaultDFSSize)
	for _, source := range nodes[1:] {
		last := len(parents) - 1
		parent := parents[last].parent
		parents[last].remaining--
		if parents[last].remaining == 0 {
			parents = parents[:last]
		}
		n := parent.insert(source.name, buffer.newNode)
		n.self += source.self
		n.total += source.total
		if source.children > 0 {
			parents = append(parents, frame{n, source.children})
		}
	}
	m.t.root = root.children
	return nil
}

type preparedTreeNode[N NodeName] struct {
	name        N
	self, total int64
	children    int
}

func prepareTreeBytes[N NodeName, I NodeNameI[N]](b []byte, preserveOrder bool) ([]preparedTreeNode[N], bool, error) {
	if len(b) == 0 {
		return nil, true, nil
	}
	count, err := countTreeNodes[N, I](b)
	if err != nil {
		return nil, false, err
	}
	nodes := make([]preparedTreeNode[N], count)
	var decoder I
	var offset int
	for i := range nodes {
		name, self, next, err := decoder.unmarshalNode(b, offset)
		if err != nil {
			return nil, false, err
		}
		offset = next
		children, width := varint.Uvarint(b[offset:])
		offset += width
		nodes[i] = preparedTreeNode[N]{name: name, self: self, total: self, children: int(children)}
	}
	// Serialized nodes are preorder. In reverse order, each child's complete
	// subtree is on the stack before its parent. Every edge is visited once.
	nodes[0].self, nodes[0].total = 0, 0 // The encoded virtual root is not a sample.
	pending := make([]int, 0, min(count, defaultDFSSize))
	for i := len(nodes) - 1; i >= 0; i-- {
		// The stack contains the child roots in reverse wire order. Tree.Bytes
		// normally emits descending names; other writers need not. Sorting only
		// this scratch slice detects duplicates without a map per parent.
		children := pending[len(pending)-nodes[i].children:]
		ordered := true
		for c := 1; c < len(children); c++ {
			if nodes[children[c-1]].name >= nodes[children[c]].name {
				ordered = false
				break
			}
		}
		if !ordered {
			if preserveOrder {
				return nil, false, nil
			}
			slices.SortFunc(children, func(a, b int) int { return cmp.Compare(nodes[a].name, nodes[b].name) })
			for c := 1; c < len(children); c++ {
				if nodes[children[c-1]].name == nodes[children[c]].name {
					return nil, false, nil
				}
			}
		}
		for c := 0; c < nodes[i].children; c++ {
			child := nodes[pending[len(pending)-1]]
			nodes[i].total += child.total
			pending = pending[:len(pending)-1]
		}
		pending = append(pending, i)
	}
	return nodes, true, nil
}

// Count and validate before allocating names/nodes or mutating the aggregate.
// Each pending node requires at least three bytes (name, self, child count),
// bounding child counts and additions by the remaining input, not wire values.
func countTreeNodes[N NodeName, I NodeNameI[N]](b []byte) (int, error) {
	var decoder I
	remaining, count, offset := uint64(1), 0, 0
	for remaining > 0 {
		name, width := varint.Uvarint(b[offset:])
		if width <= 0 {
			return 0, errMalformedTreeBytes
		}
		offset += width
		if !decoder.IsLocationTree() {
			if name > uint64(len(b)-offset) {
				return 0, errMalformedTreeBytes
			}
			offset += int(name)
		}
		_, width = varint.Uvarint(b[offset:])
		if width <= 0 {
			return 0, errMalformedTreeBytes
		}
		offset += width
		children, width := varint.Uvarint(b[offset:])
		if width <= 0 {
			return 0, errMalformedTreeBytes
		}
		offset += width
		count++
		remaining--
		available := uint64((len(b) - offset) / 3)
		if children > available || remaining > available-children {
			return 0, errMalformedTreeBytes
		}
		remaining += children
	}
	if offset != len(b) {
		return 0, errMalformedTreeBytes
	}
	return count, nil
}

func (m *TreeMerger[N, I]) Tree() *Tree[N, I] {
	if m.t == nil {
		return new(Tree[N, I])
	}
	return m.t
}

func (m *TreeMerger[N, I]) IsEmpty() bool {
	return m.t == nil
}

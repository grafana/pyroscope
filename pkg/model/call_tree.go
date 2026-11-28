package model

import (
	"cmp"
	"slices"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

// callTreeBuilder projects samples before constructing a tree. It only allocates
// output nodes within the requested depth; matching still examines the input stacks.
// Unlike Tree, it preserves exact totals and child-existence metadata without allocating deeper nodes.
type callTreeBuilder[N comparable] struct {
	lookup func(N) string
	path   []N
	depth  int
	insert func([]N, int64)
	root   *callTreeNode[N]
}

type callTreeNode[N comparable] struct {
	name        N
	total       int64
	self        int64
	hasChildren bool
	children    map[N]*callTreeNode[N]
}

func newCallTreeBuilder[N comparable](path []N, depth int, direction typesv1.FunctionTreeDirection, selection typesv1.FunctionTreeSelection, lookup func(N) string) *callTreeBuilder[N] {
	b := &callTreeBuilder[N]{
		lookup: lookup,
		path:   slices.Clone(path),
		depth:  depth,
	}
	switch {
	case direction == typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS:
		// Selectors are root-first; inverted traversal walks the chain backwards.
		slices.Reverse(b.path)
		b.insert = b.insertCallers
	case selection == typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN:
		b.insert = b.insertChainCallees
	default:
		b.insert = b.insertSelectedCallees
	}
	return b
}

// Insert accepts a root-first stack. Function-relative chains count each
// matching occurrence. ROOT_PATH stacks must already be selected by the caller.
func (b *callTreeBuilder[N]) Insert(stack []N, value int64) {
	if value > 0 && len(stack) >= len(b.path) {
		b.insert(stack, value)
	}
}

func (b *callTreeBuilder[N]) insertChainCallees(stack []N, value int64) {
	for start := 0; start+len(b.path) <= len(stack); start++ {
		if slices.Equal(stack[start:start+len(b.path)], b.path) {
			b.insertCallees(stack, start+len(b.path), value)
		}
	}
}

func (b *callTreeBuilder[N]) insertSelectedCallees(stack []N, value int64) {
	b.insertCallees(stack, len(b.path), value)
}

func (b *callTreeBuilder[N]) insertCallees(stack []N, next int, value int64) {
	n := b.reference()
	n.total += value
	if len(stack) == next {
		n.self += value
	}
	for i, depth := next, 0; i < len(stack); i, depth = i+1, depth+1 {
		n.hasChildren = true
		if depth == b.depth {
			break
		}
		n = n.child(stack[i])
		n.total += value
		if i == len(stack)-1 {
			n.self += value
		}
	}
}

func (b *callTreeBuilder[N]) reference() *callTreeNode[N] {
	if b.root == nil {
		b.root = &callTreeNode[N]{}
		if len(b.path) > 0 {
			b.root.name = b.path[len(b.path)-1]
		}
	}
	return b.root
}

func (b *callTreeBuilder[N]) insertCallers(stack []N, value int64) {
	if len(b.path) == 0 {
		return
	}
	for anchor := len(b.path) - 1; anchor < len(stack); anchor++ {
		matches := true
		for offset, name := range b.path {
			if stack[anchor-offset] != name {
				matches = false
				break
			}
		}
		if !matches {
			continue
		}
		// Each occurrence contributes separately, including recursive occurrences.
		// The output root is the last selected caller, so traversal can extend path.
		reference := anchor - len(b.path) + 1
		n := b.reference()
		n.total += value
		if reference == len(stack)-1 {
			n.self += value
		}
		for i, depth := reference-1, 0; i >= 0; i, depth = i-1, depth+1 {
			n.hasChildren = true
			if depth == b.depth {
				break
			}
			n = n.child(stack[i])
			n.total += value
		}
	}
}

func (n *callTreeNode[N]) child(name N) *callTreeNode[N] {
	if n.children == nil {
		n.children = make(map[N]*callTreeNode[N])
	}
	if child := n.children[name]; child != nil {
		return child
	}
	child := &callTreeNode[N]{name: name}
	n.children[name] = child
	return child
}

func (b *callTreeBuilder[N]) Tree() *typesv1.CallTree {
	return &typesv1.CallTree{Root: b.root.tree(b.lookup)}
}

func (n *callTreeNode[N]) tree(lookup func(N) string) *typesv1.CallTreeNode {
	if n == nil {
		return nil
	}
	out := &typesv1.CallTreeNode{
		Name:        lookup(n.name),
		Total:       n.total,
		Self:        n.self,
		HasChildren: n.hasChildren,
	}
	for _, child := range n.children {
		out.Children = append(out.Children, child.tree(lookup))
	}
	slices.SortFunc(out.Children, func(a, b *typesv1.CallTreeNode) int {
		if c := cmp.Compare(b.Total, a.Total); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})
	return out
}

// callTreeMerger sums projections at the same reference and depth. Projecting
// before merging is lossless because no sibling is removed by value or rank.
// Its owner, FunctionTreeMerger, serializes access to both directions.
type callTreeMerger struct {
	root *callTreeNode[string]
}

func (m *callTreeMerger) merge(tree *typesv1.CallTree) {
	if tree == nil || tree.Root == nil {
		return
	}
	if m.root == nil {
		m.root = &callTreeNode[string]{name: tree.Root.Name}
	}
	mergeCallTreeNode(m.root, tree.Root)
}

func mergeCallTreeNode(n *callTreeNode[string], src *typesv1.CallTreeNode) {
	n.total += src.Total
	n.self += src.Self
	n.hasChildren = n.hasChildren || src.HasChildren
	for _, c := range src.Children {
		mergeCallTreeNode(n.child(c.Name), c)
	}
}

func (m *callTreeMerger) tree() *typesv1.CallTree {
	return &typesv1.CallTree{Root: m.root.tree(func(name string) string {
		return name
	})}
}

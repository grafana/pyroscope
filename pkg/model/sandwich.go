package model

import (
	"cmp"
	"context"
	"slices"
	"sync"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	"github.com/grafana/pyroscope/v2/pkg/util/minheap"
)

// sandwichTrie is one half of a sandwich while it is being built. Children are
// keyed by name so that occurrences merge as they arrive, and flattened on the
// way out.
type sandwichTrie struct {
	name  FunctionName
	total int64
	self  int64
	// Set only on a stand-in this package created, so that a real function
	// called 'other' is never reported as one.
	truncated bool
	children  map[FunctionName]*sandwichTrie
}

func (t *sandwichTrie) child(name FunctionName) *sandwichTrie {
	if t.children == nil {
		t.children = make(map[FunctionName]*sandwichTrie)
	}
	c := t.children[name]
	if c == nil {
		c = &sandwichTrie{name: name}
		t.children[name] = c
	}
	return c
}

func (t *sandwichTrie) size() int64 {
	var n int64
	stack := []*sandwichTrie{t}
	for len(stack) > 0 {
		last := len(stack) - 1
		cur := stack[last]
		stack = stack[:last]
		n++
		for _, c := range cur.children {
			stack = append(stack, c)
		}
	}
	return n
}

// minValue mirrors Tree.minValue: the smallest total a node must have to
// survive truncation to maxNodes, so that an 'other' node here means what it
// means everywhere else.
func (t *sandwichTrie) minValue(maxNodes int64) int64 {
	if maxNodes < 1 || t.size() <= maxNodes {
		return 0
	}
	h := make([]int64, 0, maxNodes)
	stack := []*sandwichTrie{t}
	for len(stack) > 0 {
		last := len(stack) - 1
		cur := stack[last]
		stack = stack[:last]
		if int64(len(h)) >= maxNodes {
			if cur.total > h[0] {
				h = minheap.Pop(h)
			} else {
				continue
			}
		}
		h = minheap.Push(h, cur.total)
		for _, c := range cur.children {
			stack = append(stack, c)
		}
	}
	if int64(len(h)) < maxNodes {
		return 0
	}
	return h[0]
}

// truncate drops every node below minVal, collapsing the dropped children of
// each surviving node into a single stand-in. The root always survives.
func (t *sandwichTrie) truncate(maxNodes int64) {
	minVal := t.minValue(maxNodes)
	if minVal == 0 {
		return
	}
	stack := []*sandwichTrie{t}
	for len(stack) > 0 {
		last := len(stack) - 1
		cur := stack[last]
		stack = stack[:last]
		var dropped int64
		for name, c := range cur.children {
			if c.total < minVal && name != OtherFunctionName {
				dropped += c.total
				delete(cur.children, name)
			}
		}
		if dropped > 0 {
			o := cur.child(OtherFunctionName)
			o.total += dropped
			o.self += dropped
			o.truncated = true
			o.children = nil
		}
		for _, c := range cur.children {
			if !c.truncated {
				stack = append(stack, c)
			}
		}
	}
}

// nameTable interns the function names of one report, so that a name repeated
// across nodes is carried once.
type nameTable struct {
	index map[FunctionName]int32
	names []string
}

func (t *nameTable) id(name FunctionName) int32 {
	if t.index == nil {
		t.index = make(map[FunctionName]int32)
	}
	if i, ok := t.index[name]; ok {
		return i
	}
	i := int32(len(t.names))
	t.index[name] = i
	t.names = append(t.names, string(name))
	return i
}

func (t *sandwichTrie) proto(names *nameTable) *querierv1.SandwichNode {
	n := &querierv1.SandwichNode{
		NameIndex: names.id(t.name),
		Total:     t.total,
		Self:      t.self,
		Truncated: t.truncated,
	}
	if len(t.children) == 0 {
		return n
	}
	children := make([]*sandwichTrie, 0, len(t.children))
	for _, c := range t.children {
		children = append(children, c)
	}
	// Deterministic, and the order a pane would want anyway. Sorted before
	// encoding so that the name table is built in a stable order too.
	slices.SortFunc(children, func(a, b *sandwichTrie) int {
		if c := cmp.Compare(b.total, a.total); c != 0 {
			return c
		}
		return cmp.Compare(a.name, b.name)
	})
	n.Children = make([]*querierv1.SandwichNode, 0, len(children))
	for _, c := range children {
		n.Children = append(n.Children, c.proto(names))
	}
	return n
}

func (t *sandwichTrie) mergeProto(n *querierv1.SandwichNode, names []string) {
	if n == nil {
		return
	}
	t.total += n.Total
	t.self += n.Self
	t.truncated = t.truncated || n.Truncated
	for _, c := range n.Children {
		t.child(nameAt(names, c.NameIndex)).mergeProto(c, names)
	}
}

// nameAt tolerates an index the table does not cover rather than panicking on a
// malformed response from another process.
func nameAt(names []string, i int32) FunctionName {
	if i < 0 || int(i) >= len(names) {
		return ""
	}
	return FunctionName(names[i])
}

// addCallers records one occurrence on the caller half. Every ancestor is
// credited with the occurrence's own value, not with its own total, because a
// caller node here stands for what it contributed to this function. This is
// what mergeParentSubtrees does client side, where each ancestor copy takes the
// value of the child below it.
//
// treeRoots bounds the walk. A tree's top level nodes keep a parent pointer to
// an unnamed node that is not part of the tree (InsertStack builds one per call,
// and the read path nulls its parent rather than the pointers to it), so walking
// to a nil parent would hang an empty name off every caller chain.
func addCallers(root *sandwichTrie, occ *node[FunctionName], treeRoots map[*node[FunctionName]]struct{}) {
	root.total += occ.total
	cur := root
	for p := occ.parent; p != nil; p = p.parent {
		cur = cur.child(p.name)
		cur.total += occ.total
		if _, isRoot := treeRoots[p]; isRoot {
			return
		}
	}
}

// addCallees merges one occurrence's subtree into the callee half, which is
// what mergeSubtrees does client side.
func addCallees(root *sandwichTrie, occ *node[FunctionName]) {
	root.total += occ.total
	root.self += occ.self
	type pair struct {
		src *node[FunctionName]
		dst *sandwichTrie
	}
	stack := []pair{{src: occ, dst: root}}
	for len(stack) > 0 {
		last := len(stack) - 1
		p := stack[last]
		stack = stack[:last]
		for _, c := range p.src.children {
			d := p.dst.child(c.name)
			d.total += c.total
			d.self += c.self
			stack = append(stack, pair{src: c, dst: d})
		}
	}
}

// SandwichFromTree requires an untruncated tree: a truncated one has folded an
// unknown number of the function's occurrences into 'other', and cannot say
// which, so the sandwich it produces is silently incomplete.
//
// Both halves are built from every occurrence of the function, matching
// getSandwichLevels in @grafana/flamegraph. A recursive function is therefore
// present on both sides at several depths, which is the shape the flame graph
// renders today. One consequence is that the root total of a half sums a
// recursive function's occurrences and so double counts its samples;
// SandwichReport.total is the value counted once per sample, for comparing
// against the function table.
func SandwichFromTree(ctx context.Context, tree *FunctionNameTree, function string) (*querierv1.SandwichReport, error) {
	name := FunctionName(function)
	callers := &sandwichTrie{name: name}
	callees := &sandwichTrie{name: name}
	var total, self int64

	treeRoots := make(map[*node[FunctionName]]struct{}, len(tree.root))
	for _, root := range tree.root {
		treeRoots[root] = struct{}{}
	}

	type frame struct {
		node *node[FunctionName]
		next int
	}
	var stack []frame
	// Occurrences of the function already open on the current path.
	var active int

	for _, root := range tree.root {
		stack = append(stack, frame{node: root, next: -1})
		for len(stack) > 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			f := &stack[len(stack)-1]
			n := f.node
			if f.next == -1 {
				if n.name == name {
					self += n.self
					if active == 0 {
						total += n.total
					}
					active++
					addCallers(callers, n, treeRoots)
					addCallees(callees, n)
				}
				f.next = 0
			}
			if f.next < len(n.children) {
				child := n.children[f.next]
				f.next++
				stack = append(stack, frame{node: child, next: -1})
				continue
			}
			if n.name == name {
				active--
			}
			stack = stack[:len(stack)-1]
		}
	}

	var names nameTable
	return &querierv1.SandwichReport{
		Callers: callers.proto(&names),
		Callees: callees.proto(&names),
		Names:   names.names,
		Total:   total,
		Self:    self,
	}, ctx.Err()
}

// SandwichMerger sums complete sandwich reports. It must not receive truncated
// halves: their stand-in nodes would merge as if 'other' were a function, and
// the rows they replaced cannot be recovered. Truncate once, at the end.
type SandwichMerger struct {
	mu      sync.Mutex
	callers *sandwichTrie
	callees *sandwichTrie
	total   int64
	self    int64
}

func (m *SandwichMerger) Merge(report *querierv1.SandwichReport) {
	if report == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.total += report.Total
	m.self += report.Self
	if report.Callers == nil && report.Callees == nil {
		// A dataset with no matching samples. Merging it must not materialise a
		// root for a function that was never seen.
		return
	}
	if m.callers == nil {
		var name FunctionName
		if report.Callers != nil {
			name = nameAt(report.Names, report.Callers.NameIndex)
		} else if report.Callees != nil {
			name = nameAt(report.Names, report.Callees.NameIndex)
		}
		m.callers = &sandwichTrie{name: name}
		m.callees = &sandwichTrie{name: name}
	}
	m.callers.mergeProto(report.Callers, report.Names)
	m.callees.mergeProto(report.Callees, report.Names)
}

// Sandwich returns the merged report, with each half truncated to maxNodes
// independently so that a wide callee side cannot starve the callers. A
// non-positive maxNodes returns both halves whole.
func (m *SandwichMerger) Sandwich(maxNodes int64) *querierv1.SandwichReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.callers == nil {
		return &querierv1.SandwichReport{Total: m.total, Self: m.self}
	}
	m.callers.truncate(maxNodes)
	m.callees.truncate(maxNodes)
	var names nameTable
	return &querierv1.SandwichReport{
		Callers: m.callers.proto(&names),
		Callees: m.callees.proto(&names),
		Names:   names.names,
		Total:   m.total,
		Self:    m.self,
	}
}

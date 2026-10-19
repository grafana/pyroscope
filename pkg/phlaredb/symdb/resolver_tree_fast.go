package symdb

import (
	"github.com/grafana/pyroscope/v2/pkg/model"
)

// functionTree is the call tree of function names built from stack trace
// samples. Nodes are stored in creation order, so a parent always precedes
// its children; node 0 is the root.
type functionTree struct {
	parent   []int32
	name     []int32
	self     []int64
	children childIndex // keyed by parent and function name
}

func newFunctionTree(sizeHint int) *functionTree {
	t := &functionTree{
		parent: make([]int32, 1, sizeHint),
		name:   make([]int32, 1, sizeHint),
		self:   make([]int64, 1, sizeHint),
	}
	t.parent[0] = sentinel
	t.children.reserve(sizeHint)
	return t
}

func (t *functionTree) child(parent, name int32) int32 {
	k := childKey(parent, name)
	if i, ok := t.children.get(k); ok {
		return i
	}
	i := int32(len(t.parent))
	t.parent = append(t.parent, parent)
	t.name = append(t.name, name)
	t.self = append(t.self, 0)
	t.children.set(k, i)
	return i
}

// buildCallTree builds the call tree of function names of the samples by
// mapping each node of the stack trace parent pointer trees to a call tree
// node, instead of resolving and inserting every stack trace frame by frame.
// Each node on a sampled path is visited once, and a location expands into
// its lines (inlined functions), outermost first.
//
// The tree has the same paths and values as the model.StacktraceTree that
// inserting every resolved stack trace would produce: node totals do not
// include the root, whose own value is that of the samples that resolve to
// no function.
//
// The function reports false if the stack traces are not backed by parent
// pointer trees.
func buildCallTree(symbols *Symbols, appender *SampleAppender) (*model.CallTree, bool) {
	iterator, ok := symbols.Stacktraces.(StacktraceIDRangeIterator)
	if !ok || appender.Len() == 0 {
		return nil, false
	}
	var ranges []*StacktraceIDRange
	var nodes int
	for it := iterator.SplitStacktraceIDRanges(appender); it.Next(); {
		r := it.At()
		ppt, ok := r.ParentPointerTree.(*parentPointerTree)
		if !ok {
			return nil, false
		}
		ranges = append(ranges, r)
		nodes += len(ppt.nodes)
	}

	t := newFunctionTree(nodes)
	var fn []int32
	stack := make([]int32, 0, 128)
	for _, r := range ranges {
		ppt := r.ParentPointerTree.(*parentPointerTree).nodes
		fn = growInt32(fn, len(ppt))
		for i := range fn {
			fn[i] = sentinel
		}
		for i, sid := range r.IDs {
			if int(sid) >= len(ppt) {
				// Unresolved stack traces have no frames.
				t.self[0] += int64(r.Values[i])
				continue
			}
			// Collect the unmapped ancestors of the leaf. The node without
			// a parent is the root: it carries no frame.
			stack = stack[:0]
			x := int32(sid)
			for fn[x] == sentinel {
				p := ppt[x].p
				if p < 0 {
					fn[x] = 0
					break
				}
				stack = append(stack, x)
				x = p
			}
			for j := len(stack) - 1; j >= 0; j-- {
				y := stack[j]
				f := fn[ppt[y].p]
				lines := symbols.Locations[ppt[y].r].Line
				for l := len(lines) - 1; l >= 0; l-- {
					f = t.child(f, int32(symbols.Functions[lines[l].FunctionId].Name))
				}
				fn[y] = f
			}
			t.self[fn[sid]] += int64(r.Values[i])
		}
	}

	total := make([]int64, len(t.self))
	copy(total, t.self)
	for i := len(total) - 1; i > 0; i-- {
		total[t.parent[i]] += total[i]
	}
	total[0] = 0
	t.name[0] = 0
	return &model.CallTree{
		Parent: t.parent,
		Name:   t.name,
		Self:   t.self,
		Total:  total,
		Names:  symbols.Strings,
	}, true
}

// stacktraceTreeFromCallTree converts the call tree to the stack trace tree
// of function names, with node values and totals of the call tree.
func stacktraceTreeFromCallTree(c *model.CallTree) *model.StacktraceTree {
	n := len(c.Parent)
	st := &model.StacktraceTree{Nodes: make([]model.StacktraceNode, n)}
	for i := range st.Nodes {
		st.Nodes[i] = model.StacktraceNode{
			FirstChild:  sentinel,
			NextSibling: sentinel,
			Parent:      c.Parent[i],
			Location:    c.Name[i],
			Value:       c.Self[i],
			Total:       c.Total[i],
		}
	}
	st.Nodes[0].Parent = 0
	for i := n - 1; i > 0; i-- {
		p := c.Parent[i]
		st.Nodes[i].NextSibling = st.Nodes[p].FirstChild
		st.Nodes[p].FirstChild = int32(i)
	}
	return st
}

func growInt32(s []int32, n int) []int32 {
	if cap(s) < n {
		return make([]int32, n)
	}
	return s[:n]
}

package model

import (
	"errors"
	"fmt"
	"math"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

var ErrMalformedFunctionStackTree = errors.New("malformed function stack tree")

// FunctionStackTree accumulates untruncated root-first function-name stacks.
// Node 0 is the synthetic root; its self value belongs to stacks without names.
type FunctionStackTree struct {
	names     []string
	nameIDs   map[string]uint32
	nameBytes int
	nodes     []functionStackNode
	children  map[uint64]uint32
}

type functionStackNode struct {
	name        uint32
	children    uint32
	self        uint64
	firstChild  uint32
	nextSibling uint32
}

// Children of wider nodes are indexed in FunctionStackTree.children;
// narrower nodes are scanned, which is cheaper for the common small fan-out.
const functionStackNodeScanLimit = 8

func NewFunctionStackTree() *FunctionStackTree {
	return &FunctionStackTree{
		names:    []string{""},
		nameIDs:  make(map[string]uint32),
		nodes:    make([]functionStackNode, 1),
		children: make(map[uint64]uint32),
	}
}

// Len reports the number of nodes, including the synthetic root.
func (t *FunctionStackTree) Len() int { return len(t.nodes) }

// SizeEstimate approximates the size of the encoded tree from above for
// typical values, without encoding it.
func (t *FunctionStackTree) SizeEstimate() int {
	return t.nameBytes + 3*len(t.names) + 12*len(t.nodes)
}

func (t *FunctionStackTree) init() {
	if len(t.nodes) == 0 {
		*t = *NewFunctionStackTree()
	}
}

func (t *FunctionStackTree) nameID(name string) uint32 {
	id, ok := t.nameIDs[name]
	if !ok {
		id = uint32(len(t.names))
		t.names = append(t.names, name)
		t.nameIDs[name] = id
		t.nameBytes += len(name)
	}
	return id
}

func (t *FunctionStackTree) child(parent, name uint32) uint32 {
	if t.nodes[parent].children > functionStackNodeScanLimit {
		if c, ok := t.children[uint64(parent)<<32|uint64(name)]; ok {
			return c
		}
	} else {
		for c := t.nodes[parent].firstChild; c != 0; c = t.nodes[c].nextSibling {
			if t.nodes[c].name == name {
				return c
			}
		}
	}
	c := uint32(len(t.nodes))
	t.nodes = append(t.nodes, functionStackNode{name: name, nextSibling: t.nodes[parent].firstChild})
	p := &t.nodes[parent]
	p.firstChild = c
	p.children++
	switch {
	case p.children == functionStackNodeScanLimit+1:
		for x := p.firstChild; x != 0; x = t.nodes[x].nextSibling {
			t.children[uint64(parent)<<32|uint64(t.nodes[x].name)] = x
		}
	case p.children > functionStackNodeScanLimit+1:
		t.children[uint64(parent)<<32|uint64(name)] = c
	}
	return c
}

// MergeStacktraceTree adds a stack trace tree whose node locations are
// indices in names. names[0] is reserved for the root and is never used.
// Parents must precede their children, as StacktraceTree.Insert guarantees.
func (t *FunctionStackTree) MergeStacktraceTree(src *StacktraceTree, names []string) {
	if len(src.Nodes) == 0 {
		return
	}
	t.init()
	ids := make([]uint32, len(names))
	dst := make([]uint32, len(src.Nodes))
	t.nodes[0].self += uint64(src.Nodes[0].Value)
	for i := 1; i < len(src.Nodes); i++ {
		n := &src.Nodes[i]
		id := ids[n.Location]
		if id == 0 {
			id = t.nameID(names[n.Location])
			ids[n.Location] = id
		}
		d := t.child(dst[n.Parent], id)
		dst[i] = d
		t.nodes[d].self += uint64(n.Value)
	}
}

// MergeProto adds an encoded tree. On error, t may be partially updated.
func (t *FunctionStackTree) MergeProto(src *queryv1.FunctionStackTree) error {
	if err := validateFunctionStackTreeShape(src); err != nil {
		return err
	}
	t.init()
	ids := make([]uint32, len(src.Names))
	type frame struct{ node, remaining uint32 }
	stack := []frame{{node: 0, remaining: src.NodeChildren[0]}}
	t.nodes[0].self += src.NodeSelf[0]
	for i := 1; i < len(src.NodeName); i++ {
		for len(stack) > 0 && stack[len(stack)-1].remaining == 0 {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			return ErrMalformedFunctionStackTree
		}
		parent := &stack[len(stack)-1]
		parent.remaining--
		name := src.NodeName[i]
		if name == 0 || int(name) >= len(src.Names) {
			return ErrMalformedFunctionStackTree
		}
		if ids[name] == 0 {
			ids[name] = t.nameID(src.Names[name])
		}
		d := t.child(parent.node, ids[name])
		t.nodes[d].self += src.NodeSelf[i]
		stack = append(stack, frame{node: d, remaining: src.NodeChildren[i]})
	}
	for _, f := range stack {
		if f.remaining != 0 {
			return ErrMalformedFunctionStackTree
		}
	}
	return nil
}

// Proto encodes the tree in depth-first pre-order.
func (t *FunctionStackTree) Proto() *queryv1.FunctionStackTree {
	t.init()
	out := &queryv1.FunctionStackTree{
		Names:        t.names,
		NodeName:     make([]uint32, 0, len(t.nodes)),
		NodeChildren: make([]uint32, 0, len(t.nodes)),
		NodeSelf:     make([]uint64, 0, len(t.nodes)),
	}
	stack := []uint32{0}
	for len(stack) > 0 {
		i := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		n := &t.nodes[i]
		for c := n.firstChild; c != 0; c = t.nodes[c].nextSibling {
			stack = append(stack, c)
		}
		out.NodeName = append(out.NodeName, n.name)
		out.NodeChildren = append(out.NodeChildren, n.children)
		out.NodeSelf = append(out.NodeSelf, n.self)
	}
	return out
}

func validateFunctionStackTreeShape(src *queryv1.FunctionStackTree) error {
	n := len(src.NodeName)
	if n == 0 || len(src.NodeChildren) != n || len(src.NodeSelf) != n ||
		len(src.Names) == 0 || src.Names[0] != "" || src.NodeName[0] != 0 {
		return ErrMalformedFunctionStackTree
	}
	return nil
}

// FunctionStackIndex answers function projections from an encoded
// FunctionStackTree. Results are identical to projections computed from
// the samples the tree was built from.
type FunctionStackIndex struct {
	names  []string
	nameID map[string]uint32
	name   []uint32
	self   []int64
	total  []int64
	// end is the exclusive pre-order index after the node's subtree.
	end []uint32
}

func NewFunctionStackIndex(src *queryv1.FunctionStackTree) (*FunctionStackIndex, error) {
	if err := validateFunctionStackTreeShape(src); err != nil {
		return nil, err
	}
	n := len(src.NodeName)
	x := &FunctionStackIndex{
		names:  src.Names,
		nameID: make(map[string]uint32, len(src.Names)),
		name:   src.NodeName,
		self:   make([]int64, n),
		total:  make([]int64, n),
		end:    make([]uint32, n),
	}
	for i := 1; i < len(src.Names); i++ {
		if _, ok := x.nameID[src.Names[i]]; ok {
			return nil, fmt.Errorf("%w: duplicate name", ErrMalformedFunctionStackTree)
		}
		x.nameID[src.Names[i]] = uint32(i)
	}
	type frame struct{ node, remaining uint32 }
	stack := make([]frame, 0, 64)
	for i := 0; i < n; i++ {
		if src.NodeSelf[i] > math.MaxInt64 {
			return nil, ErrMalformedFunctionStackTree
		}
		x.self[i] = int64(src.NodeSelf[i])
		x.total[i] = x.self[i]
		if i > 0 {
			if len(stack) == 0 {
				return nil, ErrMalformedFunctionStackTree
			}
			if name := src.NodeName[i]; name == 0 || int(name) >= len(src.Names) {
				return nil, ErrMalformedFunctionStackTree
			}
			stack[len(stack)-1].remaining--
		}
		stack = append(stack, frame{node: uint32(i), remaining: src.NodeChildren[i]})
		for len(stack) > 0 && stack[len(stack)-1].remaining == 0 {
			done := stack[len(stack)-1].node
			stack = stack[:len(stack)-1]
			x.end[done] = uint32(i + 1)
			if len(stack) > 0 {
				x.total[stack[len(stack)-1].node] += x.total[done]
			}
		}
	}
	if len(stack) != 0 || x.end[0] != uint32(n) {
		return nil, ErrMalformedFunctionStackTree
	}
	return x, nil
}

func (x *FunctionStackIndex) lookupNames(names []string) ([]uint32, bool) {
	ids := make([]uint32, len(names))
	for i, name := range names {
		id, ok := x.nameID[name]
		if !ok {
			return nil, false
		}
		ids[i] = id
	}
	return ids, true
}

// rootPath returns the nodes whose root-first path equals path.
func (x *FunctionStackIndex) rootPath(path []uint32) []uint32 {
	nodes := []uint32{0}
	for _, name := range path {
		var next []uint32
		for _, n := range nodes {
			for c := n + 1; c < x.end[n]; c = x.end[c] {
				if x.name[c] == name {
					next = append(next, c)
				}
			}
		}
		if len(next) == 0 {
			return nil
		}
		nodes = next
	}
	return nodes
}

// chain calls fn for every node ending an occurrence of chain, with the
// root-first frames of its path; the synthetic root is not a frame.
func (x *FunctionStackIndex) chain(chain []uint32, fn func(node uint32, frames []uint32)) {
	if len(chain) == 0 {
		return
	}
	last := chain[len(chain)-1]
	frames := make([]uint32, 0, 64)
	for i := uint32(1); i < uint32(len(x.name)); i++ {
		for len(frames) > 0 && i >= x.end[frames[len(frames)-1]] {
			frames = frames[:len(frames)-1]
		}
		frames = append(frames, i)
		if x.name[i] != last || len(frames) < len(chain) {
			continue
		}
		matches := true
		for j, name := range chain[:len(chain)-1] {
			if x.name[frames[len(frames)-len(chain)+j]] != name {
				matches = false
				break
			}
		}
		if matches {
			fn(i, frames)
		}
	}
}

func (x *FunctionStackIndex) Functions(callSite []string) *typesv1.FunctionTable {
	table := new(typesv1.FunctionTable)
	path, ok := x.lookupNames(callSite)
	var refs []uint32
	if ok {
		refs = x.rootPath(path)
	}
	stats := make(map[uint32]*typesv1.FunctionStats)
	stat := func(name uint32) *typesv1.FunctionStats {
		s := stats[name]
		if s == nil {
			s = &typesv1.FunctionStats{Name: x.names[name]}
			stats[name] = s
		}
		return s
	}
	active := make([]int32, len(x.names))
	var pathNames []uint32
	for _, name := range path {
		if active[name] == 0 {
			pathNames = append(pathNames, name)
		}
		active[name]++
	}
	stack := make([]uint32, 0, 64)
	for _, ref := range refs {
		table.Total += x.total[ref]
		for _, name := range pathNames {
			stat(name).Total += x.total[ref]
		}
		if ref != 0 {
			stat(x.name[ref]).Self += x.self[ref]
		}
		stack = stack[:0]
		for i := ref + 1; i < x.end[ref]; i++ {
			for len(stack) > 0 && i >= x.end[stack[len(stack)-1]] {
				active[x.name[stack[len(stack)-1]]]--
				stack = stack[:len(stack)-1]
			}
			name := x.name[i]
			s := stat(name)
			if active[name] == 0 {
				s.Total += x.total[i]
			}
			s.Self += x.self[i]
			active[name]++
			stack = append(stack, i)
		}
		for _, n := range stack {
			active[x.name[n]]--
		}
	}
	for _, s := range stats {
		if s.Total > 0 {
			table.Functions = append(table.Functions, s)
		}
	}
	m := NewFunctionTableMerger()
	m.Merge(table)
	return m.Table()
}

// FunctionTree supports the options accepted by the projection API: ROOT_PATH
// with CALLEES, and FUNCTION_CHAIN with a non-empty chain (one function for BOTH).
func (x *FunctionStackIndex) FunctionTree(callSite []string, o *typesv1.FunctionTreeOptions) *typesv1.FunctionTree {
	depth := depthOrDefault(o)
	path, ok := x.lookupNames(callSite)
	t := new(typesv1.FunctionTree)
	identity := func(s string) string { return s }
	direction := o.GetDirection()
	if direction == typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLERS || direction == typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH {
		var root *callTreeNode[string]
		if ok {
			root = x.callers(callSite, path, depth)
		}
		t.Callers = &typesv1.CallTree{Root: root.tree(identity)}
	}
	if direction == typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_CALLEES || direction == typesv1.FunctionTreeDirection_FUNCTION_TREE_DIRECTION_BOTH {
		var root *callTreeNode[string]
		if ok {
			root = x.callees(callSite, path, depth, o.GetSelection())
		}
		t.Callees = &typesv1.CallTree{Root: root.tree(identity)}
	}
	return t
}

func (x *FunctionStackIndex) callees(callSite []string, path []uint32, depth int, selection typesv1.FunctionTreeSelection) *callTreeNode[string] {
	var root *callTreeNode[string]
	add := func(ref uint32) {
		if root == nil {
			root = &callTreeNode[string]{}
			if len(callSite) > 0 {
				root.name = callSite[len(callSite)-1]
			}
		}
		root.total += x.total[ref]
		root.self += x.self[ref]
		x.addCallees(root, ref, depth)
	}
	if selection == typesv1.FunctionTreeSelection_FUNCTION_TREE_SELECTION_FUNCTION_CHAIN {
		x.chain(path, func(node uint32, _ []uint32) { add(node) })
	} else {
		for _, ref := range x.rootPath(path) {
			add(ref)
		}
	}
	return root
}

func (x *FunctionStackIndex) addCallees(out *callTreeNode[string], node uint32, depth int) {
	if node+1 < x.end[node] {
		out.hasChildren = true
	}
	if depth == 0 {
		return
	}
	for c := node + 1; c < x.end[node]; c = x.end[c] {
		child := out.child(x.names[x.name[c]])
		child.total += x.total[c]
		child.self += x.self[c]
		x.addCallees(child, c, depth-1)
	}
}

func (x *FunctionStackIndex) callers(callSite []string, path []uint32, depth int) *callTreeNode[string] {
	var root *callTreeNode[string]
	x.chain(path, func(node uint32, frames []uint32) {
		if root == nil {
			root = &callTreeNode[string]{name: callSite[0]}
		}
		value := x.total[node]
		root.total += value
		if len(path) == 1 {
			root.self += x.self[node]
		}
		n := root
		for i, d := len(frames)-len(path)-1, 0; i >= 0; i, d = i-1, d+1 {
			n.hasChildren = true
			if d == depth {
				break
			}
			n = n.child(x.names[x.name[frames[i]]])
			n.total += value
		}
	})
	return root
}

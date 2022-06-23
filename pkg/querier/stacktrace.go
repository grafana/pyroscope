package querier

import (
	"context"
	"fmt"

	"github.com/apache/arrow/go/v8/arrow"
	"github.com/apache/arrow/go/v8/arrow/array"
	"github.com/parca-dev/parca/pkg/metastore"
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
	"github.com/xlab/treeprint"
)

type stack struct {
	locations []string
	value     int64
}

type node struct {
	parent      *node
	children    []*node
	self, total int64
	name        string
}

func (n *node) Add(name string, self, total int64) *node {
	new := &node{
		parent: n,
		name:   name,
		self:   self,
		total:  total,
	}
	n.children = append(n.children, new)
	return new
}

func (n *node) Clone() *node {
	new := *n
	return &new
}

type tree struct {
	root []*node
}

func (t *tree) Add(name string, self, total int64) *node {
	new := &node{
		name:  name,
		self:  self,
		total: total,
	}
	t.root = append(t.root, new)
	return new
}

func newTree() *tree {
	return &tree{}
}

func (t tree) String() string {
	type branch struct {
		nodes []*node
		treeprint.Tree
	}
	tree := treeprint.New()
	for _, n := range t.root {
		b := tree.AddBranch(fmt.Sprintf("%s: self %d total %d", n.name, n.self, n.total))
		remaining := append([]*branch{}, &branch{nodes: n.children, Tree: b})
		for len(remaining) > 0 {
			current := remaining[0]
			remaining = remaining[1:]
			for _, n := range current.nodes {
				if len(n.children) > 0 {
					remaining = append(remaining, &branch{nodes: n.children, Tree: current.Tree.AddBranch(fmt.Sprintf("%s: self %d total %d", n.name, n.self, n.total))})
				} else {
					current.Tree.AddNode(fmt.Sprintf("%s: self %d total %d", n.name, n.self, n.total))
				}
			}
		}
	}
	return tree.String()
}

func mergeTree(dst, src *tree) {
	// walk src and insert src's nodes into dst
	for _, rootNode := range src.root {
		parent, found, toMerge := findNodeOrParent(dst.root, rootNode)
		if found == nil {
			if parent == nil {
				dst.root = append(dst.root, toMerge)
				continue
			}
			toMerge.parent = parent
			parent.children = append(parent.children, toMerge)
			for p := parent; p != nil; p = p.parent {
				p.total = p.total + toMerge.total
			}
			continue
		}
		found.total = found.total + toMerge.self
		found.self = found.self + toMerge.self
		for p := found.parent; p != nil; p = p.parent {
			p.total = p.total + toMerge.total
		}
	}
}

// Walks into root nodes to find a node, return the latest common parent visited.
func findNodeOrParent(root []*node, new *node) (parent, found, toMerge *node) {
	current := new
	var lastParent *node
	remaining := append([]*node{}, root...)
	for len(remaining) > 0 {
		n := remaining[0]
		remaining = remaining[1:]
		// we found the common parent so we go down
		if n.name == current.name {
			// we reach the end of the new path to find.
			if len(current.children) == 0 {
				return lastParent, n, current
			}
			lastParent = n
			remaining = n.children
			current = current.children[0]
			continue
		}
	}

	return lastParent, nil, current
}

func strackToTree(stack stack) *tree {
	t := &tree{}
	if len(stack.locations) == 0 {
		return t
	}
	current := &node{
		self:  stack.value,
		total: stack.value,
		name:  stack.locations[0],
	}
	if len(stack.locations) == 1 {
		t.root = append(t.root, current)
		return t
	}
	remaining := stack.locations[1:]
	for len(remaining) > 0 {

		location := remaining[0]
		name := location
		remaining = remaining[1:]

		// This pack node with the same name as the next location
		// Disable for now but we might want to introduce it if we find it useful.
		// for len(remaining) != 0 {
		// 	if remaining[0].function == name {
		// 		remaining = remaining[1:]
		// 		continue
		// 	}
		// 	break
		// }

		parent := &node{
			children: []*node{current},
			total:    current.total,
			name:     name,
		}
		current.parent = parent
		current = parent
	}
	t.root = []*node{current}
	return t
}

func stacksToTree(stacks []stack) *tree {
	t := &tree{}
	for _, stack := range stacks {
		if stack.value == 0 {
			continue
		}
		if t == nil {
			t = strackToTree(stack)
			continue
		}
		mergeTree(t, strackToTree(stack))
	}
	return t
}

func (t *tree) toFlamebearer() *flamebearer.FlamebearerV1 {
	var total, max int64
	for _, node := range t.root {
		total += node.total
	}
	names := []string{}
	nameLocationCache := map[string]int{}
	res := [][]int{}

	xOffsets := []int{0}

	levels := []int{0}

	nodes := []*node{{children: t.root, total: total}}

	for len(nodes) > 0 {
		current := nodes[0]
		nodes = nodes[1:]

		xOffset := xOffsets[0]
		xOffsets = xOffsets[1:]

		level := levels[0]
		levels = levels[1:]
		if current.self > max {
			max = current.self
		}
		var i int
		var ok bool
		name := current.name
		if i, ok = nameLocationCache[name]; !ok {
			i = len(names)
			if i == 0 {
				name = "total"
			}
			nameLocationCache[name] = i
			names = append(names, name)
		}

		if level == len(res) {
			res = append(res, []int{})
		}

		// i+0 = x offset
		// i+1 = total
		// i+2 = self
		// i+3 = index in names array
		res[level] = append([]int{xOffset, int(current.total), int(current.self), i}, res[level]...)
		xOffset += int(current.self)

		for _, child := range current.children {
			xOffsets = append([]int{xOffset}, xOffsets...)
			levels = append([]int{level + 1}, levels...)
			nodes = append([]*node{child}, nodes...)
			xOffset += int(child.total)
		}
	}
	// delta encode xoffsets
	for _, l := range res {
		prev := 0
		for i := 0; i < len(l); i += 4 {
			l[i] -= prev
			prev += l[i] + l[i+1]
		}
	}
	return &flamebearer.FlamebearerV1{
		Names:    names,
		Levels:   res,
		NumTicks: int(total),
		MaxSelf:  int(max),
	}
}

func buildFlamebearer(ar arrow.Record, meta metastore.ProfileMetaStore) (*flamebearer.FlamebearerV1, error) {
	type sample struct {
		stacktraceID []byte
		locationIDs  [][]byte
		total        int64
		self         int64

		*metastore.Location
	}
	schema := ar.Schema()
	indices := schema.FieldIndices("stacktrace")
	if len(indices) != 1 {
		return nil, fmt.Errorf("expected exactly one stacktrace column, got %d", len(indices))
	}
	stacktraceColumn := ar.Column(indices[0]).(*array.Binary)

	indices = schema.FieldIndices("sum(value)")
	if len(indices) != 1 {
		return nil, fmt.Errorf("expected exactly one value column, got %d", len(indices))
	}
	valueColumn := ar.Column(indices[0]).(*array.Int64)

	rows := int(ar.NumRows())
	samples := make([]*sample, 0, rows)
	stacktraceUUIDs := make([][]byte, 0, rows)
	for i := 0; i < rows; i++ {
		stacktraceID := stacktraceColumn.Value(i)
		value := valueColumn.Value(i)
		stacktraceUUIDs = append(stacktraceUUIDs, stacktraceID)
		samples = append(samples, &sample{
			stacktraceID: stacktraceID,
			self:         value,
		})
	}

	stacktraceMap, err := meta.GetStacktraceByIDs(context.Background(), stacktraceUUIDs...)
	if err != nil {
		return nil, err
	}

	locationUUIDSeen := map[string]struct{}{}
	locationUUIDs := [][]byte{}
	for _, s := range stacktraceMap {
		for _, id := range s.GetLocationIds() {
			if _, seen := locationUUIDSeen[string(id)]; !seen {
				locationUUIDSeen[string(id)] = struct{}{}
				locationUUIDs = append(locationUUIDs, id)
			}
		}
	}

	locationMaps, err := metastore.GetLocationsByIDs(context.Background(), meta, locationUUIDs...)
	if err != nil {
		return nil, err
	}

	for _, s := range samples {
		s.locationIDs = stacktraceMap[string(s.stacktraceID)].LocationIds
	}

	stacks := make([]stack, 0, len(samples))
	for _, s := range samples {
		stack := stack{
			value: s.self,
		}

		for i := range s.locationIDs {
			stack.locations = append(stack.locations, locationMaps[string(s.locationIDs[i])].Lines[0].Function.Name)
		}

		stacks = append(stacks, stack)
	}
	tree := stacksToTree(stacks)
	graph := tree.toFlamebearer()
	return graph, nil
}

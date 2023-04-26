package querier

import (
	"fmt"

	"github.com/xlab/treeprint"
)

type tree struct {
	root []*node
}

func emptyTree() *tree {
	return &tree{}
}

func newTree(stacks []stacktraces) *tree {
	t := emptyTree()
	for _, stack := range stacks {
		if stack.value == 0 {
			continue
		}
		if t == nil {
			t = stackToTree(stack)
			continue
		}
		mergeTree(t, stackToTree(stack))
	}
	return t
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

func stackToTree(stack stacktraces) *tree {
	t := emptyTree()
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

type node struct {
	parent      *node
	children    []*node
	self, total int64
	name        string
}

func (n *node) String() string {
	return fmt.Sprintf("{%s: self %d total %d}", n.name, n.self, n.total)
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

// Walks into root nodes to find a node, return the latest common parent visited.
func findNodeOrParent(root []*node, new *node) (parent, found, toMerge *node) {
	current := new
	var lastParent *node
	remaining := root
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

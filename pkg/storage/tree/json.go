package tree

import "encoding/json"

func (t *Tree) MarshalJSON() ([]byte, error) {
	t.RLock()
	defer t.RUnlock()
	return json.Marshal(t.root().toJSONNode(t))
}

type treeNodeJSON struct {
	Name          string         `json:"name"`
	Total         uint64         `json:"total"`
	Self          uint64         `json:"self"`
	ChildrenNodes []treeNodeJSON `json:"children"`
}

func (n *treeNode) toJSONNode(t *Tree) treeNodeJSON {
	nodes := make([]treeNodeJSON, len(n.ChildrenNodes))
	for i := range n.ChildrenNodes {
		nodes[i] = t.at(n.ChildrenNodes[i]).toJSONNode(t)
	}
	return treeNodeJSON{
		Name:          string(t.loadLabel(n.labelPosition)),
		Total:         n.Total,
		Self:          n.Self,
		ChildrenNodes: nodes,
	}
}

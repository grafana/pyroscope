package segment

import (
	"time"
)

// var highchartsTemplate *template.Template

func init() {
}

type visualizeNode struct {
	T1      time.Time
	T2      time.Time
	Depth   int
	HasTrie bool
}

// This is here for debugging
func (s *Segment) Visualize() {
	res := []*visualizeNode{}
	if s.root != nil {
		nodes := []*streeNode{s.root}
		for len(nodes) != 0 {
			n := nodes[0]
			nodes = nodes[1:]
			// log.Debug("node:", durations[n.depth])
			res = append(res, &visualizeNode{
				T1:      n.time.UTC(),
				T2:      n.time.Add(durations[n.depth]).UTC(),
				Depth:   n.depth,
				HasTrie: n.present,
			})
			for _, v := range n.children {
				if v != nil {
					nodes = append(nodes, v)
				}
			}
		}
	}

	// jsonBytes, _ := json.MarshalIndent(res, "", "  ")
	// log.Debug(string(jsonBytes))
}

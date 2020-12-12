package tree

import (
	"sort"
	"strings"
)

type Flamebearer struct {
	Names    []string `json:"names"`
	Levels   [][]int  `json:"levels"`
	NumTicks int      `json:"numTicks"`
}

func (t *Tree) FlamebearerStruct(maxNodes int) *Flamebearer {
	t.m.RLock()
	defer t.m.RUnlock()

	res := Flamebearer{
		Names:    []string{},
		Levels:   [][]int{},
		NumTicks: int(t.Samples()),
	}

	nodes := []*treeNode{t.root}
	xOffsets := []int{0}
	levels := []int{0}
	minVal := t.minValue(maxNodes)

	for len(nodes) > 0 {
		tn := nodes[0]
		nodes = nodes[1:]

		xOffset := xOffsets[0]
		xOffsets = xOffsets[1:]

		level := levels[0]
		levels = levels[1:]

		name := string(tn.Name)

		i := sort.Search(len(res.Names), func(i int) bool {
			return strings.Compare(res.Names[i], name) >= 0
		})

		if i == len(res.Names) || res.Names[i] != name {
			res.Names = append(res.Names, "")
			copy(res.Names[i+1:len(res.Names)], res.Names[i:len(res.Names)])
			res.Names[i] = name
		}

		if tn.Total > minVal {
			if level == len(res.Levels) {
				res.Levels = append(res.Levels, []int{})
			}
			// * barIndex, delta encoded
			// * numBarTicks
			// * link to name
			// barIndex := xOffset
			// if len(res.Levels[level]) > 0 { // delta encoding
			// 	prevX := res.Levels[level][len(res.Levels[level])-3]
			// 	prevW := res.Levels[level][len(res.Levels[level])-2]
			// 	barIndex -= prevX + prevW
			// }
			res.Levels[level] = append([]int{xOffset, int(tn.Total), i}, res.Levels[level]...)

			xOffset += int(tn.Self)

			for _, n := range tn.ChildrenNodes {
				// TODO: not sure if this condition is required
				if n.Total > minVal {
					xOffsets = append([]int{xOffset}, xOffsets...)
					levels = append([]int{level + 1}, levels...)
					nodes = append([]*treeNode{n}, nodes...)
					xOffset += int(n.Total)
				}
			}
		}
	}
	for _, l := range res.Levels {
		prev := 0
		for i := 0; i < len(l); i += 3 {
			l[i] -= prev
			prev += l[i] + l[i+1]
		}
	}
	return &res
}

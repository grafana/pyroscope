package querier

import (
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
)

func NewFlamebearer(t *tree) *flamebearer.FlamebearerV1 {
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

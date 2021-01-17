package tree

type Flamebearer struct {
	Names      []string `json:"names"`
	Levels     [][]int  `json:"levels"`
	NumTicks   int      `json:"numTicks"`
	MaxSelf    int      `json:"maxSelf"`
	SpyName    string   `json:"spyName"`
	SampleRate int      `json:"sampleRate"`
}

func (t *Tree) FlamebearerStruct(maxNodes int) *Flamebearer {
	t.m.RLock()
	defer t.m.RUnlock()

	res := Flamebearer{
		Names:    []string{},
		Levels:   [][]int{},
		NumTicks: int(t.Samples()),
		MaxSelf:  int(0),
	}

	nodes := []*treeNode{t.root}
	xOffsets := []int{0}
	levels := []int{0}
	minVal := t.minValue(maxNodes)
	nameLocationCache := map[string]int{}

	for len(nodes) > 0 {
		tn := nodes[0]
		nodes = nodes[1:]

		xOffset := xOffsets[0]
		xOffsets = xOffsets[1:]

		level := levels[0]
		levels = levels[1:]

		name := string(tn.Name)
		if tn.Total >= minVal || name == "other" {
			var i int
			var ok bool
			if i, ok = nameLocationCache[name]; !ok {
				i = len(res.Names)
				nameLocationCache[name] = i
				if i == 0 {
					name = "total"
				}
				res.Names = append(res.Names, name)
			}

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
			if res.MaxSelf < int(tn.Self) {
				res.MaxSelf = int(tn.Self)
			}
			res.Levels[level] = append([]int{xOffset, int(tn.Total), int(tn.Self), i}, res.Levels[level]...)

			xOffset += int(tn.Self)
			otherTotal := uint64(0)
			for _, n := range tn.ChildrenNodes {
				if n.Total >= minVal {
					xOffsets = append([]int{xOffset}, xOffsets...)
					levels = append([]int{level + 1}, levels...)
					nodes = append([]*treeNode{n}, nodes...)
					xOffset += int(n.Total)
				} else {
					otherTotal += n.Total
				}
			}
			if otherTotal != 0 {
				n := &treeNode{
					Name:  jsonableSlice("other"),
					Total: otherTotal,
					Self:  otherTotal,
				}
				xOffsets = append([]int{xOffset}, xOffsets...)
				levels = append([]int{level + 1}, levels...)
				nodes = append([]*treeNode{n}, nodes...)
				xOffset += int(n.Total)
			}
		}
	}
	for _, l := range res.Levels {
		prev := 0
		for i := 0; i < len(l); i += 4 {
			l[i] -= prev
			prev += l[i] + l[i+1]
		}
	}
	// TODO: we used to drop the first level, because it's always an empty node
	//   but that didn't work because flamebearer doesn't work with more
	//   than one root element. Long term we should fix it on flamebearer side
	// if len(res.Levels) > 0 {
	// 	res.Levels = res.Levels[1:]
	// }
	return &res
}

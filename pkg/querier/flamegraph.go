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
	res := []*Stack[int]{}
	defer func() {
		for _, stack := range res {
			stack.Release()
		}
	}()

	xOffsets := NewStack(0)
	defer xOffsets.Release()

	levels := NewStack(0)
	defer levels.Release()

	nodes := NewStack(&node{children: t.root, total: total})
	defer nodes.Release()

	for {
		current, hasMoreNodes := nodes.Pop()
		if !hasMoreNodes {
			break
		}

		xOffset, hasMoreOffsets := xOffsets.Pop()
		if !hasMoreOffsets {
			break
		}
		level, hasMoreLevels := levels.Pop()
		if !hasMoreLevels {
			break
		}

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
			res = append(res, NewStack[int]())
		}

		// i+0 = x offset
		// i+1 = total
		// i+2 = self
		// i+3 = index in names array
		res[level].Push(i)
		res[level].Push(int(current.self))
		res[level].Push(int(current.total))
		res[level].Push(xOffset)
		xOffset += int(current.self)

		for _, child := range current.children {
			xOffsets.Push(xOffset)
			levels.Push(level + 1)
			nodes.Push(child)
			xOffset += int(child.total)
		}
	}
	result := make([][]int, len(res))
	for i := range result {
		result[i] = res[i].Slice()
	}
	// delta encode xoffsets
	for _, l := range result {
		prev := 0
		for i := 0; i < len(l); i += 4 {
			l[i] -= prev
			prev += l[i] + l[i+1]
		}
	}
	return &flamebearer.FlamebearerV1{
		Names:    names,
		Levels:   result,
		NumTicks: int(total),
		MaxSelf:  int(max),
	}
}

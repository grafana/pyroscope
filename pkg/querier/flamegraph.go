package querier

import (
	"github.com/pyroscope-io/pyroscope/pkg/structs/flamebearer"
)

type stackNode struct {
	xOffset int
	level   int
	node    *node
}

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
			stackIntPool.Put(stack)
		}
	}()

	stack := stackNodePool.Get().(*Stack[stackNode])
	defer stackNodePool.Put(stack)
	stack.Reset()
	stack.Push(stackNode{xOffset: 0, level: 0, node: &node{children: t.root, total: total}})

	for {
		current, hasMoreNodes := stack.Pop()
		if !hasMoreNodes {
			break
		}
		if current.node.self > max {
			max = current.node.self
		}
		var i int
		var ok bool
		name := current.node.name
		if i, ok = nameLocationCache[name]; !ok {
			i = len(names)
			if i == 0 {
				name = "total"
			}
			nameLocationCache[name] = i
			names = append(names, name)
		}

		if current.level == len(res) {
			s := stackIntPool.Get().(*Stack[int])
			s.Reset()
			res = append(res, s)
		}

		// i+0 = x offset
		// i+1 = total
		// i+2 = self
		// i+3 = index in names array
		level := res[current.level]
		level.Push(i)
		level.Push(int(current.node.self))
		level.Push(int(current.node.total))
		level.Push(current.xOffset)
		current.xOffset += int(current.node.self)

		for _, child := range current.node.children {
			stack.Push(stackNode{xOffset: current.xOffset, level: current.level + 1, node: child})
			current.xOffset += int(child.total)
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

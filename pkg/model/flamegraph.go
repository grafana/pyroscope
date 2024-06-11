package model

import (
	"sort"
	"sync"

	"github.com/samber/lo"

	"github.com/grafana/pyroscope/pkg/og/storage/metadata"
	"github.com/grafana/pyroscope/pkg/og/structs/flamebearer"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func NewFlameGraph(t *Tree, maxNodes int64) *querierv1.FlameGraph {
	var total, max int64
	for _, node := range t.root {
		total += node.total
	}
	names := []string{}
	nameLocationCache := map[string]int{}
	res := []*Stack[int64]{}
	defer func() {
		for _, stack := range res {
			stackIntPool.Put(stack)
		}
	}()

	minVal := t.minValue(maxNodes)

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
			s := stackIntPool.Get().(*Stack[int64])
			s.Reset()
			res = append(res, s)
		}

		// i+0 = x offset
		// i+1 = total
		// i+2 = self
		// i+3 = index in names array
		level := res[current.level]
		level.Push(int64(i))
		level.Push((current.node.self))
		level.Push((current.node.total))
		level.Push(int64(current.xOffset))
		current.xOffset += int(current.node.self)

		otherTotal := int64(0)
		for _, child := range current.node.children {
			if child.total >= minVal && child.name != "other" {
				stack.Push(stackNode{xOffset: current.xOffset, level: current.level + 1, node: child})
				current.xOffset += int(child.total)
			} else {
				otherTotal += child.total
			}
		}
		if otherTotal != 0 {
			child := &node{
				name:   "other",
				parent: current.node,
				self:   otherTotal,
				total:  otherTotal,
			}
			stack.Push(stackNode{xOffset: current.xOffset, level: current.level + 1, node: child})
			current.xOffset += int(child.total)
		}
	}

	result := make([][]int64, len(res))
	for i := range result {
		result[i] = res[i].Slice()
	}
	// delta encode xoffsets
	for _, l := range result {
		prev := int64(0)
		for i := 0; i < len(l); i += 4 {
			l[i] -= prev
			prev += l[i] + l[i+1]
		}
	}
	levels := make([]*querierv1.Level, len(result))
	for i := range levels {
		levels[i] = &querierv1.Level{
			Values: result[i],
		}
	}

	return &querierv1.FlameGraph{
		Names:   names,
		Levels:  levels,
		Total:   total,
		MaxSelf: max,
	}
}

// ExportToFlamebearer exports the flamegraph to a Flamebearer struct.
func ExportToFlamebearer(fg *querierv1.FlameGraph, profileType *typesv1.ProfileType) *flamebearer.FlamebearerProfile {
	if fg == nil {
		fg = &querierv1.FlameGraph{}
	}
	unit := metadata.Units(profileType.SampleUnit)
	sampleRate := uint32(100)

	switch profileType.SampleType {
	case "inuse_objects", "alloc_objects", "goroutine", "samples":
		unit = metadata.ObjectsUnits
	case "cpu":
		unit = metadata.SamplesUnits
		sampleRate = uint32(1_000_000_000)

	}
	levels := make([][]int, len(fg.Levels))
	for i := range levels {
		levels[i] = lo.Map(fg.Levels[i].Values, func(v int64, i int) int { return int(v) })
	}
	return &flamebearer.FlamebearerProfile{
		Version: 1,
		FlamebearerProfileV1: flamebearer.FlamebearerProfileV1{
			Flamebearer: flamebearer.FlamebearerV1{
				Names:    fg.Names,
				NumTicks: int(fg.Total),
				MaxSelf:  int(fg.MaxSelf),
				Levels:   levels,
			},
			Metadata: flamebearer.FlamebearerMetadataV1{
				Format:     "single",
				Units:      unit,
				Name:       profileType.SampleType,
				SampleRate: sampleRate,
			},
		},
	}
}

func ExportDiffToFlamebearer(fg *querierv1.FlameGraphDiff, profileType *typesv1.ProfileType) *flamebearer.FlamebearerProfile {
	// Since a normal flamegraph and a diff are so similar, convert it to reuse the export function
	singleFlamegraph := &querierv1.FlameGraph{
		Names:   fg.Names,
		Levels:  fg.Levels,
		Total:   fg.Total,
		MaxSelf: fg.MaxSelf,
	}

	fb := ExportToFlamebearer(singleFlamegraph, profileType)
	fb.LeftTicks = uint64(fg.LeftTicks)
	fb.RightTicks = uint64(fg.RightTicks)
	fb.FlamebearerProfileV1.Metadata.Format = "double"

	return fb
}

type FlameGraphMerger struct {
	mu sync.Mutex
	t  *Tree
}

func NewFlameGraphMerger() *FlameGraphMerger {
	return &FlameGraphMerger{t: new(Tree)}
}

// MergeFlameGraph adds the flame graph stack traces to the resulting
// flame graph. The call is thread-safe, but the resulting flame graph
// or tree should be only accessed after all the samples are merged.
func (m *FlameGraphMerger) MergeFlameGraph(src *querierv1.FlameGraph) {
	m.mu.Lock()
	defer m.mu.Unlock()
	deltaDecoding(src.Levels, 0, 4)
	dst := make([]string, 0, len(src.Levels))
	for i, l := range src.Levels {
		if i == 0 {
			// Skip the root node ("total").
			continue
		}
		for j := 0; j < len(l.Values); j += 4 {
			self := l.Values[j+2]
			if self > 0 {
				dst = buildStack(dst, src, i, j)
				m.t.InsertStack(self, dst...)
			}
		}
	}
}

func (m *FlameGraphMerger) MergeTreeBytes(src []byte) error {
	t, err := UnmarshalTree(src)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.t.Merge(t)
	return nil
}

func (m *FlameGraphMerger) Tree() *Tree { return m.t }

func (m *FlameGraphMerger) FlameGraph(maxNodes int64) *querierv1.FlameGraph {
	t := m.t
	if t == nil {
		t = new(Tree)
	}
	return NewFlameGraph(t, maxNodes)
}

func deltaDecoding(levels []*querierv1.Level, start, step int) {
	for _, l := range levels {
		prev := int64(0)
		for i := start; i < len(l.Values); i += step {
			delta := l.Values[i] + l.Values[i+1]
			l.Values[i] += prev
			prev += delta
		}
	}
}

func buildStack(dst []string, f *querierv1.FlameGraph, level, idx int) []string {
	if cap(dst) < level {
		// Actually, it should never be the case, because
		// we know the depth in advance and can allocate
		// dst with the right capacity.
		dst = make([]string, level, level*2)
	} else {
		dst = dst[:level]
	}
	dst[level-1] = f.Names[f.Levels[level].Values[idx+3]]
	x := f.Levels[level].Values[idx]
	for i := level - 1; i > 0; i-- {
		j := sort.Search(len(f.Levels[i].Values)/4, func(j int) bool { return f.Levels[i].Values[j*4] > x }) - 1
		dst[i-1] = f.Names[f.Levels[i].Values[j*4+3]]
		x = f.Levels[i].Values[j*4]
	}
	return dst
}

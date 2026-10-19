package model

import (
	"cmp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
)

type FlatTreeParts struct {
	parts  []flatTreePart
	inputs atomic.Int64

	mu       sync.Mutex
	assigned map[string]int
	load     []int64
}

type flatTreePart struct {
	sync.Mutex
	t *FlatTree
}

func NewFlatTreeParts(n int) *FlatTreeParts {
	n = max(n, 1)
	f := &FlatTreeParts{
		parts:    make([]flatTreePart, n),
		assigned: make(map[string]int),
		load:     make([]int64, n),
	}
	for i := range f.parts {
		f.parts[i].t = NewFlatTree()
	}
	return f
}

func (f *FlatTreeParts) Inputs() int { return int(f.inputs.Load()) }

func (f *FlatTreeParts) partsOf(names []string, weights []int64) []int {
	parts := make([]int, len(names))
	f.mu.Lock()
	defer f.mu.Unlock()
	var unassigned []int
	for i, name := range names {
		k, ok := f.assigned[name]
		if !ok {
			unassigned = append(unassigned, i)
			continue
		}
		parts[i] = k
		f.load[k] += weights[i]
	}
	slices.SortStableFunc(unassigned, func(a, b int) int { return cmp.Compare(weights[b], weights[a]) })
	for _, i := range unassigned {
		k := 0
		for j := range f.load {
			if f.load[j] < f.load[k] {
				k = j
			}
		}
		f.assigned[names[i]] = k
		f.load[k] += weights[i]
		parts[i] = k
	}
	return parts
}

func (f *FlatTreeParts) MergePreparedCallTree(p *PreparedCallTree) {
	f.inputs.Add(1)
	if p.c == nil {
		return
	}
	roots := p.kids[p.off[0]:p.off[1]]
	names := make([]string, len(roots))
	for i, r := range roots {
		names[i] = p.nameOf(r)
	}
	groups := make([][]int32, len(f.parts))
	for i, k := range f.partsOf(names, p.subtreeSizes(roots)) {
		groups[k] = append(groups[k], roots[i])
	}
	f.each(groups, func(t *FlatTree, roots []int32) { t.mergePreparedRoots(p, roots) })
}

func (f *FlatTreeParts) MergeTree(src *FlatTree) {
	f.inputs.Add(int64(src.inputs))
	if len(src.parent) == 1 {
		return
	}
	top := make([]int32, len(src.parent))
	size := make([]int64, len(src.parent))
	for i := 1; i < len(src.parent); i++ {
		if p := src.parent[i]; p == 0 {
			top[i] = int32(i)
		} else {
			top[i] = top[p]
		}
		size[top[i]]++
	}
	var roots []int32
	var names []string
	var weights []int64
	for r := src.child[0]; r != noNode; r = src.next[r] {
		roots = append(roots, r)
		names = append(names, src.names.names[src.name[r]])
		weights = append(weights, size[r])
	}
	groups := make([][]int32, len(f.parts))
	for i, k := range f.partsOf(names, weights) {
		groups[k] = append(groups[k], roots[i])
	}
	f.each(groups, func(t *FlatTree, roots []int32) { t.mergeTreeRoots(src, roots) })
}

func (f *FlatTreeParts) each(groups [][]int32, fn func(*FlatTree, []int32)) {
	var wg sync.WaitGroup
	for k, roots := range groups {
		if len(roots) == 0 {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := &f.parts[k]
			p.Lock()
			defer p.Unlock()
			fn(p.t, roots)
		}()
	}
	wg.Wait()
}

func (f *FlatTreeParts) Tree() *FlatTree {
	n := 1
	for i := range f.parts {
		n += len(f.parts[i].t.parent) - 1
	}
	t := &FlatTree{
		name:   make([]int32, n),
		self:   make([]int64, n),
		parent: make([]int32, n),
		child:  make([]int32, n),
		next:   make([]int32, n),
		inputs: int(f.inputs.Load()),
	}
	t.names.ids = make(map[string]int32)
	t.name[0], t.parent[0], t.child[0], t.next[0] = noNode, noNode, noNode, noNode

	type span struct {
		part  *FlatTree
		base  int32
		names []int32
	}
	spans := make([]span, 0, len(f.parts))
	base := int32(0)
	for i := range f.parts {
		p := f.parts[i].t
		if len(p.parent) == 1 {
			continue
		}
		names := make([]int32, len(p.names.names))
		for j, s := range p.names.names {
			names[j] = t.names.id(s)
		}
		spans = append(spans, span{part: p, base: base, names: names})
		base += int32(len(p.parent)) - 1
	}

	var wg sync.WaitGroup
	for _, s := range spans {
		wg.Add(1)
		go func() {
			defer wg.Done()
			shift := func(x int32) int32 {
				if x == noNode || x == 0 {
					return x
				}
				return x + s.base
			}
			p := s.part
			for j := 1; j < len(p.parent); j++ {
				x := s.base + int32(j)
				t.name[x] = s.names[p.name[j]]
				t.self[x] = p.self[j]
				t.parent[x] = shift(p.parent[j])
				t.child[x] = shift(p.child[j])
				t.next[x] = shift(p.next[j])
			}
		}()
	}
	wg.Wait()

	var roots []int32
	for _, s := range spans {
		for r := s.part.child[0]; r != noNode; r = s.part.next[r] {
			roots = append(roots, r+s.base)
		}
	}
	slices.SortFunc(roots, func(a, b int32) int {
		return strings.Compare(t.names.names[t.name[b]], t.names.names[t.name[a]])
	})
	for i, r := range roots {
		if i == 0 {
			t.child[0] = r
		}
		if i+1 < len(roots) {
			t.next[r] = roots[i+1]
		} else {
			t.next[r] = noNode
		}
	}
	return t
}

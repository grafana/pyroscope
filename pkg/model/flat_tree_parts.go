package model

import (
	"cmp"
	"encoding/binary"
	"errors"
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

func (f *FlatTreeParts) MergeCompactBytes(b []byte, names []string) error {
	f.inputs.Add(1)
	if len(b) < 2 {
		return nil
	}
	_, _, n, offset, err := readCompactRecord(b, 0)
	if err != nil {
		return err
	}
	var offsets []int
	var rootNames []string
	var weights []int64
	for ; n > 0; n-- {
		name, _, children, next, err := readCompactRecord(b, offset)
		if err != nil {
			return err
		}
		if name >= uint64(len(names)) {
			return errMalformedTreeBytes
		}
		end, err := skipCompactRecords(b, next, children)
		if err != nil {
			return err
		}
		offsets = append(offsets, offset)
		rootNames = append(rootNames, names[name])
		weights = append(weights, int64(end-offset)/compactRecordSize)
		offset = end
	}
	groups := make([][]int, len(f.parts))
	for i, k := range f.partsOf(rootNames, weights) {
		groups[k] = append(groups[k], offsets[i])
	}
	errs := make([]error, len(f.parts))
	each(f, groups, func(k int, t *FlatTree, offsets []int) {
		errs[k] = t.mergeCompactRoots(b, names, offsets)
	})
	return errors.Join(errs...)
}

const compactRecordSize = 6

func (f *FlatTreeParts) BytesCompact(maxNodes int64) ([]byte, []string) {
	if f.Inputs() == 1 {
		maxNodes = 0
	}
	type span struct{ lo, hi int }
	type part struct {
		t     *FlatTree
		total []int64
		used  []bool
		m     *treeMarshaler
		roots []int32
		spans []span
	}
	var parts []*part
	n := 0
	for i := range f.parts {
		if t := f.parts[i].t; len(t.parent) > 1 {
			parts = append(parts, &part{t: t})
			n += len(t.parent) - 1
		}
	}
	if n == 0 {
		return nil, nil
	}
	forEach(parts, func(p *part) { p.total = p.t.totals() })
	var minValue int64
	if maxNodes >= 1 && int64(n) > maxNodes {
		all := make([]int64, 0, n)
		for _, p := range parts {
			all = append(all, p.total[1:]...)
		}
		minValue = kthLargest(all, int(maxNodes))
	}

	forEach(parts, func(p *part) {
		other, hasOther := p.t.names.ids[truncatedNodeName]
		p.used = make([]bool, len(p.t.names.names))
		for i := 1; i < len(p.t.parent); i++ {
			if p.total[i] >= minValue || (hasOther && p.t.name[i] == other) {
				p.used[p.t.name[i]] = true
			}
		}
	})
	var table []string
	positions := make(map[string]int32)
	for _, p := range parts {
		p.m = newTreeMarshaler(p.t, p.total, minValue, true)
		for id, used := range p.used {
			if !used {
				continue
			}
			name := p.t.names.names[id]
			x, ok := positions[name]
			if !ok {
				x = int32(len(table))
				table = append(table, name)
				positions[name] = x
			}
			p.m.index[id] = x
		}
	}
	otherIndex, ok := positions[truncatedNodeName]
	if !ok && minValue > 0 {
		otherIndex = int32(len(table))
		table = append(table, truncatedNodeName)
	}
	for _, p := range parts {
		p.m.otherIndex = otherIndex
	}

	type top struct {
		p     *part
		x     int32
		name  string
		total int64
	}
	var tops []top
	for _, p := range parts {
		for c := p.t.child[0]; c != noNode; c = p.t.next[c] {
			tops = append(tops, top{p: p, x: c, name: p.t.names.names[p.t.name[c]], total: p.total[c]})
		}
	}
	slices.SortFunc(tops, func(a, b top) int { return strings.Compare(b.name, a.name) })
	kept := tops[:0]
	var truncated int64
	otherAt := -1
	for _, c := range tops {
		isOther := c.name == truncatedNodeName
		if c.total >= minValue || isOther {
			if isOther {
				otherAt = len(kept)
			}
			kept = append(kept, c)
		} else {
			truncated += c.total
		}
	}
	synthetic := -1
	if truncated > 0 {
		if otherAt >= 0 {
			kept[otherAt].p.m.addExtra(kept[otherAt].x, truncated)
		} else {
			synthetic = 0
			for synthetic < len(kept) && kept[synthetic].name > truncatedNodeName {
				synthetic++
			}
		}
	}
	at := make([]int, len(kept))
	for i, c := range kept {
		at[i] = len(c.p.roots)
		c.p.roots = append(c.p.roots, c.x)
	}
	forEach(parts, func(p *part) {
		p.spans = make([]span, len(p.roots))
		for i, x := range p.roots {
			lo := len(p.m.buf)
			p.m.write(x)
			p.spans[i] = span{lo, len(p.m.buf)}
		}
	})

	size := 32
	for _, p := range parts {
		size += len(p.m.buf)
	}
	buf := make([]byte, 0, size)
	buf = binary.AppendUvarint(buf, 0)
	buf = binary.AppendUvarint(buf, 0)
	children := len(kept)
	if synthetic >= 0 {
		children++
	}
	buf = binary.AppendUvarint(buf, uint64(children))
	writeSynthetic := func() {
		buf = binary.AppendUvarint(buf, uint64(otherIndex))
		buf = binary.AppendUvarint(buf, uint64(truncated))
		buf = binary.AppendUvarint(buf, 0)
	}
	for i, c := range kept {
		if i == synthetic {
			writeSynthetic()
		}
		s := c.p.spans[at[i]]
		buf = append(buf, c.p.m.buf[s.lo:s.hi]...)
	}
	if synthetic == len(kept) {
		writeSynthetic()
	}
	return buf, table
}

func forEach[T any](items []T, fn func(T)) {
	var wg sync.WaitGroup
	for _, item := range items {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn(item)
		}()
	}
	wg.Wait()
}

func (f *FlatTreeParts) each(groups [][]int32, fn func(*FlatTree, []int32)) {
	each(f, groups, func(_ int, t *FlatTree, roots []int32) { fn(t, roots) })
}

func each[T any](f *FlatTreeParts, groups [][]T, fn func(int, *FlatTree, []T)) {
	var wg sync.WaitGroup
	for k, g := range groups {
		if len(g) == 0 {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := &f.parts[k]
			p.Lock()
			defer p.Unlock()
			fn(k, p.t, g)
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

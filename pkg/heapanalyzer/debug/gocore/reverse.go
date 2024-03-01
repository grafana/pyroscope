// Copyright 2017 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gocore

import (
	"sort"

	"github.com/grafana/pyroscope/pkg/heapanalyzer/debug/core"
)

func (p *Process) reverseEdges() {
	p.initReverseEdges.Do(func() {
		// First, count the number of edges into each object.
		// This allows for efficient packing of the reverse edge storage.
		cnt := make([]int64, p.nObj+1)
		p.ForEachObject(func(x Object) bool {
			p.ForEachPtr(x, func(_ int64, y Object, _ int64) bool {
				idx, _ := p.findObjectIndex(p.Addr(y))
				cnt[idx]++
				return true
			})
			return true
		})
		p.ForEachRoot(func(r *Root) bool {
			p.ForEachRootPtr(r, func(_ int64, y Object, _ int64) bool {
				idx, _ := p.findObjectIndex(p.Addr(y))
				cnt[idx]++
				return true
			})
			return true
		})

		// Compute cumulative count of all incoming edges up to and including each object.
		var n int64
		for idx, c := range cnt {
			n += c
			cnt[idx] = n
		}

		// Allocate all the storage for the reverse edges.
		p.redge = make([]core.Address, n)

		// Add edges to the lists.
		p.ForEachObject(func(x Object) bool {
			p.ForEachPtr(x, func(i int64, y Object, _ int64) bool {
				idx, _ := p.findObjectIndex(p.Addr(y))
				e := cnt[idx]
				e--
				cnt[idx] = e
				p.redge[e] = p.Addr(x).Add(i)
				return true
			})
			return true
		})
		p.ForEachRoot(func(r *Root) bool {
			p.ForEachRootPtr(r, func(i int64, y Object, _ int64) bool {
				idx, _ := p.findObjectIndex(p.Addr(y))
				e := cnt[idx]
				e--
				cnt[idx] = e
				p.redge[e] = r.Addr.Add(i)
				return true
			})
			return true
		})
		// At this point, cnt contains the cumulative count of all edges up to
		// but *not* including each object.
		p.ridx = cnt

		// Make root index.
		p.ForEachRoot(func(r *Root) bool {
			p.rootIdx = append(p.rootIdx, r)
			return true
		})
		sort.Slice(p.rootIdx, func(i, j int) bool { return p.rootIdx[i].Addr < p.rootIdx[j].Addr })
	})
}

// ForEachReversePtr calls fn for all pointers it finds pointing to y.
// It calls fn with:
//
//	the object or root which points to y (exactly one will be non-nil)
//	the offset i in that object or root where the pointer appears.
//	the offset j in y where the pointer points.
//
// If fn returns false, ForEachReversePtr returns immediately.
func (p *Process) ForEachReversePtr(y Object, fn func(x Object, r *Root, i, j int64) bool) {
	p.reverseEdges()

	idx, _ := p.findObjectIndex(p.Addr(y))
	for _, a := range p.redge[p.ridx[idx]:p.ridx[idx+1]] {
		// Read pointer, compute offset in y.
		ptr := p.proc.ReadPtr(a)
		j := ptr.Sub(p.Addr(y))

		// Find source of pointer.
		x, i := p.FindObject(a)
		if x != 0 {
			// Source is an object.
			if !fn(x, nil, i, j) {
				return
			}
			continue
		}
		// Source is a root.
		k := sort.Search(len(p.rootIdx), func(k int) bool {
			r := p.rootIdx[k]
			return a < r.Addr.Add(r.Type.Size)
		})
		r := p.rootIdx[k]
		if !fn(0, r, a.Sub(r.Addr), j) {
			return
		}
	}
}

func (p *Process) findRootIndex(r *Root) int {
	return sort.Search(len(p.rootIdx), func(k int) bool {
		return p.rootIdx[k].Addr >= r.Addr
	})
}

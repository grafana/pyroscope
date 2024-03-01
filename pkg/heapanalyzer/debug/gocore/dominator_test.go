// Copyright 2018 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package gocore

import (
	"fmt"
	"os"
	"testing"
)

func TestLT(t *testing.T) {
	p := loadExample(t)
	lt := runLT(p)
	sanityCheck(t, lt)
	if false {
		lt.dot(os.Stdout)
	}
}

func TestDominators(t *testing.T) {
	p := loadExample(t)
	d := p.calculateDominators()
	if size := d.size[pseudoRoot]; size < 100<<10 {
		t.Errorf("total size of objects is only %v bytes, should be >100KiB", size)
	}
}

func sanityCheck(t *testing.T, d ltDom) bool {
	t.Helper()
	// Build pointer-y graph.
	pRoot := sanityVertex{}
	roots := make([]sanityVertex, d.nRoots)
	objects := make([]sanityVertex, d.p.nObj)

	for i, r := range d.p.rootIdx {
		v := &roots[i]
		v.root = r
		pRoot.succ = append(pRoot.succ, v)
		v.pred = append(v.pred, &pRoot)
		d.p.ForEachRootPtr(r, func(_ int64, x Object, _ int64) bool {
			idx, _ := d.p.findObjectIndex(d.p.Addr(x))
			v.succ = append(v.succ, &objects[idx])
			objects[idx].pred = append(objects[idx].pred, v)
			return true
		})
	}
	d.p.ForEachObject(func(x Object) bool {
		xIdx, _ := d.p.findObjectIndex(d.p.Addr(x))
		v := &objects[xIdx]
		v.obj = x
		d.p.ForEachPtr(x, func(_ int64, y Object, _ int64) bool {
			yIdx, _ := d.p.findObjectIndex(d.p.Addr(y))
			v.succ = append(v.succ, &objects[yIdx])
			objects[yIdx].pred = append(objects[yIdx].pred, v)
			return true
		})
		return true
	})

	// Precompute postorder traversal.
	var postorder []*sanityVertex
	type workItem struct {
		v    *sanityVertex
		mode dfsMode
	}
	seen := make(map[*sanityVertex]bool, d.nVertices)
	work := []workItem{{&pRoot, down}}
	for len(work) > 0 {
		item := &work[len(work)-1]

		if item.mode == down && len(item.v.succ) != 0 {
			item.mode = up
			for _, w := range item.v.succ {
				// Only push each node once.
				if seen[w] {
					continue
				}
				seen[w] = true

				work = append(work, workItem{w, down})
			}
			continue
		}

		work = work[:len(work)-1]
		postorder = append(postorder, item.v)
	}

	// Make map from block id to order index (for intersect call)
	postnum := make(map[*sanityVertex]int, d.nVertices)
	for i, b := range postorder {
		postnum[b] = i
	}

	// Make the pseudo-root a self-loop
	pRoot.idom = &pRoot
	if postnum[&pRoot] != len(postorder)-1 {
		panic("pseudo-root not last in postorder")
	}

	// Compute relaxation of idom entries
	for {
		changed := false

		for i := len(postorder) - 2; i >= 0; i-- {
			v := postorder[i]
			var d *sanityVertex

			for _, pred := range v.pred {
				if pred.idom == nil {
					continue
				}

				if d == nil {
					d = pred
					continue
				}

				d = intersect(d, pred, postnum)
			}
			if v.idom != d {
				v.idom = d
				changed = true
			}
		}

		if !changed {
			break
		}
	}

	pRoot.idom = nil

	getVertex := func(n vName) *sanityVertex {
		r, o := d.findVertexByName(n)
		switch {
		case n == pseudoRoot:
			return &pRoot
		case r != nil:
			return &roots[d.p.findRootIndex(r)]
		default:
			idx, _ := d.p.findObjectIndex(d.p.Addr(o))
			return &objects[idx]
		}
	}

	matches := true
	for vertName, domName := range d.idom {
		if vName(vertName) == pseudoRoot {
			continue
		}
		vert := getVertex(vName(vertName))
		dom := getVertex(domName)

		if vert.idom != dom {
			matches = false
			t.Errorf("Mismatch in idom for %v, name #%04v: fast reports %v, sanity reports %v\n", vert.String(d.p), vertName, dom.String(d.p), vert.idom.String(d.p))
		}
	}
	return matches
}

func intersect(v, w *sanityVertex, postnum map[*sanityVertex]int) *sanityVertex {
	for v != w {
		if postnum[v] < postnum[w] {
			v = v.idom
		} else {
			w = w.idom
		}
	}
	return v
}

type sanityVertex struct {
	root *Root
	obj  Object
	pred []*sanityVertex
	succ []*sanityVertex
	idom *sanityVertex
}

func (v *sanityVertex) String(p *Process) string {
	switch {
	case v.root != nil:
		return fmt.Sprintf("root %s %#x (type %s)", v.root.Name, v.root.Addr, v.root.Type)
	case v.obj != 0:
		typ, _ := p.Type(v.obj)
		var typeName string
		if typ != nil {
			typeName = typ.Name
		}
		return fmt.Sprintf("object %#x (type %s)", v.obj, typeName)
	default:
		return "pseudo-root"
	}
}

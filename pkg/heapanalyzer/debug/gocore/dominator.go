// Copyright 2018 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gocore

import (
	"fmt"
	"io"
)

// Code liberally adapted from cmd/compile/internal/ssa/dom.go and
// x/tools/go/ssa/dom.go.
//
// We use the algorithm described in Lengauer & Tarjan. 1979.  A fast
// algorithm for finding dominators in a flowgraph.
// http://doi.acm.org/10.1145/357062.357071
//
// We also apply the optimizations to SLT described in Georgiadis et
// al, Finding Dominators in Practice, JGAA 2006,
// http://jgaa.info/accepted/2006/GeorgiadisTarjanWerneck2006.10.1.pdf
// to avoid the need for buckets of size > 1.

// Vertex name, as used in the papers.
// 0 -> the pseudo-root, a made-up object that parents all the GC roots.
// 1...nRoots -> a root, found at p.rootIdx[#-1]
// nRoots+1... -> an object, with object index # - nRoots - 1
type vName int

const pseudoRoot vName = 0

// Vertex number, assigned in the DFS traversal in step 1.
type vNumber int

type ltDom struct {
	p *Process

	// mapping from object ID to object
	objs []Object

	// number -> name
	vertices []vName

	// name -> parent name
	parents []vName

	// name -> vertex number before step 2 or semidominator number after.
	semis []vNumber

	// name -> ancestor name
	ancestor []vName
	labels   []vName

	// name -> dominator name
	idom []vName

	nVertices, nRoots int
}

type dominators struct {
	p *Process

	// mapping from object ID to object
	objs []Object

	// name -> dominator name
	idom []vName

	// Reverse dominator tree edges, stored just like the ones in Process. name -> child name.
	ridx  []int
	redge []vName

	// Retained size for each vertex. name -> retained size.
	size []int64
}

func (p *Process) calculateDominators() *dominators {
	lt := runLT(p)
	d := dominators{p: p, idom: lt.idom, objs: lt.objs}
	lt = ltDom{}

	d.reverse()
	d.calcSize(p)

	return &d
}

func runLT(p *Process) ltDom {
	p.typeHeap()
	p.reverseEdges()

	nVertices := 1 + len(p.rootIdx) + p.nObj
	lt := ltDom{
		p:         p,
		nRoots:    len(p.rootIdx),
		nVertices: nVertices,
		objs:      make([]Object, p.nObj),
		vertices:  make([]vName, nVertices),
		parents:   make([]vName, nVertices),
		semis:     make([]vNumber, nVertices),
		ancestor:  make([]vName, nVertices),
		labels:    make([]vName, nVertices),
		idom:      make([]vName, nVertices),
	}
	// TODO: increment all the names and use 0 as the uninitialized value.
	for i := range lt.semis {
		lt.semis[i] = -1
	}
	for i := range lt.ancestor {
		lt.ancestor[i] = -1
	}
	for i := range lt.labels {
		lt.labels[i] = vName(i)
	}
	lt.initialize()
	lt.calculate()
	return lt
}

// initialize implements step 1 of LT.
func (d *ltDom) initialize() {
	type workItem struct {
		name       vName
		parentName vName
	}

	// Initialize objs for mapping from object index back to Object.
	i := 0
	d.p.ForEachObject(func(x Object) bool {
		d.objs[i] = x
		i++
		return true
	})

	// Add roots to the work stack, essentially pretending to visit
	// the pseudo-root, numbering it 0.
	d.semis[pseudoRoot] = 0
	d.parents[pseudoRoot] = -1
	d.vertices[0] = pseudoRoot
	var work []workItem
	for i := 1; i < 1+d.nRoots; i++ {
		work = append(work, workItem{name: vName(i), parentName: 0})
	}

	n := vNumber(1) // 0 was the pseudo-root.

	// Build the spanning tree, assigning vertex numbers to each object
	// and initializing semi and parent.
	for len(work) != 0 {
		item := work[len(work)-1]
		work = work[:len(work)-1]

		if d.semis[item.name] != -1 {
			continue
		}

		d.semis[item.name] = n
		d.parents[item.name] = item.parentName
		d.vertices[n] = item.name
		n++

		visitChild := func(_ int64, child Object, _ int64) bool {
			childIdx, _ := d.p.findObjectIndex(d.p.Addr(child))
			work = append(work, workItem{name: vName(childIdx + d.nRoots + 1), parentName: item.name})
			return true
		}

		root, object := d.findVertexByName(item.name)
		if root != nil {
			d.p.ForEachRootPtr(root, visitChild)
		} else {
			d.p.ForEachPtr(object, visitChild)
		}

	}
}

// findVertexByName returns the root/object named by n, or nil,0 for the pseudo-root.
func (d *ltDom) findVertexByName(n vName) (*Root, Object) {
	if n == 0 {
		return nil, 0
	}
	if int(n) < len(d.p.rootIdx)+1 {
		return d.p.rootIdx[n-1], 0
	}
	return nil, d.objs[int(n)-len(d.p.rootIdx)-1]
}
func (d *dominators) findVertexByName(n vName) (*Root, Object) {
	if n == 0 {
		return nil, 0
	}
	if int(n) < len(d.p.rootIdx)+1 {
		return d.p.rootIdx[n-1], 0
	}
	return nil, d.objs[int(n)-len(d.p.rootIdx)-1]
}

// calculate runs the main part of LT.
func (d *ltDom) calculate() {
	// name -> bucket (a name), per Georgiadis.
	buckets := make([]vName, d.nVertices)
	for i := range buckets {
		buckets[i] = vName(i)
	}

	for i := vNumber(len(d.vertices)) - 1; i > 0; i-- {
		w := d.vertices[i]

		// Step 3. Implicitly define the immediate dominator of each node.
		for v := buckets[w]; v != w; v = buckets[v] {
			u := d.eval(v)
			if d.semis[u] < d.semis[v] {
				d.idom[v] = u
			} else {
				d.idom[v] = w
			}
		}

		// Step 2. Compute the semidominators of all nodes.
		root, obj := d.findVertexByName(w)
		// This loop never visits the pseudo-root.
		if root != nil {
			u := d.eval(pseudoRoot)
			if d.semis[u] < d.semis[w] {
				d.semis[w] = d.semis[u]
			}
		} else {
			d.p.ForEachReversePtr(obj, func(x Object, r *Root, _, _ int64) bool {
				var v int
				if r != nil {
					v = d.p.findRootIndex(r) + 1
				} else {
					v, _ = d.p.findObjectIndex(d.p.Addr(x))
					v += d.nRoots + 1
				}
				u := d.eval(vName(v))
				if d.semis[u] < d.semis[w] {
					d.semis[w] = d.semis[u]
				}
				return true
			})
		}

		d.link(d.parents[w], w)

		if d.parents[w] == d.vertices[d.semis[w]] {
			d.idom[w] = d.parents[w]
		} else {
			buckets[w] = buckets[d.vertices[d.semis[w]]]
			buckets[d.vertices[d.semis[w]]] = w
		}
	}

	// The final 'Step 3' is now outside the loop.
	for v := buckets[pseudoRoot]; v != pseudoRoot; v = buckets[v] {
		d.idom[v] = pseudoRoot
	}

	// Step 4. Explicitly define the immediate dominator of each
	// node, in preorder.
	for _, w := range d.vertices[1:] {
		if d.idom[w] != d.vertices[d.semis[w]] {
			d.idom[w] = d.idom[d.idom[w]]
		}
	}
}

// eval is EVAL from the papers.
func (d *ltDom) eval(v vName) vName {
	if d.ancestor[v] == -1 {
		return v
	}
	d.compress(v)
	return d.labels[v]
}

// compress is COMPRESS from the papers.
func (d *ltDom) compress(v vName) {
	var stackBuf [20]vName
	stack := stackBuf[:0]
	for d.ancestor[d.ancestor[v]] != -1 {
		stack = append(stack, v)
		v = d.ancestor[v]
	}

	for len(stack) != 0 {
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if d.semis[d.labels[d.ancestor[v]]] < d.semis[d.labels[v]] {
			d.labels[v] = d.labels[d.ancestor[v]]
		}
		d.ancestor[v] = d.ancestor[d.ancestor[v]]
	}
}

// link is LINK from the papers.
func (d *ltDom) link(v, w vName) {
	d.ancestor[w] = v
}

// reverse computes and stores reverse edges for each vertex.
func (d *dominators) reverse() {
	// One inbound edge per vertex. Then we need an extra so that you can
	// always look at ridx[i+1], and another for working storage while
	// populating redge.
	cnt := make([]int, len(d.idom)+2)

	// Fill cnt[2:] with the number of outbound edges for each vertex.
	tmp := cnt[2:]
	for _, idom := range d.idom {
		tmp[idom]++
	}

	// Make tmp cumulative. After this step, cnt[1:] is what we want for
	// ridx, but the next step messes it up.
	var n int
	for idx, c := range tmp {
		n += c
		tmp[idx] = n
	}

	// Store outbound edges in redge, using cnt[1:] as the index to store
	// the next edge for each vertex. After we're done, everything's been
	// shifted over one, and cnt is ridx.
	redge := make([]vName, len(d.idom))
	tmp = cnt[1:]
	for i, idom := range d.idom {
		redge[tmp[idom]] = vName(i)
		tmp[idom]++
	}
	d.redge, d.ridx = redge, cnt[:len(cnt)-1]
}

type dfsMode int

const (
	down dfsMode = iota
	up
)

// calcSize calculates the total retained size for each vertex.
func (d *dominators) calcSize(p *Process) {
	d.size = make([]int64, len(d.idom))
	type workItem struct {
		v    vName
		mode dfsMode
	}
	work := []workItem{{pseudoRoot, down}}

	for len(work) > 0 {
		item := &work[len(work)-1]

		kids := d.redge[d.ridx[item.v]:d.ridx[item.v+1]]
		if item.mode == down && len(kids) != 0 {
			item.mode = up
			for _, w := range kids {
				if w == 0 {
					// bogus self-edge. Ignore.
					continue
				}
				work = append(work, workItem{w, down})
			}
			continue
		}

		work = work[:len(work)-1]

		root, obj := d.findVertexByName(item.v)
		var size int64
		switch {
		case item.v == pseudoRoot:
			break
		case root != nil:
			size += root.Type.Size
		default:
			size += p.Size(obj)
		}
		for _, w := range kids {
			size += d.size[w]
		}
		d.size[item.v] = size
	}
}

func (d *ltDom) dot(w io.Writer) {
	fmt.Fprintf(w, "digraph %s {\nrankdir=\"LR\"\n", "dominators")
	for number, name := range d.vertices {
		var label string
		root, obj := d.findVertexByName(name)

		switch {
		case name == 0:
			label = "pseudo-root"
		case root != nil:
			typeName := root.Type.Name
			if len(typeName) > 30 {
				typeName = typeName[:30]
			}
			label = fmt.Sprintf("root %s %#x (type %s)", root.Name, root.Addr, typeName)
		default:
			typ, _ := d.p.Type(obj)
			var typeName string
			if typ != nil {
				typeName = typ.Name
				if len(typeName) > 30 {
					typeName = typeName[:30]
				}
			}
			label = fmt.Sprintf("object %#x (type %s)", obj, typeName)
		}

		fmt.Fprintf(w, "\t%v [label=\"name #%04v, number #%04v: %s\"]\n", name, name, number, label)
	}

	fmt.Fprint(w, "\n\n")
	for v, parent := range d.parents {
		fmt.Fprintf(w, "\t%v -> %v [style=\"solid\"]\n", parent, v)
	}
	for v, idom := range d.idom {
		fmt.Fprintf(w, "\t%v -> %v [style=\"bold\"]\n", idom, v)
	}
	for v, sdom := range d.semis {
		fmt.Fprintf(w, "\t%v -> %v [style=\"dotted\"]\n", v, d.vertices[sdom])
	}
	fmt.Fprint(w, "}\n")
}

package segment

import (
	"fmt"
	"time"
)

type streeNode struct {
	depth    int
	time     time.Time
	present  bool
	samples  uint64
	children []*streeNode
}

var durations []time.Duration

func (parent *streeNode) replace(child *streeNode) {
	i := child.time.Sub(parent.time) / durations[child.depth]
	parent.children[i] = child
}

func ft(t time.Time) string {
	return fmt.Sprintf("%d", t.Sub(time.Time{})/1000000000)
}

func calcMultiplier(m, d int) (r int) {
	r = 1
	for i := 0; i < d; i++ {
		r *= m
	}
	return
}

func (sn *streeNode) relationship(st, et time.Time) rel {
	t2 := sn.time.Add(durations[sn.depth])
	return relationship(sn.time, t2, st, et)
}

func (sn *streeNode) findAddons() []Addon {
	res := []Addon{}
	if sn.present {
		res = append(res, Addon{
			Depth: sn.depth,
			T:     sn.time,
		})
	} else {
		for _, child := range sn.children {
			if child != nil {
				res = append(res, child.findAddons()...)
			}
		}
	}
	return res
}
func (sn *streeNode) put(st, et time.Time, samples uint64, cb func(n *streeNode, depth int, dt time.Time, addons []Addon)) {
	nodes := []*streeNode{sn}

	for len(nodes) > 0 {
		sn = nodes[0]
		nodes = nodes[1:]

		rel := sn.relationship(st, et)
		if rel != outside {
			childrenCount := 0
			createNewChildren := rel == inside || rel == overlap
			for i, v := range sn.children {
				if createNewChildren && v == nil { // maybe create a new child
					childT := sn.time.Truncate(durations[sn.depth]).Add(time.Duration(i) * durations[sn.depth-1])

					rel2 := relationship(childT, childT.Add(durations[sn.depth-1]), st, et)
					if rel2 != outside {
						sn.children[i] = newNode(childT, sn.depth-1, 10)
					}
				}

				if sn.children[i] != nil {
					childrenCount++
					nodes = append(nodes, sn.children[i])
				}
			}
			var addons []Addon
			if rel == match || rel == contain || childrenCount > 1 || sn.present {
				// TODO: if has children and not present need to pass child
				if !sn.present {
					addons = sn.findAddons()
				}
				cb(sn, sn.depth, sn.time, addons)
				sn.present = true
			}
		}
	}
}

func normalize(st, et time.Time) (time.Time, time.Time) {
	st = st.Truncate(durations[0])
	et2 := et.Truncate(durations[0])
	if et2.Equal(et) {
		return st, et
	}
	return st, et2.Add(durations[0])
}

func (sn *streeNode) get(st, et time.Time, cb func(sn *streeNode, d int, t time.Time)) {
	rel := sn.relationship(st, et)
	if sn.present && (rel == contain || rel == match) {
		cb(sn, sn.depth, sn.time)
	} else if rel != outside {
		// TODO: for ranges that are not covered by children need to use values from this node
		//   but add a multiplier
		for _, v := range sn.children {
			if v != nil {
				v.get(st, et, cb)
			}
		}
	}
}

type Segment struct {
	resolution time.Duration
	multiplier int
	root       *streeNode
	durations  []time.Duration
}

func newNode(t time.Time, depth, multiplier int) *streeNode {
	sn := &streeNode{
		depth: depth,
		time:  t,
	}
	if depth > 0 {
		sn.children = make([]*streeNode, multiplier)
	}
	return sn
}

// TODO: global state is not good
func InitializeGlobalState(resolution time.Duration, multiplier int) {
	// this is here just to initialize global duration variable
	New(resolution, multiplier)
}

func New(resolution time.Duration, multiplier int) *Segment {
	st := &Segment{
		resolution: resolution,
		multiplier: multiplier,
		durations:  []time.Duration{},
	}

	// TODO better upper boundary
	d := resolution
	for i := 0; i < 50; i++ {
		st.durations = append(st.durations, d)
		d *= time.Duration(multiplier)
	}
	durations = st.durations

	return st
}

// TODO: DRY
func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func (s *Segment) growTree(st, et time.Time) {
	var prevVal *streeNode
	if s.root != nil {
		st = minTime(st, s.root.time)
		et = maxTime(et, s.root.time)
	} else {
		st = st.Truncate(s.durations[0])
		s.root = newNode(st, 0, s.multiplier)
	}

	for {
		rel := s.root.relationship(st, et)

		if rel == inside || rel == match {
			break
		}

		prevVal = s.root
		newDepth := prevVal.depth + 1
		s.root = newNode(st.Truncate(s.durations[newDepth]), newDepth, s.multiplier)
		if prevVal != nil {
			s.root.samples = prevVal.samples
			s.root.replace(prevVal)
		}
	}
}

type Addon struct {
	Depth int
	T     time.Time
}

// TODO: just give d+t info here
func (s *Segment) Put(st, et time.Time, samples uint64, cb func(depth int, t time.Time, m, d int, addons []Addon)) {
	st, et = normalize(st, et)
	s.growTree(st, et)
	d := uint64(int(et.Sub(st) / durations[0]))
	v := newVis()
	s.root.put(st, et, samples, func(sn *streeNode, depth int, tm time.Time, addons []Addon) {
		m := uint64(calcMultiplier(s.multiplier, depth))
		sn.samples += samples
		// case when not all children are within [st,et]
		// TODO: maybe we need childrenCount be in durations[0] terms
		v.add(sn, int(m), int(d), true)
		cb(depth, tm, int(m), int(d), addons)
	})
	v.print(fmt.Sprintf("/tmp/0-put-%s-%s.html", st.String(), et.String()))
}

func (s *Segment) Get(st, et time.Time, cb func(depth int, t time.Time, m, d int)) {
	st, et = normalize(st, et)
	if s.root == nil {
		return
	}
	// divider := int(et.Sub(st) / durations[0])
	v := newVis()
	s.root.get(st, et, func(sn *streeNode, depth int, t time.Time) {
		// TODO: pass m / d from .get() ?
		m := 1
		d := 1
		v.add(sn, m, d, true)
		cb(depth, t, m, d)

	})
	v.print(fmt.Sprintf("/tmp/0-get-%s-%s.html", st.String(), et.String()))
}

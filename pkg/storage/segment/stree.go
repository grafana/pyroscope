package segment

import (
	"fmt"
	"time"
)

type streeNode struct {
	depth    int
	time     time.Time
	present  bool
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

func (sn *streeNode) put(st, et time.Time, cb func(int, int, time.Time)) {
	nodes := []*streeNode{sn}

	for len(nodes) > 0 {
		sn = nodes[0]
		nodes = nodes[1:]

		if isInside(sn.time, st, et, durations[sn.depth]) {
			// TODO: merges / etc
			cb(-1, sn.depth, sn.time)
			sn.present = true
		} else if isNotOutside(sn.time, st, et, durations[sn.depth]) {
			childrenCount := 0
			for i, v := range sn.children {
				if v != nil {
					childrenCount++
					nodes = append(nodes, v)
				} else {
					childT := sn.time.Truncate(durations[sn.depth]).Add(time.Duration(i) * durations[sn.depth-1])
					b := isNotOutside(childT, st, et, durations[sn.depth-1])
					if b {
						// TODO: pass multiplier
						sn.children[i] = newNode(childT, sn.depth-1, 10)
						nodes = append(nodes, sn.children[i])
						childrenCount++
					}
				}
			}
			if childrenCount > 1 {
				// TODO: this actually requires a merge
				cb(childrenCount, sn.depth, sn.time)
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

func (sn *streeNode) get(st, et time.Time, cb func(d int, t time.Time)) {
	rel := relationship(sn.time, sn.time.Add(durations[sn.depth]), st, et)
	if sn.present && (rel == inside || rel == match) {
		cb(sn.depth, sn.time)
	} else if rel != outside {
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

func newNode(t time.Time, d, multiplier int) *streeNode {
	sn := &streeNode{
		depth: d,
		time:  t,
	}
	if d > 0 {
		sn.children = make([]*streeNode, 10)
	}
	return sn
}

// move link to tries to strees
func New(resolution time.Duration, multiplier int) *Segment {
	st := &Segment{
		resolution: resolution,
		multiplier: multiplier,
		durations:  []time.Duration{},
	}

	// TODO: global state is not good

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
		if isMatchOrContain(s.root.time, st, et, s.durations[s.root.depth]) {
			break
		}

		prevVal = s.root
		newDepth := prevVal.depth + 1
		s.root = newNode(st.Truncate(s.durations[newDepth]), newDepth, s.multiplier)
		if prevVal != nil {
			s.root.replace(prevVal)
		}
	}
}

// TODO: just give d+t info here
func (s *Segment) Put(st, et time.Time, cb func(depth int, t time.Time, m, d int)) {
	st, et = normalize(st, et)
	s.growTree(st, et)
	divider := int(et.Sub(st) / durations[0])
	count := 0
	s.root.put(st, et, func(childrenCount int, depth int, tm time.Time) {
		count++

		extraM := 1
		extraD := 1
		if childrenCount != -1 && childrenCount != s.multiplier {
			// TODO: Use multiplier + divider better
			extraM = childrenCount
			extraD = s.multiplier
		}
		// case when not all children are within [st,et]
		// TODO: maybe we need childrenCount be in durations[0] terms
		cb(depth, tm, calcMultiplier(s.multiplier, depth)*extraM, divider*extraD)
	})
}

func (s *Segment) Get(st, et time.Time, cb func(d int, t time.Time)) {
	st, et = normalize(st, et)
	if s.root == nil {
		return
	}
	s.root.get(st, et, func(d int, t time.Time) {
		cb(d, t)
	})
	// TODO: remove this, it's temporary
	// s.Visualize()
}

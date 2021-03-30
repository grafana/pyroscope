package segment

import (
	"fmt"
	"math/big"
	"sync"
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

func (sn *streeNode) relationship(st, et time.Time) rel {
	t2 := sn.time.Add(durations[sn.depth])
	return relationship(sn.time, t2, st, et)
}

func (sn *streeNode) endTime() time.Time {
	return sn.time.Add(durations[sn.depth])
}

func (sn *streeNode) overlapRead(st, et time.Time) *big.Rat {
	t2 := sn.time.Add(durations[sn.depth])
	return overlapRead(sn.time, t2, st, et, durations[0])
}

func (sn *streeNode) overlapWrite(st, et time.Time) *big.Rat {
	t2 := sn.time.Add(durations[sn.depth])
	return overlapWrite(sn.time, t2, st, et, durations[0])
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

func (sn *streeNode) put(st, et time.Time, samples uint64, cb func(n *streeNode, depth int, dt time.Time, r *big.Rat, addons []Addon)) {
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

			r := sn.overlapWrite(st, et)
			fv, _ := r.Float64()
			sn.samples += uint64(float64(samples) * fv)

			//  relationship                               overlap read             overlap write
			// 	inside  rel = iota   // | S E |            <1                       1/1
			// 	match                // matching ranges    1/1                      1/1
			// 	outside              // | | S E            0/1                      0/1
			// 	overlap              // | S | E            <1                       <1
			// 	contain              // S | | E            1/1                      <1

			if rel == match || rel == contain || childrenCount > 1 || sn.present {
				if !sn.present {
					addons = sn.findAddons()
				}

				cb(sn, sn.depth, sn.time, r, addons)
				sn.present = true
			}
		}
	}
}

func normalize(st, et time.Time) (time.Time, time.Time) {
	st = st.Truncate(durations[0])
	et2 := et.Truncate(durations[0])
	if et2.Equal(et) && !st.Equal(et2) {
		return st, et
	}
	return st, et2.Add(durations[0])
}

//  relationship                               overlap read             overlap write
// 	inside  rel = iota   // | S E |            <1                       1/1
// 	match                // matching ranges    1/1                      1/1
// 	outside              // | | S E            0/1                      0/1
// 	overlap              // | S | E            <1                       <1
// 	contain              // S | | E            1/1                      <1
func (sn *streeNode) get(st, et time.Time, cb func(sn *streeNode, d int, t time.Time, r *big.Rat)) {
	rel := sn.relationship(st, et)
	if sn.present && (rel == contain || rel == match) {
		cb(sn, sn.depth, sn.time, big.NewRat(1, 1))
	} else if rel != outside { // inside or overlap
		if sn.present && len(sn.children) == 0 {
			// TODO: I did not test this logic as extensively as I would love to.
			//   See https://github.com/pyroscope-io/pyroscope/issues/28 for more context and ideas on what to do
			cb(sn, sn.depth, sn.time, sn.overlapRead(st, et))
		} else {
			// if current node doesn't have a tree present or has children, defer to children
			for _, v := range sn.children {
				if v != nil {
					v.get(st, et, cb)
				}
			}
		}
	}
}

type Segment struct {
	m          sync.RWMutex
	resolution time.Duration
	multiplier int
	root       *streeNode
	durations  []time.Duration

	spyName    string
	sampleRate int
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
		et = maxTime(et, s.root.endTime())
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
		s.root = newNode(prevVal.time.Truncate(s.durations[newDepth]), newDepth, s.multiplier)
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
func (s *Segment) Put(st, et time.Time, samples uint64, cb func(depth int, t time.Time, r *big.Rat, addons []Addon)) {
	s.m.Lock()
	defer s.m.Unlock()

	st, et = normalize(st, et)
	s.growTree(st, et)
	v := newVis()
	s.root.put(st, et, samples, func(sn *streeNode, depth int, tm time.Time, r *big.Rat, addons []Addon) {
		v.add(sn, r, true)
		cb(depth, tm, r, addons)
	})
	v.print(fmt.Sprintf("/tmp/0-put-%s-%s.html", st.String(), et.String()))
}

func (s *Segment) Get(st, et time.Time, cb func(depth int, samples uint64, t time.Time, r *big.Rat)) {
	s.m.RLock()
	defer s.m.RUnlock()

	st, et = normalize(st, et)
	if s.root == nil {
		return
	}
	// divider := int(et.Sub(st) / durations[0])
	v := newVis()
	s.root.get(st, et, func(sn *streeNode, depth int, t time.Time, r *big.Rat) {
		// TODO: pass m / d from .get() ?
		v.add(sn, r, true)
		cb(depth, sn.samples, t, r)
	})
	v.print(fmt.Sprintf("/tmp/0-get-%s-%s.html", st.String(), et.String()))
}

// TODO: this should be refactored

func (s *Segment) SetMetadata(spyName string, sampleRate int) {
	s.spyName = spyName
	s.sampleRate = sampleRate
}

func (s *Segment) SpyName() string {
	return s.spyName
}

func (s *Segment) SampleRate() int {
	return s.sampleRate
}

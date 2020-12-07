package segment

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
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

func (sn *streeNode) put(st, et time.Time, samples uint64, cb func(n *streeNode, childrenCount int, depth int, dt time.Time)) {
	nodes := []*streeNode{sn}

	for len(nodes) > 0 {
		sn = nodes[0]
		nodes = nodes[1:]

		rel := sn.relationship(st, et)
		if rel == match || rel == contain {
			// TODO: need to add weights here
			// TODO: if has children and not present need to merge with a child
			cb(sn, -1, sn.depth, sn.time)
			sn.present = true
		} else if rel == inside || rel == overlap { // the one left is "outside"
			childrenCount := 0
			for i, v := range sn.children {
				if v != nil {
					childrenCount++
					nodes = append(nodes, v)
				} else {
					childT := sn.time.Truncate(durations[sn.depth]).Add(time.Duration(i) * durations[sn.depth-1])

					rel := relationship(childT, childT.Add(durations[sn.depth-1]), st, et)
					if rel != outside {
						sn.children[i] = newNode(childT, sn.depth-1, 10)
						nodes = append(nodes, sn.children[i])
						childrenCount++
					}
				}
			}
			if childrenCount > 1 {
				// TODO: need to add weights here
				cb(sn, childrenCount, sn.depth, sn.time)
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
	rel := sn.relationship(st, et)
	if sn.present && (rel == contain || rel == match) {
		cb(sn.depth, sn.time)
	} else if rel == inside || rel == overlap { // same as rel != outside
		for _, v := range sn.children {
			if v != nil {
				v.get(st, et, cb)
			}
		}
	}
}

func (sn *streeNode) generateTimeline(st, et time.Time, minDuration time.Duration, buf [][]uint64) {
	rel := sn.relationship(st, et)
	logrus.WithFields(logrus.Fields{
		"_t0":         st.String(),
		"_t1":         et.String(),
		"_t2":         sn.time.String(),
		"_sn.depth":   sn.depth,
		"minDuration": minDuration.String(),
		"rel":         rel,
	}).Info("generateTimeline")
	if rel != outside {
		currentDuration := durations[sn.depth]
		if len(sn.children) > 0 && currentDuration >= minDuration {
			for _, v := range sn.children {
				if v != nil {
					v.generateTimeline(st, et, minDuration, buf)
				}
			}
		}

		i := int(sn.time.Sub(st) / minDuration)
		rightBoundary := i + int(currentDuration/minDuration)
		l := len(buf)
		for i < rightBoundary {
			if i >= 0 && i < l {
				buf[i][1] += sn.samples
			}
			i++
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
		rel := s.root.relationship(st, et)

		if rel == inside || rel == match {
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
func (s *Segment) Put(st, et time.Time, samples uint64, cb func(depth int, t time.Time, m, d int)) {
	st, et = normalize(st, et)
	s.growTree(st, et)
	divider := int(et.Sub(st) / durations[0])
	s.root.put(st, et, samples, func(sn *streeNode, childrenCount int, depth int, tm time.Time) {
		extraM := 1
		extraD := 1
		if childrenCount != -1 && childrenCount != s.multiplier {
			// TODO: Use multiplier + divider better
			extraM = childrenCount
			extraD = s.multiplier
		}
		m := uint64(calcMultiplier(s.multiplier, depth) * extraM)
		d := uint64(divider * extraD)
		sn.samples += samples * m / d
		// case when not all children are within [st,et]
		// TODO: maybe we need childrenCount be in durations[0] terms
		cb(depth, tm, int(m), int(d))
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
}

func (s *Segment) GenerateTimeline(st, et time.Time) [][]uint64 {
	st, et = normalize(st, et)
	if s.root == nil {
		return [][]uint64{}
	}

	totalDuration := et.Sub(st)
	minDuration := totalDuration / time.Duration(2048/2)
	durThreshold := s.durations[0]
	for _, d := range s.durations {
		if d < 0 {
			break
		}
		if d < minDuration {
			durThreshold = d
		}
	}

	// TODO: need to figure out optimal size
	res := make([][]uint64, totalDuration/durThreshold)
	currentTime := st
	for i, _ := range res {
		// uint64(rand.Intn(100))
		res[i] = []uint64{uint64(currentTime.Unix() * 1000), 0}
		currentTime = currentTime.Add(durThreshold)
	}
	s.root.generateTimeline(st, et, durThreshold, res)

	logrus.WithField("min", minDuration).WithField("threshold", durThreshold).Debug("duration")

	return res
}

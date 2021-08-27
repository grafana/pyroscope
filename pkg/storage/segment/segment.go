package segment

import (
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type streeNode struct {
	depth    int
	time     time.Time
	present  bool
	samples  uint64
	writes   uint64
	children []*streeNode
}

func (sn *streeNode) replace(child *streeNode) {
	i := child.time.Sub(sn.time) / durations[child.depth]
	sn.children[i] = child
}

func (sn *streeNode) relationship(st, et time.Time) rel {
	t2 := sn.time.Add(durations[sn.depth])
	return relationship(sn.time, t2, st, et)
}

func (sn *streeNode) isBefore(rt time.Time) bool {
	t2 := sn.time.Add(durations[sn.depth])
	return !t2.After(rt)
}

func (sn *streeNode) isAfter(rt time.Time) bool {
	return sn.time.After(rt)
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
			sn.writes += uint64(1)

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

func normalizeTime(t time.Time) time.Time {
	return t.Truncate(durations[0])
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

// deleteDataBefore returns true if the node should be deleted
func (sn *streeNode) deleteDataBefore(retentionThreshold time.Time, cb func(depth int, t time.Time)) bool {
	if !sn.isAfter(retentionThreshold) {
		isBefore := sn.isBefore(retentionThreshold)
		if isBefore {
			cb(sn.depth, sn.time)
		}

		for i, v := range sn.children {
			if v != nil {
				deletedData := v.deleteDataBefore(retentionThreshold, cb)
				if deletedData {
					sn.children[i] = nil
				}
			}
		}
		return isBefore
	}

	return false
}

type Segment struct {
	m    sync.RWMutex
	root *streeNode

	spyName         string
	sampleRate      uint32
	units           string
	aggregationType string
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

func New() *Segment {
	st := &Segment{}

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

func (s *Segment) growTree(st, et time.Time) bool {
	var prevVal *streeNode
	if s.root != nil {
		st = minTime(st, s.root.time)
		et = maxTime(et, s.root.endTime())
	} else {
		st = st.Truncate(durations[0])
		s.root = newNode(st, 0, multiplier)
	}

	for {
		rel := s.root.relationship(st, et)

		if rel == inside || rel == match {
			break
		}

		prevVal = s.root
		newDepth := prevVal.depth + 1
		if newDepth >= len(durations) {
			return false
		}
		s.root = newNode(prevVal.time.Truncate(durations[newDepth]), newDepth, multiplier)
		if prevVal != nil {
			s.root.samples = prevVal.samples
			s.root.writes = prevVal.writes
			s.root.replace(prevVal)
		}
	}
	return true
}

type Addon struct {
	Depth int
	T     time.Time
}

var errStartTimeBeforeEndTime = errors.New("start time cannot be after end time")
var errTreeMaxSize = errors.New("segment tree reached max size, check start / end time parameters")

// TODO: simplify arguments
// TODO: validate st < et
func (s *Segment) Put(st, et time.Time, samples uint64, cb func(depth int, t time.Time, r *big.Rat, addons []Addon)) error {
	s.m.Lock()
	defer s.m.Unlock()

	st, et = normalize(st, et)
	if st.After(et) {
		return errStartTimeBeforeEndTime
	}

	if !s.growTree(st, et) {
		return errTreeMaxSize
	}
	v := newVis()
	s.root.put(st, et, samples, func(sn *streeNode, depth int, tm time.Time, r *big.Rat, addons []Addon) {
		v.add(sn, r, true)
		cb(depth, tm, r, addons)
	})
	v.print(filepath.Join(os.TempDir(), fmt.Sprintf("0-put-%s-%s.html", st.String(), et.String())))
	return nil
}

// TODO: simplify arguments
// TODO: validate st < et
func (s *Segment) Get(st, et time.Time, cb func(depth int, samples, writes uint64, t time.Time, r *big.Rat)) {
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
		cb(depth, sn.samples, sn.writes, t, r)
	})
	v.print(filepath.Join(os.TempDir(), fmt.Sprintf("0-get-%s-%s.html", st.String(), et.String())))
}

func (s *Segment) DeleteDataBefore(retentionThreshold time.Time, cb func(depth int, t time.Time)) bool {
	s.m.Lock()
	defer s.m.Unlock()

	if s.root == nil {
		return true
	}

	retentionThreshold = normalizeTime(retentionThreshold)
	shouldDeleteRoot := s.root.deleteDataBefore(retentionThreshold, func(depth int, t time.Time) {
		cb(depth, t)
	})

	if shouldDeleteRoot {
		s.root = nil
		return true
	}

	return false
}

// TODO: this should be refactored
func (s *Segment) SetMetadata(spyName string, sampleRate uint32, units, aggregationType string) {
	s.spyName = spyName
	s.sampleRate = sampleRate
	s.units = units
	s.aggregationType = aggregationType
}

func (s *Segment) SpyName() string {
	return s.spyName
}

func (s *Segment) SampleRate() uint32 {
	return s.sampleRate
}

func (s *Segment) Units() string {
	return s.units
}

func (s *Segment) AggregationType() string {
	return s.aggregationType
}

var zeroTime time.Time

func (s *Segment) StartTime() time.Time {
	if s.root == nil {
		return zeroTime
	}
	n := s.root

	for {
		if len(n.children) == 0 {
			return n.time
		}

		oldN := n

		for _, child := range n.children {
			if child != nil {
				n = child
				break
			}
		}

		if n == oldN {
			return n.time
		}
	}
}

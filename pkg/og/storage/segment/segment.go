package segment

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"runtime/trace"
	"sync"
	"time"

	"github.com/grafana/pyroscope/pkg/og/storage/metadata"
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

// get traverses through the tree searching for the nodes satisfying
// the given time range. If no nodes were found, the most precise
// down-sampling root node will be passed to the callback function,
// and relationship r will be proportional to the down-sampling factor.
//
//  relationship                               overlap read             overlap write
// 	inside  rel = iota   // | S E |            <1                       1/1
// 	match                // matching ranges    1/1                      1/1
// 	outside              // | | S E            0/1                      0/1
// 	overlap              // | S | E            <1                       <1
// 	contain              // S | | E            1/1                      <1
func (sn *streeNode) get(ctx context.Context, s *Segment, st, et time.Time, cb func(*streeNode, *big.Rat)) {
	r := sn.relationship(st, et)
	trace.Logf(ctx, traceCatNodeGet, "D=%d T=%v P=%v R=%v", sn.depth, sn.time.Unix(), sn.present, r)
	switch r {
	case outside:
		return
	case inside, overlap:
		// Defer to children.
	case contain, match:
		// Take the node as is.
		if sn.present {
			cb(sn, big.NewRat(1, 1))
			return
		}
	}
	trace.Log(ctx, traceCatNodeGet, "drill down")
	// Whether child nodes are outside the retention period.
	if sn.time.Before(s.watermarks.levels[sn.depth-1]) && sn.present {
		trace.Log(ctx, traceCatNodeGet, "sampled")
		// Create a sampled tree from the current node.
		cb(sn, sn.overlapRead(st, et))
		return
	}
	// Traverse nodes recursively.
	for _, v := range sn.children {
		if v != nil {
			v.get(ctx, s, st, et, cb)
		}
	}
}

// deleteDataBefore returns true if the node should be deleted.
func (sn *streeNode) deleteNodesBefore(t *RetentionPolicy) (bool, error) {
	if sn.isAfter(t.AbsoluteTime) && t.Levels == nil {
		return false, nil
	}
	remove := t.isToBeDeleted(sn)
	for i, v := range sn.children {
		if v == nil {
			continue
		}
		ok, err := v.deleteNodesBefore(t)
		if err != nil {
			return false, err
		}
		if ok {
			sn.children[i] = nil
		}
	}
	return remove, nil
}

func (sn *streeNode) walkNodesToDelete(t *RetentionPolicy, cb func(depth int, t time.Time) error) (bool, error) {
	if sn.isAfter(t.AbsoluteTime) && t.Levels == nil {
		return false, nil
	}
	var err error
	remove := t.isToBeDeleted(sn)
	if remove {
		if err = cb(sn.depth, sn.time); err != nil {
			return false, err
		}
	}
	for _, v := range sn.children {
		if v == nil {
			continue
		}
		if _, err = v.walkNodesToDelete(t, cb); err != nil {
			return false, err
		}
	}
	return remove, nil
}

type Segment struct {
	m    sync.RWMutex
	root *streeNode

	spyName         string
	sampleRate      uint32
	units           metadata.Units
	aggregationType metadata.AggregationType

	watermarks
}

type watermarks struct {
	absoluteTime time.Time
	levels       map[int]time.Time
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
	return &Segment{watermarks: watermarks{
		levels: make(map[int]time.Time),
	}}
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

const (
	traceRegionGet  = "segment.Get"
	traceCatGet     = traceRegionGet
	traceCatNodeGet = "node.get"
)

//revive:disable-next-line:get-return callback
func (s *Segment) Get(st, et time.Time, cb func(depth int, samples, writes uint64, t time.Time, r *big.Rat)) {
	// TODO: simplify arguments
	// TODO: validate st < et
	s.GetContext(context.Background(), st, et, cb)
}

//revive:disable-next-line:get-return callback
func (s *Segment) GetContext(ctx context.Context, st, et time.Time, cb func(depth int, samples, writes uint64, t time.Time, r *big.Rat)) {
	defer trace.StartRegion(ctx, traceRegionGet).End()
	s.m.RLock()
	defer s.m.RUnlock()
	if st.Before(s.watermarks.absoluteTime) {
		trace.Logf(ctx, traceCatGet, "start time %s is outside the retention period; set to %s", st, s.watermarks.absoluteTime)
		st = s.watermarks.absoluteTime
	}
	st, et = normalize(st, et)
	if s.root == nil {
		trace.Log(ctx, traceCatGet, "empty")
		return
	}
	// divider := int(et.Sub(st) / durations[0])
	v := newVis()
	s.root.get(ctx, s, st, et, func(sn *streeNode, r *big.Rat) {
		// TODO: pass m / d from .get() ?
		v.add(sn, r, true)
		cb(sn.depth, sn.samples, sn.writes, sn.time, r)
	})
	v.print(filepath.Join(os.TempDir(), fmt.Sprintf("0-get-%s-%s.html", st.String(), et.String())))
}

func (s *Segment) DeleteNodesBefore(t *RetentionPolicy) (bool, error) {
	s.m.Lock()
	defer s.m.Unlock()
	if s.root == nil {
		return true, nil
	}
	ok, err := s.root.deleteNodesBefore(t.normalize())
	if err != nil {
		return false, err
	}
	if ok {
		s.root = nil
	}
	s.updateWatermarks(t)
	return ok, nil
}

func (s *Segment) updateWatermarks(t *RetentionPolicy) {
	if t.AbsoluteTime.After(s.watermarks.absoluteTime) {
		s.watermarks.absoluteTime = t.AbsoluteTime
	}
	for k, v := range t.Levels {
		if level, ok := s.watermarks.levels[k]; ok && v.Before(level) {
			continue
		}
		s.watermarks.levels[k] = v
	}
}

func (s *Segment) WalkNodesToDelete(t *RetentionPolicy, cb func(depth int, t time.Time) error) (bool, error) {
	s.m.RLock()
	defer s.m.RUnlock()
	if s.root == nil {
		return true, nil
	}
	return s.root.walkNodesToDelete(t.normalize(), cb)
}

func (s *Segment) SetMetadata(md metadata.Metadata) {
	s.m.Lock()
	s.spyName = md.SpyName
	s.sampleRate = md.SampleRate
	s.units = md.Units
	s.aggregationType = md.AggregationType
	s.m.Unlock()
}

func (s *Segment) GetMetadata() metadata.Metadata {
	s.m.Lock()
	md := metadata.Metadata{
		SpyName:         s.spyName,
		SampleRate:      s.sampleRate,
		Units:           s.units,
		AggregationType: s.aggregationType,
	}
	s.m.Unlock()
	return md
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

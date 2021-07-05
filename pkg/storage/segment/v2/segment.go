package v2

import (
	"errors"
	"math"
	"time"
)

var (
	ErrInvalidTimeRange  = errors.New("invalid time range")
	ErrTimeRangeExceeded = errors.New("time range exceeded")
)

const (
	mul = 10
	res = 10

	maxTime = 3999999999
)

// Segment represents segment tree.
type Segment struct {
	creationTime time.Time
}

type Node struct {
	Level int
	I     uint32
}

func NewSegment(t time.Time) Segment {
	return Segment{t}
}

// StartTime returns the time when the node was created.
func (s Segment) StartTime(n Node) time.Time {
	return s.creationTime.Add(time.Duration(n.Offset()) * time.Second)
}

// Duration reports node size in seconds.
func (n Node) Duration() uint32 { return uint32(math.Pow10(n.Level)) * res }

// Offset returns number of seconds from the segment creation time.
func (n Node) Offset() uint32 { return n.Duration() * n.I }

// NodeIndexes returns slice of node indexes for every affected level.
func (s Segment) NodeIndexes(t time.Time) []uint32 {
	ixs := make([]uint32, 1, mul) // empirical.
	ixs[0] = uint32(t.Sub(s.creationTime).Seconds()) / res
	x := ixs[0]
	for {
		x /= res
		ixs = append(ixs, x)
		if x == 0 {
			break
		}
	}
	return ixs
}

// Visitor for a node.
type Visitor func(Node) bool

// Walk calls visitor fn for every node within the time range from the top
// to bottom, left to right. When visitor returns false, Walk returns.
func (s Segment) Walk(from, to time.Time, fn Visitor) error {
	if from.After(to) {
		return ErrInvalidTimeRange
	}
	if to.Second() > maxTime {
		return ErrTimeRangeExceeded
	}
	if from.Before(s.creationTime) {
		from = s.creationTime
	}
	s.walk(from, to, fn)
	return nil
}

func (s Segment) walk(from, to time.Time, fn Visitor) {
	a, b := s.NodeIndexes(from), s.NodeIndexes(to)
	// a and b are slices containing boundary nodes, e.g.:
	//  Level   a     b
	//    3     0     3
	//    2     0    35
	//    1     1   356
	//    0    12  3560
	if len(b) > len(a) {
		// If b > a, add padding. a can not be greater than b.
		a2 := make([]uint32, len(b))
		copy(a2, a)
		a = a2
	}

	// Traverse through the levels in descending order
	// to ensure the most significant nodes go first.
	var leftHigh, rightLow uint32
	top := len(b) - 1
	i := iter{visit: fn}
	for level := top; level >= 0; level-- {
		left, right := a[level], b[level]
		if left == right {
			// Skip Level, if both left and right
			// boundaries point to the same node.
			top--
			continue
		}

		// Check if left-most and right-most nodes need
		// to be included, e.g. when the range fits all
		// nodes at the level.
		n := Node{level, left}
		if !s.isInRange(n, from, to) {
			left++
		}
		n.I = right
		if !s.isInRange(n, from, to) {
			right--
		}

		// Iterate over the ranges from left to right.
		i.level = level
		if level == top {
			i.rng(true, left, right, true)
			goto next
		}
		if !i.rng(true, left, leftHigh, false) {
			return
		}
		if !i.rng(false, rightLow, right, true) {
			return
		}

	next:
		leftHigh = left * mul
		rightLow = (right+1)*mul - 1
	}

	return
}

// isInRange reports whether node n pertains to the range [from, to).
func (s *Segment) isInRange(n Node, from, to time.Time) bool {
	begins := s.StartTime(n)
	if !(begins.After(from) || begins.Equal(from)) {
		return false
	}
	n.I++
	ends := s.StartTime(n)
	return ends.Before(to) || ends.Equal(to)
}

// iter is a helper for more convenient iterating over index ranges.
type iter struct {
	visit Visitor
	level int
}

// rng visits every node in the range (left, right): il and ir specify whether
// to include the edge value, e.g. when both set to true: [left, right].
func (r iter) rng(il bool, left, right uint32, ir bool) bool {
	if left == right {
		if il && ir {
			return r.visit(Node{Level: r.level, I: left})
		}
		return true
	}
	x := int(left)
	y := int(right)
	if il {
		x--
	}
	if !ir {
		y--
	}
	for i := x; i < y; i++ {
		x++
		if !r.visit(Node{Level: r.level, I: uint32(x)}) {
			return false
		}
	}
	return true
}

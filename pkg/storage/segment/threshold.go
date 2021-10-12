package segment

import (
	"time"
)

type Threshold struct {
	now      time.Time
	absolute time.Time
	levels   map[int]time.Time
}

func NewThreshold() *Threshold { return &Threshold{now: time.Now()} }

func (t Threshold) LowerBoundary() time.Time {
	if t.levels == nil {
		return t.absolute
	}
	return t.levels[0]
}

func (t *Threshold) SetAbsoluteMaxAge(maxAge time.Duration) *Threshold {
	t.absolute = t.timeBefore(maxAge)
	return t
}

func (t *Threshold) SetLevelMaxAge(level int, maxAge time.Duration) *Threshold {
	if t.levels == nil {
		t.levels = make(map[int]time.Time)
	}
	t.levels[level] = t.timeBefore(maxAge)
	return t
}

func (t Threshold) isBefore(sn *streeNode) bool {
	if sn.isBefore(t.absolute) {
		return true
	}
	return sn.isBefore(t.levelThreshold(sn.depth))
}

func (t Threshold) timeBefore(age time.Duration) time.Time {
	if age == 0 {
		return zeroTime
	}
	return t.now.Add(-1 * age)
}

func (t *Threshold) normalize() *Threshold {
	t.absolute = normalizeTime(t.absolute)
	for i := range t.levels {
		t.levels[i] = normalizeTime(t.levels[i])
	}
	return t
}

func (t Threshold) levelThreshold(depth int) time.Time {
	if t.levels == nil {
		return zeroTime
	}
	return t.levels[depth]
}

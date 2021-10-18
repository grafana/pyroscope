package segment

import (
	"time"
)

type RetentionPolicy struct {
	now      time.Time
	absolute time.Time
	levels   map[int]time.Time
	size     int
}

func NewRetentionPolicy() *RetentionPolicy {
	return &RetentionPolicy{now: time.Now()}
}

func (t RetentionPolicy) IsPeriodBased() bool {
	return t.levels != nil || t.absolute != zeroTime
}

func (t RetentionPolicy) IsSizeBased() bool { return t.size > 0 }

func (t RetentionPolicy) LowerTimeBoundary() time.Time {
	if t.levels == nil {
		return t.absolute
	}
	return t.levels[0]
}

func (t *RetentionPolicy) SetAbsoluteSize(s int) *RetentionPolicy {
	t.size = s
	return t
}

func (t *RetentionPolicy) SetAbsoluteMaxAge(maxAge time.Duration) *RetentionPolicy {
	t.absolute = t.timeBefore(maxAge)
	return t
}

func (t *RetentionPolicy) SetLevelMaxAge(level int, maxAge time.Duration) *RetentionPolicy {
	if t.levels == nil {
		t.levels = make(map[int]time.Time)
	}
	t.levels[level] = t.timeBefore(maxAge)
	return t
}

func (t RetentionPolicy) isBefore(sn *streeNode) bool {
	if sn.isBefore(t.absolute) {
		return true
	}
	return sn.isBefore(t.levelThreshold(sn.depth))
}

func (t RetentionPolicy) timeBefore(age time.Duration) time.Time {
	if age == 0 {
		return zeroTime
	}
	return t.now.Add(-1 * age)
}

func (t *RetentionPolicy) normalize() *RetentionPolicy {
	t.absolute = normalizeTime(t.absolute)
	for i := range t.levels {
		t.levels[i] = normalizeTime(t.levels[i])
	}
	return t
}

func (t RetentionPolicy) levelThreshold(depth int) time.Time {
	if t.levels == nil {
		return zeroTime
	}
	return t.levels[depth]
}

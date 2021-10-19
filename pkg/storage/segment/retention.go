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

func (r RetentionPolicy) LowerTimeBoundary() time.Time {
	if r.levels == nil {
		return r.absolute
	}
	return r.levels[0]
}

func (r *RetentionPolicy) SetAbsoluteSize(s int) *RetentionPolicy {
	r.size = s
	return r
}

func (r *RetentionPolicy) SetAbsoluteMaxAge(maxAge time.Duration) *RetentionPolicy {
	r.absolute = r.timeBefore(maxAge)
	return r
}

func (r *RetentionPolicy) SetLevelMaxAge(level int, maxAge time.Duration) *RetentionPolicy {
	if r.levels == nil {
		r.levels = make(map[int]time.Time)
	}
	r.levels[level] = r.timeBefore(maxAge)
	return r
}

func (r RetentionPolicy) isBefore(sn *streeNode) bool {
	if sn.isBefore(r.absolute) {
		return true
	}
	return sn.isBefore(r.levelRetentionPeriod(sn.depth))
}

func (r RetentionPolicy) timeBefore(age time.Duration) time.Time {
	if age == 0 {
		return zeroTime
	}
	return r.now.Add(-1 * age)
}

func (r *RetentionPolicy) normalize() *RetentionPolicy {
	r.absolute = normalizeTime(r.absolute)
	for i := range r.levels {
		r.levels[i] = normalizeTime(r.levels[i])
	}
	return r
}

func (r RetentionPolicy) levelRetentionPeriod(depth int) time.Time {
	if r.levels == nil {
		return zeroTime
	}
	return r.levels[depth]
}

package segment

import (
	"time"
)

type RetentionPolicy struct {
	now time.Time

	AbsoluteTime time.Time
	Levels       map[int]time.Time

	ExemplarsRetentionTime time.Time
}

func NewRetentionPolicy() *RetentionPolicy {
	return &RetentionPolicy{now: time.Now()}
}

func (r RetentionPolicy) LowerTimeBoundary() time.Time {
	if len(r.Levels) == 0 {
		return r.AbsoluteTime
	}
	return r.Levels[0]
}

func (r *RetentionPolicy) SetAbsolutePeriod(period time.Duration) *RetentionPolicy {
	r.AbsoluteTime = r.periodToTime(period)
	return r
}

func (r *RetentionPolicy) SetExemplarsRetentionPeriod(period time.Duration) *RetentionPolicy {
	r.ExemplarsRetentionTime = r.periodToTime(period)
	return r
}

func (r *RetentionPolicy) SetLevelPeriod(level int, period time.Duration) *RetentionPolicy {
	if r.Levels == nil {
		r.Levels = make(map[int]time.Time)
	}
	r.Levels[level] = r.periodToTime(period)
	return r
}

func (r *RetentionPolicy) SetLevels(levels ...time.Duration) *RetentionPolicy {
	if r.Levels == nil {
		r.Levels = make(map[int]time.Time)
	}
	for level, period := range levels {
		if period != 0 {
			r.Levels[level] = r.periodToTime(period)
		}
	}
	return r
}

func (r RetentionPolicy) isToBeDeleted(sn *streeNode) bool {
	return sn.isBefore(r.AbsoluteTime) || sn.isBefore(r.levelMaxTime(sn.depth))
}

func (r RetentionPolicy) periodToTime(age time.Duration) time.Time {
	if age == 0 {
		return time.Time{}
	}
	return r.now.Add(-1 * age)
}

func (r *RetentionPolicy) normalize() *RetentionPolicy {
	r.AbsoluteTime = normalizeTime(r.AbsoluteTime)
	for k, v := range r.Levels {
		r.Levels[k] = normalizeTime(v)
	}
	return r
}

func (r RetentionPolicy) levelMaxTime(depth int) time.Time {
	if r.Levels == nil {
		return time.Time{}
	}
	return r.Levels[depth]
}

package segment

import (
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

// TODO(kolesnikovae): refactor.

type RetentionPolicy struct {
	sizeLimit bytesize.ByteSize
	now       time.Time

	AbsoluteTime time.Time
	Levels       map[int]time.Time
}

func NewRetentionPolicy() *RetentionPolicy {
	return &RetentionPolicy{now: time.Now()}
}

func (r *RetentionPolicy) watermark() RetentionPolicy {
	w := RetentionPolicy{AbsoluteTime: r.AbsoluteTime}
	if len(r.Levels) == 0 {
		return w
	}
	w.Levels = make(map[int]time.Time, len(r.Levels))
	for k, v := range r.Levels {
		w.Levels[k] = v
	}
	return w
}

func (r RetentionPolicy) LowerTimeBoundary() time.Time {
	if r.Levels == nil {
		return r.AbsoluteTime
	}
	return r.Levels[0]
}

func (r *RetentionPolicy) SetAbsoluteTime(t time.Time) *RetentionPolicy {
	r.AbsoluteTime = t
	return r
}

func (r *RetentionPolicy) SetAbsoluteMaxAge(maxAge time.Duration) *RetentionPolicy {
	r.AbsoluteTime = r.timeBefore(maxAge)
	return r
}

func (r *RetentionPolicy) SetLevelMaxTime(level int, t time.Time) *RetentionPolicy {
	if r.Levels == nil {
		r.Levels = make(map[int]time.Time)
	}
	r.Levels[level] = t
	return r
}

func (r *RetentionPolicy) SetLevelMaxAge(level int, maxAge time.Duration) *RetentionPolicy {
	if r.Levels == nil {
		r.Levels = make(map[int]time.Time)
	}
	r.Levels[level] = r.timeBefore(maxAge)
	return r
}

func (r RetentionPolicy) isBefore(sn *streeNode) bool {
	if sn.isBefore(r.AbsoluteTime) {
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
	r.AbsoluteTime = normalizeTime(r.AbsoluteTime)
	for i := range r.Levels {
		r.Levels[i] = normalizeTime(r.Levels[i])
	}
	return r
}

func (r RetentionPolicy) levelRetentionPeriod(depth int) time.Time {
	if r.Levels == nil {
		return zeroTime
	}
	return r.Levels[depth]
}

func (r *RetentionPolicy) SizeLimit() bytesize.ByteSize {
	return r.sizeLimit
}

func (r *RetentionPolicy) SetSizeLimit(x bytesize.ByteSize) *RetentionPolicy {
	r.sizeLimit = x
	return r
}

// CapacityToReclaim reports disk space capacity to reclaim,
// calculated as follows: used - limit + limit*ratio.
//
// The call never returns a negative value.
//
// Example: limit 10GB  used 9GB    t 0.05 = 0
//          limit 10GB  used 9.5GB  t 0.05 = 0
//          limit 10GB  used 9.6GB  t 0.05 = 0.1GB
//          limit 10GB  used 10GB   t 0.05 = 0.5GB
//          limit 10GB  used 20GB   t 0.05 = 10.5GB
func (r RetentionPolicy) CapacityToReclaim(used bytesize.ByteSize, ratio float64) bytesize.ByteSize {
	if r.sizeLimit <= 0 || used <= 0 {
		return 0
	}
	if v := used + bytesize.ByteSize(float64(r.sizeLimit)*ratio) - r.sizeLimit; v > 0 {
		return v
	}
	return 0
}

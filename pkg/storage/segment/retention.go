package segment

import (
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
)

type RetentionPolicy struct {
	sizeLimit bytesize.ByteSize

	now            time.Time
	absolutePeriod time.Time
	levels         map[int]time.Time
}

func NewRetentionPolicy() *RetentionPolicy {
	return &RetentionPolicy{now: time.Now()}
}

func (r RetentionPolicy) LowerTimeBoundary() time.Time {
	if r.levels == nil {
		return r.absolutePeriod
	}
	return r.levels[0]
}

func (r *RetentionPolicy) SetAbsoluteMaxAge(maxAge time.Duration) *RetentionPolicy {
	r.absolutePeriod = r.timeBefore(maxAge)
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
	if sn.isBefore(r.absolutePeriod) {
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
	r.absolutePeriod = normalizeTime(r.absolutePeriod)
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

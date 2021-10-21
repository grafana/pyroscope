package segment

import (
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/util/bytesize"
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

func (r *RetentionPolicy) SizeLimit() bytesize.ByteSize {
	return bytesize.ByteSize(r.size)
}

func (r *RetentionPolicy) SetAbsoluteSize(s int) *RetentionPolicy {
	r.size = s
	return r
}

// CapacityToReclaim reports disk space capacity in bytes needs to be reclaimed.
// The function reserves share t of the available capacity, reported value
// includes this size.
//
// Example: used 9GB    available 10GB  t 0.05 – the function returns 0.
//          used 9.5GB  available 10GB  t 0.05 – the function returns 0.
//          used 9.6GB  available 10GB  t 0.05 – the function returns 0.1GB.
//          used 10GB   available 10GB  t 0.05 – the function returns 0.5GB.
func (r RetentionPolicy) CapacityToReclaim(used bytesize.ByteSize, t float64) bytesize.ByteSize {
	if r.size <= 0 || used <= 0 {
		return 0
	}
	if v := used + bytesize.ByteSize(float64(r.size)*t) - bytesize.ByteSize(r.size); v > 0 {
		return v
	}
	return 0
}

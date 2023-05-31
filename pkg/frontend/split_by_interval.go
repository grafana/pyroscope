package frontend

import (
	"time"

	"github.com/grafana/phlare/pkg/util/math"
)

// TimeIntervalIterator splits a time range into non-overlapping sub-ranges,
// where the boundary adjoining on the left is not included, e.g:
//
//	[t1, t2), [t3, t4), ..., [tn-1, tn].
//
// By default, a sub-range start time is a multiple of the interval.
// See WithAlignment option, if a custom alignment is needed.
type TimeIntervalIterator struct {
	startTime int64
	endTime   int64
	interval  int64
	alignment int64
}

type TimeInterval struct{ Start, End time.Time }

type TimeIntervalIteratorOption func(*TimeIntervalIterator)

// WithAlignment causes a sub-range start time to be a multiple of the
// alignment. This makes it possible for a sub-range to be shorter
// than the interval specified, but not more than by the alignment.
//
// The interval can't be less than the alignment.
func WithAlignment(a time.Duration) TimeIntervalIteratorOption {
	return func(i *TimeIntervalIterator) {
		i.alignment = a.Nanoseconds()
	}
}

// NewTimeIntervalIterator returns a new interval iterator.
// If the interval is zero, the entire time span is taken as a single interval.
func NewTimeIntervalIterator(startTime, endTime time.Time, interval time.Duration,
	options ...TimeIntervalIteratorOption) *TimeIntervalIterator {
	i := &TimeIntervalIterator{
		startTime: startTime.UnixNano(),
		endTime:   endTime.UnixNano(),
		interval:  interval.Nanoseconds(),
	}
	if interval == 0 {
		i.interval = 2 * endTime.Sub(startTime).Nanoseconds()
	}
	for _, option := range options {
		option(i)
	}
	i.interval = math.Max(i.interval, i.alignment)
	return i
}

func (i *TimeIntervalIterator) Next() bool { return i.startTime < i.endTime }

func (i *TimeIntervalIterator) At() TimeInterval {
	t := TimeInterval{Start: time.Unix(0, i.startTime)}
	i.startTime += i.interval
	if i.alignment > 0 {
		// Sub-ranges start at a multiple of 'alignment'.
		i.startTime -= i.interval % i.alignment
	} else {
		// Sub-ranges start at a multiple of 'interval'.
		i.startTime -= i.startTime % i.interval
	}
	if i.endTime > i.startTime {
		// -1 to ensure the adjacent ranges don't overlap.
		// Could be an option.
		t.End = time.Unix(0, i.startTime-1)
	} else {
		t.End = time.Unix(0, i.endTime)
	}
	return t
}

func (*TimeIntervalIterator) Err() error { return nil }

func (*TimeIntervalIterator) Close() error { return nil }

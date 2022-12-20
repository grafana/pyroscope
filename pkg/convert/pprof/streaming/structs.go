package streaming

import "github.com/pyroscope-io/pyroscope/pkg/storage/segment"

const (
	profSampleType        = 1
	profSample            = 2
	profMapping           = 3
	profLocation          = 4
	profFunction          = 5
	profStringTable       = 6
	profDropFrames        = 7
	profKeepFrames        = 8
	profTimeNanos         = 9
	profDurationNanos     = 10
	profPeriodType        = 11
	profPeriod            = 12
	profComment           = 13
	profDefaultSampleType = 14

	stType = 1
	stUnit = 2

	locId   = 1
	locLine = 4

	lineFunctionId = 1

	funcId   = 1
	funcName = 2

	sampleLocationId = 1
	sampleValue      = 2
	sampleLabel      = 3

	labelKey = 1
	labelStr = 2
)

var (
	profileIdLabel = []byte(segment.ProfileIDLabelName)
)


type valueType struct {
	Type int
	unit int
}
type function struct {
	id uint64
	name int
}
type location struct {
	id uint64

	fn1 uint64
	//fn2     int64
	extraFn []uint64
}
type label struct {
	k, v int
}

func (l *location) addFunction(fn uint64) {
	if l.fn1 == 0 {
		l.fn1 = fn
		return
	}
	l.extraFn = append(l.extraFn, fn) //todo compare 1 field + slice,2 fields + slice, slice-only
}

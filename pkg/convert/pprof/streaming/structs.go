package streaming

import (
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
)

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

	locID   = 1
	locLine = 4

	lineFunctionID = 1

	funcID   = 1
	funcName = 2

	sampleLocationID = 1
	sampleValue      = 2
	sampleLabel      = 3

	labelKey = 1
	labelStr = 2
)

var (
	profileIDLabel = []byte(segment.ProfileIDLabelName)
)

type valueType struct {
	Type int64
	unit int64
}
type function struct {
	id   uint64
	name int64
}

const noFunction = 0xffffffffffffffff

type location struct {
	id uint64

	fn1 uint64
	fn2 uint64

	extraFn []uint64 // todo maybe try make it a pointer to a slice?
}

type line struct {
	functionID uint64
}

type label struct {
	k, v int64
}

type sample struct {
	tmpValues   []int64
	tmpLabels   []label
	tmpStack    [][]byte
	tmpStackLoc []uint64
}

func (s *sample) preAllocate(nSampleTypes int) {
	// 64 is max pc for golang + speculative number of inlines
	s.tmpStack = make([][]byte, 0, 64+8)
	s.tmpStackLoc = make([]uint64, 0, 64+8)
	s.tmpValues = make([]int64, 0, nSampleTypes)
}

func (s *sample) resetSample() {
	s.tmpValues = s.tmpValues[:0]
	s.tmpLabels = s.tmpLabels[:0]
	s.tmpStack = s.tmpStack[:0]
	s.tmpStackLoc = s.tmpStackLoc[:0]
}

func (l *location) addFunction(fn uint64) {
	if l.fn1 == noFunction {
		l.fn1 = fn
		return
	}
	if l.fn2 == noFunction {
		l.fn2 = fn
		return
	}
	l.extraFn = append(l.extraFn, fn) //todo compare 1 field + slice,2 fields + slice, slice-only
}

package streaming

import (
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/richardartoul/molecule"
	"github.com/richardartoul/molecule/src/codec"
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
	Type int
	unit int
}
type function struct {
	id   uint64
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

func parseLocation(buffer, tmpBuf *codec.Buffer) (location, error) {
	var l = location{}
	err := molecule.MessageEach(buffer, func(field int32, value molecule.Value) (bool, error) {
		switch field {
		case locID:
			l.id = value.Number
		case locLine:
			tmpBuf.Reset(value.Bytes)
			err := molecule.MessageEach(tmpBuf, func(field int32, value molecule.Value) (bool, error) {
				if field == lineFunctionID {
					l.addFunction(value.Number)
				}
				return true, nil
			})
			if err != nil {
				return false, err
			}
		}
		return true, nil
	})
	return l, err
}

func parseFunction(buffer *codec.Buffer) (function, error) {
	//todo try to pass a pointer to a struct to write?
	var l = function{}
	err := molecule.MessageEach(buffer, func(field int32, value molecule.Value) (bool, error) {
		switch field {
		case funcID:
			l.id = value.Number
		case funcName:
			l.name = int(value.Number)
		}
		return true, nil
	})
	return l, err
}

func parseLabel(buffer *codec.Buffer) (label, error) {
	var l = label{}
	err := molecule.MessageEach(buffer, func(field int32, value molecule.Value) (bool, error) {
		switch field {
		case labelKey:
			l.k = int(value.Number)
		case labelStr:
			l.v = int(value.Number)
		}
		return true, nil
	})
	return l, err
}

func parseValueType(buffer *codec.Buffer) (valueType, error) {
	var unit int
	var sType int
	err := molecule.MessageEach(buffer, func(field int32, value molecule.Value) (bool, error) {
		switch field {
		case stUnit:
			unit = int(value.Number)
		case stType:
			sType = int(value.Number)
		}
		return true, nil
	})
	return valueType{unit: unit, Type: sType}, err
}

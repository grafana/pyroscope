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
	Type int64
	unit int64
}
type function struct {
	id   uint64
	name int64
}
type location struct {
	id uint64

	fn1 uint64
	//fn2     int64
	extraFn []uint64
}
type label struct {
	k, v int64
}

func (l *location) addFunction(fn uint64) {
	if l.fn1 == 0 {
		l.fn1 = fn
		return
	}
	l.extraFn = append(l.extraFn, fn) //todo compare 1 field + slice,2 fields + slice, slice-only
}

func parseLocation(buffer, tmpBuf *codec.Buffer, l *location) error {
	l.id = 0
	l.fn1 = 0
	l.extraFn = nil
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
	return err
}

func parseFunction(buffer *codec.Buffer, f *function) error {
	//todo try to pass a pointer to a struct to write?
	f.id = 0
	f.name = 0
	err := molecule.MessageEach(buffer, func(field int32, value molecule.Value) (bool, error) {
		switch field {
		case funcID:
			f.id = value.Number
		case funcName:
			f.name = int64(value.Number)
		}
		return true, nil
	})
	return err
}

func parseLabel(buffer *codec.Buffer) (label, error) {
	var l = label{}
	err := molecule.MessageEach(buffer, func(field int32, value molecule.Value) (bool, error) {
		switch field {
		case labelKey:
			l.k = int64(value.Number)
		case labelStr:
			l.v = int64(value.Number)
		}
		return true, nil
	})
	return l, err
}

func parseValueType(buffer *codec.Buffer, vt *valueType) error {
	vt.unit = 0
	vt.Type = 0
	return molecule.MessageEach(buffer, func(field int32, value molecule.Value) (bool, error) {
		switch field {
		case stUnit:
			vt.unit = int64(value.Number)
		case stType:
			vt.Type = int64(value.Number)
		}
		return true, nil
	})
}

type profileCallbacks struct {
	string     func(s []byte)
	periodType func(pt *valueType)
	sampleType func(pt *valueType)
	function   func(f *function)
	location   func(f *location)
}

type profileParser struct {
	mainBuf    *codec.Buffer
	tmpBuf1    *codec.Buffer
	tmpBuf2    *codec.Buffer
	period     int
	nFunctions int
	nLocations int

	tmpValueType valueType
	function     function
	location     location
}

func (p *profileParser) parse(profile []byte, callbacks profileCallbacks) error {
	p.period = 0
	p.nFunctions = 0
	p.nLocations = 0
	if p.mainBuf == nil {
		p.mainBuf = codec.NewBuffer(profile)
	} else {
		p.mainBuf.Reset(profile)
	}
	vt := &p.tmpValueType
	f := &p.function
	l := &p.location
	nFunctions := 0
	nLocations := 0
	err := molecule.MessageEach(p.mainBuf, func(field int32, value molecule.Value) (bool, error) {
		switch field {
		case profPeriod:
			p.period = int(value.Number)
		case profPeriodType:
			if callbacks.periodType != nil {
				if p.tmpBuf1 == nil {
					p.tmpBuf1 = codec.NewBuffer(value.Bytes)
				} else {
					p.tmpBuf1.Reset(value.Bytes)
				}
				err := parseValueType(p.tmpBuf1, vt)
				if err != nil {
					return false, nil
				}
				callbacks.periodType(vt)
			}
		case profSampleType:
			if callbacks.sampleType != nil {
				if p.tmpBuf1 == nil {
					p.tmpBuf1 = codec.NewBuffer(value.Bytes)
				} else {
					p.tmpBuf1.Reset(value.Bytes)
				}
				err := parseValueType(p.tmpBuf1, vt)
				if err != nil {
					return false, nil
				}
				callbacks.sampleType(vt)
			}
		case profLocation:
			nLocations++
			if callbacks.location != nil {
				if p.tmpBuf1 == nil {
					p.tmpBuf1 = codec.NewBuffer(value.Bytes)
				} else {
					p.tmpBuf1.Reset(value.Bytes)
				}
				if p.tmpBuf2 == nil {
					p.tmpBuf2 = codec.NewBuffer(nil)
				}
				err := parseLocation(p.tmpBuf1, p.tmpBuf2, l)
				if err != nil {
					return false, err
				}
				callbacks.location(l)
			}
		case profFunction:
			nFunctions++
			if callbacks.function != nil {
				if p.tmpBuf1 == nil {
					p.tmpBuf1 = codec.NewBuffer(value.Bytes)
				} else {
					p.tmpBuf1.Reset(value.Bytes)
				}
				err := parseFunction(p.tmpBuf1, f)
				if err != nil {
					return false, err
				}
				callbacks.function(f)
			}
		case profStringTable:
			if callbacks.string != nil {
				callbacks.string(value.Bytes)
			}
		}
		return true, nil
	})
	p.nFunctions = nFunctions
	p.nLocations = nLocations
	return err

}

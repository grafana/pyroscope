package speedscope

import (
	"fmt"

	"github.com/grafana/pyroscope/pkg/og/storage/metadata"
	"github.com/grafana/pyroscope/pkg/og/storage/segment"
)

type unit string

const (
	unitNone         = unit("none")
	unitNanoseconds  = unit("nanoseconds")
	unitMicroseconds = unit("microseconds")
	unitMilliseconds = unit("milliseconds")
	unitSeconds      = unit("seconds")
	unitBytes        = unit("bytes")
)

// This number defines how much precision we want to keep when converting
// from doubles to integers
var timePrecisionMultiplier = 100

func (u unit) defaultSampleRate() uint32 {
	switch u {
	case unitNanoseconds:
		return uint32(timePrecisionMultiplier) * 1000 * 1000 * 1000
	case unitMicroseconds:
		return uint32(timePrecisionMultiplier) * 1000 * 1000
	case unitMilliseconds:
		return uint32(timePrecisionMultiplier) * 1000
	case unitSeconds:
		return uint32(timePrecisionMultiplier)
	case unitNone:
		// 100 is a common default value for sample rate
		return uint32(timePrecisionMultiplier) * 100
	case unitBytes:
		return 0
	default:
		panic("unknown unit " + u)
	}
}

func (u unit) precisionMultiplier() uint64 {
	switch u {
	case unitNanoseconds:
		return uint64(timePrecisionMultiplier)
	case unitMicroseconds:
		return uint64(timePrecisionMultiplier)
	case unitMilliseconds:
		return uint64(timePrecisionMultiplier)
	case unitSeconds:
		return uint64(timePrecisionMultiplier)
	case unitNone:
		return uint64(timePrecisionMultiplier)
	case unitBytes:
		return 1
	default:
		panic("unknown unit " + u)
	}
}

func (u unit) chooseMetadataUnit() metadata.Units {
	switch u {
	case unitBytes:
		return metadata.BytesUnits
	default:
		return metadata.SamplesUnits
	}
}

func (u unit) chooseKey(orig *segment.Key) *segment.Key {
	// This means we'll have duplicate keys if multiple profiles have the same units. Probably ok.
	name := fmt.Sprintf("%s.%s", orig.AppName(), u)
	result := orig.Clone()
	result.Add("__name__", name)
	return result
}

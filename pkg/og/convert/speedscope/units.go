package speedscope

import (
	"fmt"

	"github.com/grafana/pyroscope/api/model/labelset"
	"github.com/grafana/pyroscope/v2/pkg/og/storage/metadata"
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

func (u unit) defaultSampleRate() (uint32, error) {
	switch u {
	case unitNanoseconds:
		return uint32(timePrecisionMultiplier) * 1000 * 1000 * 1000, nil
	case unitMicroseconds:
		return uint32(timePrecisionMultiplier) * 1000 * 1000, nil
	case unitMilliseconds:
		return uint32(timePrecisionMultiplier) * 1000, nil
	case unitSeconds:
		return uint32(timePrecisionMultiplier), nil
	case unitNone:
		// 100 is a common default value for sample rate
		return uint32(timePrecisionMultiplier) * 100, nil
	case unitBytes:
		return 0, nil
	default:
		return 0, fmt.Errorf("unknown unit: %s", u)
	}
}

func (u unit) precisionMultiplier() (uint64, error) {
	switch u {
	case unitNanoseconds, unitMicroseconds, unitMilliseconds, unitSeconds, unitNone:
		return uint64(timePrecisionMultiplier), nil
	case unitBytes:
		return 1, nil
	default:
		return 0, fmt.Errorf("unknown unit: %s", u)
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

func (u unit) chooseKey(orig *labelset.LabelSet) *labelset.LabelSet {
	// This means we'll have duplicate keys if multiple profiles have the same units. Probably ok.
	name := fmt.Sprintf("%s.%s", orig.ServiceName(), u)
	result := orig.Clone()
	result.Add("__name__", name)
	return result
}

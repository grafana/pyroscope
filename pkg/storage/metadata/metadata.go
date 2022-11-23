package metadata

type Units string

func (u Units) String() string {
	return string(u)
}

const (
	SamplesUnits         Units = "samples"
	ObjectsUnits         Units = "objects"
	GoroutinesUnits      Units = "goroutines"
	BytesUnits           Units = "bytes"
	LockNanosecondsUnits Units = "lock_nanoseconds"
	LockSamplesUnits     Units = "lock_samples"
)

type AggregationType string

const (
	AverageAggregationType AggregationType = "average"
	SumAggregationType     AggregationType = "sum"
)

func (a AggregationType) String() string {
	return string(a)
}

type Metadata struct {
	SpyName         string
	SampleRate      uint32
	Units           Units
	AggregationType AggregationType
}

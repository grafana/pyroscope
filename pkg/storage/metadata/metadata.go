package metadata

type Units string

func (u Units) String() string {
	return string(u)
}

const (
	SamplesUnits         Units = "samples"
	ObjectsUnits               = "objects"
	GoroutinesUnits            = "goroutines"
	BytesUnits                 = "bytes"
	LockNanosecondsUnits       = "lock_nanoseconds"
	LockSamplesUnits           = "lock_samples"
)

type AggregationType string

const (
	AverageAggregationType AggregationType = "average"
	SumAggregationType     AggregationType = "sum"
)

func (a AggregationType) String() string {
	return string(a)
}

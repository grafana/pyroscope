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

var validUnits = map[string]bool{
	string(SamplesUnits):         true,
	string(ObjectsUnits):         true,
	string(GoroutinesUnits):      true,
	string(BytesUnits):           true,
	string(LockNanosecondsUnits): true,
	string(LockSamplesUnits):     true,
}

var validAggregationTypes = map[string]bool{
	string(AverageAggregationType): true,
	string(SumAggregationType):     true,
}

// IsValidUnit returns true if the given unit string is a valid, recognized unit.
func IsValidUnit(unit string) bool {
	return validUnits[unit]
}

// IsValidAggregationType returns true if the given aggregation type string is valid.
func IsValidAggregationType(aggType string) bool {
	return validAggregationTypes[aggType]
}

type Metadata struct {
	SpyName         string
	SampleRate      uint32
	Units           Units
	AggregationType AggregationType
}

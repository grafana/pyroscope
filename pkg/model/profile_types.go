package model

import (
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

// ProfileTypeInfo contains aggregation and semantic information for profile types
type ProfileTypeInfo struct {
	Name            string
	Description     string
	Group           string
	Unit            string
	IsCumulative    bool
	AggregationType typesv1.TimeSeriesAggregationType
}

// ProfilesTypeRegistry contains all known profile types
var ProfilesTypeRegistry = map[string]ProfileTypeInfo{
	"samples": {
		Name:            "samples",
		Description:     "Number of sampling events collected",
		Group:           "cpu",
		Unit:            "short",
		IsCumulative:    false,
		AggregationType: typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_SUM,
	},
	"cpu": {
		Name:            "cpu",
		Description:     "CPU time consumed",
		Group:           "cpu",
		Unit:            "ns",
		IsCumulative:    true,
		AggregationType: typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_RATE,
	},
	"inuse_objects": {
		Name:            "inuse_objects",
		Description:     "Number of objects currently in use",
		Group:           "memory",
		Unit:            "short",
		IsCumulative:    false,
		AggregationType: typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_AVERAGE,
	},
	"inuse_space": {
		Name:            "inuse_space",
		Description:     "Size of memory currently in use",
		Group:           "memory",
		Unit:            "bytes",
		IsCumulative:    false,
		AggregationType: typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_AVERAGE,
	},
	"alloc_objects": {
		Name:            "alloc_objects",
		Description:     "Number of objects allocated",
		Group:           "memory",
		Unit:            "short",
		IsCumulative:    true,
		AggregationType: typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_RATE,
	},
	"alloc_space": {
		Name:            "alloc_space",
		Description:     "Size of memory allocated in the heap",
		Group:           "memory",
		Unit:            "bytes",
		IsCumulative:    true,
		AggregationType: typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_RATE,
	},
	"alloc_samples": {
		Name:            "alloc_samples",
		Description:     "Number of memory allocation samples during CPU time",
		Group:           "memory",
		Unit:            "short",
		IsCumulative:    true,
		AggregationType: typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_RATE,
	},
	"alloc_size": {
		Name:            "alloc_size",
		Description:     "Size of memory allocated during CPU time",
		Group:           "memory",
		Unit:            "bytes",
		IsCumulative:    true,
		AggregationType: typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_RATE,
	},
	"alloc_in_new_tlab_bytes": {
		Name:            "alloc_in_new_tlab_bytes",
		Description:     "Size of memory allocated inside Thread-Local Allocation Buffers",
		Group:           "memory",
		Unit:            "bytes",
		IsCumulative:    true,
		AggregationType: typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_RATE,
	},
	"alloc_in_new_tlab_objects": {
		Name:            "alloc_in_new_tlab_objects",
		Description:     "Number of objects allocated inside Thread-Local Allocation Buffers",
		Group:           "memory",
		Unit:            "short",
		IsCumulative:    true,
		AggregationType: typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_RATE,
	},
	"goroutine": {
		Name:            "goroutine",
		Description:     "Number of goroutines",
		Group:           "goroutine",
		Unit:            "short",
		IsCumulative:    false,
		AggregationType: typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_AVERAGE,
	},
	"contentions": {
		Name:            "contentions",
		Description:     "Number of lock contentions observed",
		Group:           "locks",
		Unit:            "short",
		IsCumulative:    false,
		AggregationType: typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_SUM,
	},
	"lock_count": {
		Name:            "lock_count",
		Description:     "Number of lock acquisitions attempted",
		Group:           "locks",
		Unit:            "short",
		IsCumulative:    false,
		AggregationType: typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_SUM,
	},
	"delay": {
		Name:            "delay",
		Description:     "Time spent in blocking delays",
		Group:           "locks",
		Unit:            "ns",
		IsCumulative:    true,
		AggregationType: typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_RATE,
	},
	"lock_time": {
		Name:            "lock_time",
		Description:     "Cumulative time spent acquiring locks",
		Group:           "locks",
		Unit:            "ns",
		IsCumulative:    true,
		AggregationType: typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_RATE,
	},
	"exceptions": {
		Name:            "exceptions",
		Description:     "Number of exceptions within the sampled CPU time",
		Group:           "exceptions",
		Unit:            "short",
		IsCumulative:    false,
		AggregationType: typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_SUM,
	},
}

// GetProfileTypeInfo returns aggregation information for a profile type
func GetProfileTypeInfo(profileType string) ProfileTypeInfo {
	if info, exists := ProfilesTypeRegistry[profileType]; exists {
		return info
	}

	return ProfileTypeInfo{
		Name:            profileType,
		Description:     "Unknown profile type",
		Group:           "unknown",
		Unit:            "unknown",
		IsCumulative:    true,
		AggregationType: typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_RATE,
	}
}

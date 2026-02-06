// Package timeseriescompact provides types and utilities for working with
// compact time series data using attribute table references instead of
// string labels. This format is optimized for efficient merging and
// aggregation across distributed query backends.
//
// Currently only used for exemplar retrieval. Over time, this package will
// replace pkg/model/timeseries for all time series queries.
package timeseriescompact

import (
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

// CompactValue represents a single data point during iteration.
type CompactValue struct {
	Ts             int64
	SeriesKey      string
	SeriesRefs     []int64
	Value          float64
	AnnotationRefs []int64
	Exemplars      []*queryv1.Exemplar
}

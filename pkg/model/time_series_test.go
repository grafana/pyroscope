package model

import (
	"testing"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/iter"
	"github.com/grafana/pyroscope/pkg/testhelper"

	"github.com/stretchr/testify/assert"
)

func Test_RangeSeriesSum(t *testing.T) {
	seriesA := NewLabelsBuilder(nil).Set("foo", "bar").Set("__type__", "contentions").Labels()
	seriesB := NewLabelsBuilder(nil).Set("foo", "buzz").Set("__type__", "contentions").Labels()
	for _, tc := range []struct {
		name string
		in   []TimeSeriesValue
		out  []*typesv1.Series
	}{
		{
			name: "single series",
			in: []TimeSeriesValue{
				{Ts: 1, Value: 1, Lbs: seriesA, LabelsHash: seriesA.Hash()},
				{Ts: 1, Value: 1, Lbs: seriesA, LabelsHash: seriesA.Hash()},
				{Ts: 2, Value: 2, Lbs: seriesA, LabelsHash: seriesA.Hash()},
				{Ts: 3, Value: 3, Lbs: seriesA, LabelsHash: seriesA.Hash()},
				{Ts: 4, Value: 4, Lbs: seriesA, LabelsHash: seriesA.Hash()},
				{Ts: 5, Value: 5, Lbs: seriesA, LabelsHash: seriesA.Hash(), Annotations: []*typesv1.ProfileAnnotation{{Key: "foo", Value: "bar"}}},
			},
			out: []*typesv1.Series{
				{
					Labels: seriesA,
					Points: []*typesv1.Point{
						{Timestamp: 1, Value: 2, Annotations: []*typesv1.ProfileAnnotation{}},
						{Timestamp: 2, Value: 2, Annotations: []*typesv1.ProfileAnnotation{}},
						{Timestamp: 3, Value: 3, Annotations: []*typesv1.ProfileAnnotation{}},
						{Timestamp: 4, Value: 4, Annotations: []*typesv1.ProfileAnnotation{}},
						{Timestamp: 5, Value: 5, Annotations: []*typesv1.ProfileAnnotation{{Key: "foo", Value: "bar"}}},
					},
				},
			},
		},
		{
			name: "multiple series",
			in: []TimeSeriesValue{
				{Ts: 1, Value: 1, Lbs: seriesA, LabelsHash: seriesA.Hash()},
				{Ts: 1, Value: 1, Lbs: seriesB, LabelsHash: seriesB.Hash()},
				{Ts: 2, Value: 1, Lbs: seriesA, LabelsHash: seriesA.Hash()},
				{Ts: 3, Value: 1, Lbs: seriesB, LabelsHash: seriesB.Hash()},
				{Ts: 3, Value: 1, Lbs: seriesB, LabelsHash: seriesB.Hash()},
				{Ts: 4, Value: 4, Lbs: seriesB, LabelsHash: seriesB.Hash(), Annotations: []*typesv1.ProfileAnnotation{{Key: "foo", Value: "bar"}}},
				{Ts: 4, Value: 4, Lbs: seriesB, LabelsHash: seriesB.Hash(), Annotations: []*typesv1.ProfileAnnotation{{Key: "foo", Value: "buzz"}}},
				{Ts: 4, Value: 4, Lbs: seriesA, LabelsHash: seriesA.Hash()},
				{Ts: 5, Value: 5, Lbs: seriesA, LabelsHash: seriesA.Hash()},
			},
			out: []*typesv1.Series{
				{
					Labels: seriesA,
					Points: []*typesv1.Point{
						{Timestamp: 1, Value: 1, Annotations: []*typesv1.ProfileAnnotation{}},
						{Timestamp: 2, Value: 1, Annotations: []*typesv1.ProfileAnnotation{}},
						{Timestamp: 4, Value: 4, Annotations: []*typesv1.ProfileAnnotation{}},
						{Timestamp: 5, Value: 5, Annotations: []*typesv1.ProfileAnnotation{}},
					},
				},
				{
					Labels: seriesB,
					Points: []*typesv1.Point{
						{Timestamp: 1, Value: 1, Annotations: []*typesv1.ProfileAnnotation{}},
						{Timestamp: 3, Value: 2, Annotations: []*typesv1.ProfileAnnotation{}},
						{Timestamp: 4, Value: 8, Annotations: []*typesv1.ProfileAnnotation{
							{Key: "foo", Value: "bar"},
							{Key: "foo", Value: "buzz"}}},
					},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := iter.NewSliceIterator(tc.in)
			out := RangeSeries(in, 1, 5, 1, nil)
			testhelper.EqualProto(t, tc.out, out)
		})
	}
}

func Test_RangeSeriesAvg(t *testing.T) {
	seriesA := NewLabelsBuilder(nil).Set("foo", "bar").Set("__type__", "inuse_objects").Labels()
	seriesB := NewLabelsBuilder(nil).Set("foo", "buzz").Set("__type__", "inuse_objects").Labels()
	for _, tc := range []struct {
		name string
		in   []TimeSeriesValue
		out  []*typesv1.Series
	}{
		{
			name: "single series",
			in: []TimeSeriesValue{
				{Ts: 1, Value: 1, Lbs: seriesA, LabelsHash: seriesA.Hash()},
				{Ts: 1, Value: 2, Lbs: seriesA, LabelsHash: seriesA.Hash()},
				{Ts: 2, Value: 2, Lbs: seriesA, LabelsHash: seriesA.Hash()},
				{Ts: 2, Value: 3, Lbs: seriesA, LabelsHash: seriesA.Hash()},
				{Ts: 3, Value: 4, Lbs: seriesA, LabelsHash: seriesA.Hash()},
				{Ts: 4, Value: 5, Lbs: seriesA, LabelsHash: seriesA.Hash(), Annotations: []*typesv1.ProfileAnnotation{{Key: "foo", Value: "bar"}}},
			},
			out: []*typesv1.Series{
				{
					Labels: seriesA,
					Points: []*typesv1.Point{
						{Timestamp: 1, Value: 1.5, Annotations: []*typesv1.ProfileAnnotation{}}, // avg of 1 and 2
						{Timestamp: 2, Value: 2.5, Annotations: []*typesv1.ProfileAnnotation{}}, // avg of 2 and 3
						{Timestamp: 3, Value: 4, Annotations: []*typesv1.ProfileAnnotation{}},
						{Timestamp: 4, Value: 5, Annotations: []*typesv1.ProfileAnnotation{{Key: "foo", Value: "bar"}}},
					},
				},
			},
		},
		{
			name: "multiple series",
			in: []TimeSeriesValue{
				{Ts: 1, Value: 1, Lbs: seriesA, LabelsHash: seriesA.Hash()},
				{Ts: 1, Value: 1, Lbs: seriesB, LabelsHash: seriesB.Hash()},
				{Ts: 2, Value: 1, Lbs: seriesA, LabelsHash: seriesA.Hash()},
				{Ts: 2, Value: 2, Lbs: seriesA, LabelsHash: seriesA.Hash()},
				{Ts: 3, Value: 1, Lbs: seriesB, LabelsHash: seriesB.Hash()},
				{Ts: 3, Value: 2, Lbs: seriesB, LabelsHash: seriesB.Hash()},
				{Ts: 4, Value: 4, Lbs: seriesB, LabelsHash: seriesB.Hash()},
				{Ts: 4, Value: 6, Lbs: seriesB, LabelsHash: seriesB.Hash()},
				{Ts: 4, Value: 4, Lbs: seriesA, LabelsHash: seriesA.Hash()},
				{Ts: 5, Value: 5, Lbs: seriesA, LabelsHash: seriesA.Hash()},
			},
			out: []*typesv1.Series{
				{
					Labels: seriesA,
					Points: []*typesv1.Point{
						{Timestamp: 1, Value: 1, Annotations: []*typesv1.ProfileAnnotation{}},
						{Timestamp: 2, Value: 1.5, Annotations: []*typesv1.ProfileAnnotation{}}, // avg of 1 and 2
						{Timestamp: 4, Value: 4, Annotations: []*typesv1.ProfileAnnotation{}},
						{Timestamp: 5, Value: 5, Annotations: []*typesv1.ProfileAnnotation{}},
					},
				},
				{
					Labels: seriesB,
					Points: []*typesv1.Point{
						{Timestamp: 1, Value: 1, Annotations: []*typesv1.ProfileAnnotation{}},
						{Timestamp: 3, Value: 1.5, Annotations: []*typesv1.ProfileAnnotation{}}, // avg of 1 and 2
						{Timestamp: 4, Value: 5, Annotations: []*typesv1.ProfileAnnotation{}},   // avg of 4 and 6
					},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := iter.NewSliceIterator(tc.in)
			aggregation := typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_AVERAGE
			out := RangeSeries(in, 1, 5, 1, &aggregation)
			testhelper.EqualProto(t, tc.out, out)
		})
	}
}

func Test_RangeSeriesRateNormalization(t *testing.T) {
	cpuLabels := NewLabelsBuilder(nil).Set("__type__", "cpu").Labels()

	testData := []TimeSeriesValue{
		{Ts: 0, Value: 0, Lbs: cpuLabels, LabelsHash: cpuLabels.Hash()},      // t=0: start
		{Ts: 15000, Value: 30, Lbs: cpuLabels, LabelsHash: cpuLabels.Hash()}, // t=15s: 30s CPU
		{Ts: 30000, Value: 0, Lbs: cpuLabels, LabelsHash: cpuLabels.Hash()},  // t=30s: reset
		{Ts: 45000, Value: 30, Lbs: cpuLabels, LabelsHash: cpuLabels.Hash()}, // t=45s: 30s CPU again
	}

	// Test with 15 second steps (step = 15000ms) - creates 4 bucket
	in15s := iter.NewSliceIterator(testData)
	rateAggregation := typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_RATE
	out15s := RangeSeries(in15s, 0, 60000, 15000, &rateAggregation)

	// Test with 30 second steps (step = 30000ms) - creates 2 buckets
	in30s := iter.NewSliceIterator(testData)
	out30s := RangeSeries(in30s, 0, 60000, 30000, &rateAggregation)

	var total15s, total30s float64
	for _, series := range out15s {
		for _, point := range series.Points {
			total15s += point.Value * 15.0
		}
	}

	for _, series := range out30s {
		for _, point := range series.Points {
			total30s += point.Value * 30.0
		}
	}

	assert.Equal(t, total15s, total30s)
	assert.Equal(t, 60.0, total15s)
	assert.Equal(t, 60.0, total30s)
}

func Test_NewTimeSeriesAggregatorSelection(t *testing.T) {
	stepDuration := 5.0

	tests := []struct {
		name         string
		aggregation  *typesv1.TimeSeriesAggregationType
		expectedType string
		description  string
	}{
		{
			name:         "No aggregation defaults to sum for backward compatibility",
			aggregation:  nil,
			expectedType: "sum",
		},
		{
			name:         "Explicit sum aggregation",
			aggregation:  &[]typesv1.TimeSeriesAggregationType{typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_SUM}[0],
			expectedType: "sum",
		},
		{
			name:         "Explicit rate aggregation",
			aggregation:  &[]typesv1.TimeSeriesAggregationType{typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_RATE}[0],
			expectedType: "rate",
		},
		{
			name:         "Explicit average aggregation",
			aggregation:  &[]typesv1.TimeSeriesAggregationType{typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_AVERAGE}[0],
			expectedType: "avg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aggregator := NewTimeSeriesAggregator(stepDuration, tt.aggregation)
			actualType := getAggregatorType(t, aggregator)
			if actualType != tt.expectedType {
				t.Errorf("Expected %s aggregator, got %s.", tt.expectedType, actualType)
			}
		})
	}
}

func getAggregatorType(t *testing.T, agg TimeSeriesAggregator) string {
	t.Helper()
	switch agg.(type) {
	case *sumTimeSeriesAggregator:
		return "sum"
	case *avgTimeSeriesAggregator:
		return "avg"
	case *rateTimeSeriesAggregator:
		return "rate"
	default:
		return "unknown"
	}
}

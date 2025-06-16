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
		{Ts: 1000, Value: 50, Lbs: cpuLabels, LabelsHash: cpuLabels.Hash()},
		{Ts: 1000, Value: 50, Lbs: cpuLabels, LabelsHash: cpuLabels.Hash()},
	}

	// Test with 1 second steps (step = 1000ms)
	in1s := iter.NewSliceIterator(testData)
	out1s := RangeSeries(in1s, 1000, 1000, 1000, nil)

	// Test with 5 second steps (step = 5000ms)
	in5s := iter.NewSliceIterator(testData)
	out5s := RangeSeries(in5s, 1000, 1000, 5000, nil)

	if len(out1s) != 1 || len(out1s[0].Points) != 1 {
		t.Fatal("Expected 1 series with 1 point for 1s test")
	}
	if len(out5s) != 1 || len(out5s[0].Points) != 1 {
		t.Fatal("Expected 1 series with 1 point for 5s test")
	}

	rate1s := out1s[0].Points[0].Value
	rate5s := out5s[0].Points[0].Value

	if rate1s <= rate5s {
		t.Errorf("Expected 1s rate (%f) to be higher than 5s rate (%f) due to normalization", rate1s, rate5s)
	}

	expectedRate1s := 100.0 / 1.0
	expectedRate5s := 100.0 / 5.0

	assert.Equal(t, expectedRate1s, rate1s)
	assert.Equal(t, expectedRate5s, rate5s)
}

func Test_NewTimeSeriesAggregatorSelection(t *testing.T) {
	stepDuration := 5.0

	tests := []struct {
		name         string
		profileType  string
		expectedType string
		description  string
	}{
		{
			name:         "CPU samples uses sum aggregation",
			profileType:  "samples",
			expectedType: "sum",
			description:  "CPU samples are count of sampling events (instant)",
		},
		{
			name:         "CPU time uses rate normalization",
			profileType:  "cpu",
			expectedType: "rate",
			description:  "CPU time is cumulative - actual time consumed over duration",
		},
		{
			name:         "Allocated objects uses rate normalization",
			profileType:  "alloc_objects",
			expectedType: "rate",
			description:  "Cumulative allocation profiles need rate normalization",
		},
		{
			name:         "Contentions use sum aggregation",
			profileType:  "contentions",
			expectedType: "sum",
			description:  "Contention events are instant counts that should be summed",
		},
		{
			name:         "Lock time uses rate normalization",
			profileType:  "lock_time",
			expectedType: "rate",
			description:  "Lock time is cumulative duration that needs rate normalization",
		},
		{
			name:         "In-use objects uses average aggregation",
			profileType:  "inuse_objects",
			expectedType: "avg",
			description:  "Instant memory profiles should be averaged",
		},
		{
			name:         "Goroutines use average aggregation",
			profileType:  "goroutine",
			expectedType: "avg",
			description:  "Goroutine counts are instant values that should be averaged",
		},
		{
			name:         "Unknown types default to rate normalization",
			profileType:  "unknown_profile_type",
			expectedType: "rate",
			description:  "Unknown profile types default to cumulative for safety",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labels := []*typesv1.LabelPair{
				{Name: "__type__", Value: tt.profileType},
			}

			aggregator := NewTimeSeriesAggregator(labels, stepDuration, nil)
			actualType := getAggregatorType(aggregator)

			if actualType != tt.expectedType {
				t.Errorf("Expected %s aggregator for %s, got %s. %s",
					tt.expectedType, tt.profileType, actualType, tt.description)
			}
		})
	}
}

func getAggregatorType(agg TimeSeriesAggregator) string {
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

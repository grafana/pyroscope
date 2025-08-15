package model

import (
	"testing"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/iter"
	"github.com/grafana/pyroscope/pkg/testhelper"
)

func Test_RangeSeriesSum(t *testing.T) {
	seriesA := NewLabelsBuilder(nil).Set("foo", "bar").Labels()
	seriesB := NewLabelsBuilder(nil).Set("foo", "buzz").Labels()
	for _, tc := range []struct {
		name string
		in   []TimeSeriesValue
		out  []*typesv1.Series
	}{
		{
			name: "single series",
			in: []TimeSeriesValue{
				{Ts: 1, Value: 1},
				{Ts: 1, Value: 1},
				{Ts: 2, Value: 2},
				{Ts: 3, Value: 3},
				{Ts: 4, Value: 4},
				{Ts: 5, Value: 5, Annotations: []*typesv1.ProfileAnnotation{{Key: "foo", Value: "bar"}}},
			},
			out: []*typesv1.Series{
				{
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
	seriesA := NewLabelsBuilder(nil).Set("foo", "bar").Labels()
	seriesB := NewLabelsBuilder(nil).Set("foo", "buzz").Labels()
	for _, tc := range []struct {
		name string
		in   []TimeSeriesValue
		out  []*typesv1.Series
	}{
		{
			name: "single series",
			in: []TimeSeriesValue{
				{Ts: 1, Value: 1},
				{Ts: 1, Value: 2},
				{Ts: 2, Value: 2},
				{Ts: 2, Value: 3},
				{Ts: 3, Value: 4},
				{Ts: 4, Value: 5, Annotations: []*typesv1.ProfileAnnotation{{Key: "foo", Value: "bar"}}},
			},
			out: []*typesv1.Series{
				{
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
		{
			name: "single series from multiple underlying series",
			in: []TimeSeriesValue{
				// we have 3 processes: a, b, c. All use 1 value, process c is scraping twice a second
				{Ts: 1, Value: 1, SeriesFingerprints: []uint64{0xa}},
				{Ts: 1, Value: 1, SeriesFingerprints: []uint64{0xb}},
				{Ts: 1, Value: 1, SeriesFingerprints: []uint64{0xc}},
				{Ts: 1, Value: 1, SeriesFingerprints: []uint64{0xc}},
			},
			out: []*typesv1.Series{
				{
					Points: []*typesv1.Point{
						{Timestamp: 1, Value: 3, SeriesFingerprints: []uint64{0xa, 0xb, 0xc}}, // build the avg of each process and sum it up
					},
				},
			},
		},
		{
			name: "single series",
			in: []TimeSeriesValue{
				{Ts: 1, Value: 1},
				{Ts: 1, Value: 2},
				{Ts: 2, Value: 2},
				{Ts: 2, Value: 3},
				{Ts: 3, Value: 4},
				{Ts: 4, Value: 5, Annotations: []*typesv1.ProfileAnnotation{{Key: "foo", Value: "bar"}}},
			},
			out: []*typesv1.Series{
				{
					Points: []*typesv1.Point{
						{Timestamp: 1, Value: 1.5, Annotations: []*typesv1.ProfileAnnotation{}}, // avg of 1 and 2
						{Timestamp: 2, Value: 2.5, Annotations: []*typesv1.ProfileAnnotation{}}, // avg of 2 and 3
						{Timestamp: 3, Value: 4, Annotations: []*typesv1.ProfileAnnotation{}},
						{Timestamp: 4, Value: 5, Annotations: []*typesv1.ProfileAnnotation{{Key: "foo", Value: "bar"}}},
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

package model

import (
	"testing"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/iter"
	"github.com/grafana/pyroscope/pkg/testhelper"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := iter.NewSliceIterator(tc.in)
			aggregation := typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_AVERAGE
			out := RangeSeries(in, 1, 5, 1, &aggregation)
			testhelper.EqualProto(t, tc.out, out)
		})
	}
}

func Test_RangeSeriesWithExemplars(t *testing.T) {
	sum := typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_SUM

	for _, tc := range []struct {
		name         string
		series       []*typesv1.Series
		start        int64
		end          int64
		step         int64
		aggregation  *typesv1.TimeSeriesAggregationType
		maxExemplars int // 0 means use default
		out          []*typesv1.Series
	}{
		{
			name: "exemplar tracking keeps highest value",
			series: []*typesv1.Series{{
				Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "api"}},
				Points: []*typesv1.Point{
					{Timestamp: 1000, Value: 100.0, Exemplars: []*typesv1.Exemplar{{ProfileId: "prof-1", Value: 100, Timestamp: 1000}}},
					{Timestamp: 1000, Value: 300.0, Exemplars: []*typesv1.Exemplar{{ProfileId: "prof-2", Value: 300, Timestamp: 1000}}},
					{Timestamp: 2000, Value: 200.0, Exemplars: []*typesv1.Exemplar{{ProfileId: "prof-3", Value: 200, Timestamp: 2000}}},
				},
			}},
			start:       1000,
			end:         3000,
			step:        1000,
			aggregation: &sum,
			out: []*typesv1.Series{{
				Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "api"}},
				Points: []*typesv1.Point{
					{Timestamp: 1000, Value: 400.0, Annotations: []*typesv1.ProfileAnnotation{}, Exemplars: []*typesv1.Exemplar{{ProfileId: "prof-2", Value: 300, Timestamp: 1000}}},
					{Timestamp: 2000, Value: 200.0, Annotations: []*typesv1.ProfileAnnotation{}, Exemplars: []*typesv1.Exemplar{{ProfileId: "prof-3", Value: 200, Timestamp: 2000}}},
				},
			}},
		},
		{
			name: "exemplar labels preserved through re-aggregation",
			series: []*typesv1.Series{{
				Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "api"}},
				Points: []*typesv1.Point{
					{
						Timestamp: 1000,
						Value:     100.0,
						Exemplars: []*typesv1.Exemplar{{
							ProfileId: "prof-1",
							Value:     100,
							Timestamp: 1000,
							Labels:    []*typesv1.LabelPair{{Name: "pod", Value: "pod-123"}, {Name: "region", Value: "us-east"}},
						}},
					},
				},
			}},
			start:       1000,
			end:         2000,
			step:        1000,
			aggregation: &sum,
			out: []*typesv1.Series{{
				Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "api"}},
				Points: []*typesv1.Point{
					{
						Timestamp:   1000,
						Value:       100.0,
						Annotations: []*typesv1.ProfileAnnotation{},
						Exemplars: []*typesv1.Exemplar{{
							ProfileId: "prof-1",
							Value:     100,
							Timestamp: 1000,
							Labels:    []*typesv1.LabelPair{{Name: "pod", Value: "pod-123"}, {Name: "region", Value: "us-east"}},
						}},
					},
				},
			}},
		},
		{
			name: "multi-block path supports top-2 exemplars",
			series: []*typesv1.Series{{
				Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "api"}},
				Points: []*typesv1.Point{
					{
						Timestamp: 1000,
						Value:     100.0,
						Exemplars: []*typesv1.Exemplar{
							{ProfileId: "prof-1", Value: 100, Timestamp: 1000, Labels: []*typesv1.LabelPair{{Name: "pod", Value: "pod-1"}}},
							{ProfileId: "prof-2", Value: 200, Timestamp: 1000, Labels: []*typesv1.LabelPair{{Name: "pod", Value: "pod-2"}}},
						},
					},
					{
						Timestamp: 1000,
						Value:     150.0,
						Exemplars: []*typesv1.Exemplar{
							{ProfileId: "prof-3", Value: 300, Timestamp: 1000, Labels: []*typesv1.LabelPair{{Name: "pod", Value: "pod-3"}}},
							{ProfileId: "prof-4", Value: 50, Timestamp: 1000, Labels: []*typesv1.LabelPair{{Name: "pod", Value: "pod-4"}}},
						},
					},
				},
			}},
			start:        1000,
			end:          2000,
			step:         1000,
			aggregation:  &sum,
			maxExemplars: 2,
			out: []*typesv1.Series{{
				Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "api"}},
				Points: []*typesv1.Point{
					{
						Timestamp:   1000,
						Value:       250.0,
						Annotations: []*typesv1.ProfileAnnotation{},
						Exemplars: []*typesv1.Exemplar{
							{ProfileId: "prof-3", Value: 300, Timestamp: 1000, Labels: []*typesv1.LabelPair{{Name: "pod", Value: "pod-3"}}},
							{ProfileId: "prof-2", Value: 200, Timestamp: 1000, Labels: []*typesv1.LabelPair{{Name: "pod", Value: "pod-2"}}},
						},
					},
				},
			}},
		},
		{
			name: "same profileID across blocks - aggregates values correctly",
			series: []*typesv1.Series{{
				Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "api"}},
				Points: []*typesv1.Point{
					{
						Timestamp: 1000,
						Value:     100.0,
						Exemplars: []*typesv1.Exemplar{
							{ProfileId: "Profile-X", Value: 100, Timestamp: 1000, Labels: []*typesv1.LabelPair{{Name: "block", Value: "A"}}},
							{ProfileId: "Profile-Y", Value: 60, Timestamp: 1000, Labels: []*typesv1.LabelPair{{Name: "block", Value: "A"}}},
							{ProfileId: "Profile-Z", Value: 40, Timestamp: 1000, Labels: []*typesv1.LabelPair{{Name: "block", Value: "A"}}},
						},
					},
					{
						Timestamp: 1000,
						Value:     140.0,
						Exemplars: []*typesv1.Exemplar{
							{ProfileId: "Profile-X", Value: 20, Timestamp: 1000, Labels: []*typesv1.LabelPair{{Name: "block", Value: "B"}}},
							{ProfileId: "Profile-Y", Value: 30, Timestamp: 1000, Labels: []*typesv1.LabelPair{{Name: "block", Value: "B"}}},
							{ProfileId: "Profile-Z", Value: 90, Timestamp: 1000, Labels: []*typesv1.LabelPair{{Name: "block", Value: "B"}}},
						},
					},
					{
						Timestamp: 1000,
						Value:     105.0,
						Exemplars: []*typesv1.Exemplar{
							{ProfileId: "Profile-X", Value: 10, Timestamp: 1000, Labels: []*typesv1.LabelPair{{Name: "block", Value: "C"}}},
							{ProfileId: "Profile-Y", Value: 80, Timestamp: 1000, Labels: []*typesv1.LabelPair{{Name: "block", Value: "C"}}},
							{ProfileId: "Profile-Z", Value: 15, Timestamp: 1000, Labels: []*typesv1.LabelPair{{Name: "block", Value: "C"}}},
						},
					},
				},
			}},
			start:       1000,
			end:         2000,
			step:        1000,
			aggregation: &sum,
			out: []*typesv1.Series{{
				Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "api"}},
				Points: []*typesv1.Point{
					{
						Timestamp:   1000,
						Value:       345.0, // 100+140+105
						Annotations: []*typesv1.ProfileAnnotation{},
						Exemplars: []*typesv1.Exemplar{
							{ProfileId: "Profile-Y", Value: 170, Timestamp: 1000, Labels: []*typesv1.LabelPair{{Name: "block", Value: "A"}}},
						},
					},
				},
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			iter := NewTimeSeriesMergeIterator(tc.series)
			var result []*typesv1.Series
			if tc.maxExemplars > 0 {
				result = rangeSeriesWithLimit(iter, tc.start, tc.end, tc.step, tc.aggregation, tc.maxExemplars)
			} else {
				result = RangeSeries(iter, tc.start, tc.end, tc.step, tc.aggregation)
			}
			testhelper.EqualProto(t, tc.out, result)
		})
	}
}

func TestMergeAndSelectExemplars_SumsValuesForSameProfileID(t *testing.T) {
	t.Run("top-2 selection after aggregation", func(t *testing.T) {
		var existing []*exemplarCandidate

		blockA := []*exemplarCandidate{
			{profileID: "A", value: 50},
			{profileID: "B", value: 30},
			{profileID: "C", value: 20},
		}
		existing = mergeExemplarCandidates(existing, blockA)

		blockB := []*exemplarCandidate{
			{profileID: "A", value: 10},
			{profileID: "B", value: 40},
			{profileID: "C", value: 50},
		}
		merged := mergeExemplarCandidates(existing, blockB)

		result := selectTopNExemplars(merged, 2)

		require.Len(t, result, 2)

		byProfileID := make(map[string]uint64)
		for _, ex := range result {
			byProfileID[ex.profileID] = ex.value
		}

		assert.Contains(t, byProfileID, "B")
		assert.Equal(t, uint64(70), byProfileID["B"])
		assert.Contains(t, byProfileID, "C")
		assert.Equal(t, uint64(70), byProfileID["C"])
	})

	t.Run("single block backward compatibility", func(t *testing.T) {
		candidates := []*exemplarCandidate{
			{profileID: "prof-1", value: 100},
			{profileID: "prof-2", value: 300},
			{profileID: "prof-3", value: 200},
		}

		merged := mergeExemplarCandidates(nil, candidates)
		result := selectTopNExemplars(merged, 1)

		require.Len(t, result, 1)
		assert.Equal(t, "prof-2", result[0].profileID)
		assert.Equal(t, uint64(300), result[0].value)
	})

	t.Run("duplicate profileID in same batch gets summed", func(t *testing.T) {
		candidates := []*exemplarCandidate{
			{profileID: "prof-A", value: 100},
			{profileID: "prof-A", value: 50},
			{profileID: "prof-B", value: 120},
		}

		merged := mergeExemplarCandidates(nil, candidates)
		result := selectTopNExemplars(merged, 1)

		require.Len(t, result, 1)
		assert.Equal(t, "prof-A", result[0].profileID)
		assert.Equal(t, uint64(150), result[0].value)
	})

	t.Run("empty exemplars", func(t *testing.T) {
		result := mergeExemplarCandidates(nil, nil)
		assert.Nil(t, result)

		result = mergeExemplarCandidates(nil, []*exemplarCandidate{})
		assert.Nil(t, result)
	})

	t.Run("fewer exemplars than max", func(t *testing.T) {
		candidates := []*exemplarCandidate{
			{profileID: "prof-1", value: 100},
		}

		merged := mergeExemplarCandidates(nil, candidates)
		result := selectTopNExemplars(merged, 5)

		require.Len(t, result, 1)
		assert.Equal(t, "prof-1", result[0].profileID)
	})
}

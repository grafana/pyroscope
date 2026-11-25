package timeseriescompact

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

func TestMergeExemplars_Empty(t *testing.T) {
	assert.Nil(t, MergeExemplars(nil, nil))
	assert.Equal(t, []*queryv1.Exemplar{{ProfileId: "a"}}, MergeExemplars(nil, []*queryv1.Exemplar{{ProfileId: "a"}}))
	assert.Equal(t, []*queryv1.Exemplar{{ProfileId: "a"}}, MergeExemplars([]*queryv1.Exemplar{{ProfileId: "a"}}, nil))
}

func TestMergeExemplars_DifferentProfiles(t *testing.T) {
	a := []*queryv1.Exemplar{{ProfileId: "prof-1", Value: 100}}
	b := []*queryv1.Exemplar{{ProfileId: "prof-2", Value: 200}}

	result := MergeExemplars(a, b)
	require.Len(t, result, 2)

	// Sorted by profile ID
	assert.Equal(t, "prof-1", result[0].ProfileId)
	assert.Equal(t, "prof-2", result[1].ProfileId)
}

func TestMergeExemplars_SameProfile_KeepsHigherValue(t *testing.T) {
	a := []*queryv1.Exemplar{{ProfileId: "prof-1", Value: 100, AttributeRefs: []int64{1, 2}}}
	b := []*queryv1.Exemplar{{ProfileId: "prof-1", Value: 200, AttributeRefs: []int64{1, 2}}}

	result := MergeExemplars(a, b)
	require.Len(t, result, 1)
	assert.Equal(t, "prof-1", result[0].ProfileId)
	assert.Equal(t, int64(200), result[0].Value) // Higher value kept
}

func TestMergeExemplars_SameProfile_IntersectsRefs(t *testing.T) {
	// Same profile with different attribute refs should intersect
	a := []*queryv1.Exemplar{{ProfileId: "prof-1", Value: 100, AttributeRefs: []int64{1, 2, 3}}}
	b := []*queryv1.Exemplar{{ProfileId: "prof-1", Value: 50, AttributeRefs: []int64{2, 3, 4}}}

	result := MergeExemplars(a, b)
	require.Len(t, result, 1)

	// Should keep higher value exemplar
	assert.Equal(t, int64(100), result[0].Value)

	// Refs should be intersected: [1,2,3] ∩ [2,3,4] = [2,3]
	assert.ElementsMatch(t, []int64{2, 3}, result[0].AttributeRefs)
}

func TestMergeExemplars_SameProfile_NoCommonRefs(t *testing.T) {
	a := []*queryv1.Exemplar{{ProfileId: "prof-1", Value: 100, AttributeRefs: []int64{1, 2}}}
	b := []*queryv1.Exemplar{{ProfileId: "prof-1", Value: 50, AttributeRefs: []int64{3, 4}}}

	result := MergeExemplars(a, b)
	require.Len(t, result, 1)

	// No common refs
	assert.Empty(t, result[0].AttributeRefs)
}

func TestIntersectRefs_Empty(t *testing.T) {
	assert.Nil(t, IntersectRefs(nil))
	assert.Nil(t, IntersectRefs([][]int64{}))
}

func TestIntersectRefs_SingleSet(t *testing.T) {
	refs := [][]int64{{1, 2, 3}}
	result := IntersectRefs(refs)
	assert.Equal(t, []int64{1, 2, 3}, result)
}

func TestIntersectRefs_TwoSets(t *testing.T) {
	refs := [][]int64{
		{1, 2, 3, 4},
		{2, 3, 4, 5},
	}
	result := IntersectRefs(refs)
	assert.Equal(t, []int64{2, 3, 4}, result)
}

func TestIntersectRefs_ThreeSets(t *testing.T) {
	refs := [][]int64{
		{1, 2, 3, 4, 5},
		{2, 3, 4, 5, 6},
		{3, 4, 5, 6, 7},
	}
	result := IntersectRefs(refs)
	assert.Equal(t, []int64{3, 4, 5}, result)
}

func TestIntersectRefs_NoCommon(t *testing.T) {
	refs := [][]int64{
		{1, 2},
		{3, 4},
	}
	result := IntersectRefs(refs)
	assert.Nil(t, result)
}

func TestIntersectRefs_SortedOutput(t *testing.T) {
	refs := [][]int64{
		{5, 3, 1, 4, 2},
		{4, 2, 5, 1, 3},
	}
	result := IntersectRefs(refs)
	// Should be sorted
	assert.Equal(t, []int64{1, 2, 3, 4, 5}, result)
}

func TestDedupeRefs_Empty(t *testing.T) {
	assert.Nil(t, DedupeRefs(nil))
	assert.Equal(t, []int64{}, DedupeRefs([]int64{}))
}

func TestDedupeRefs_Single(t *testing.T) {
	assert.Equal(t, []int64{1}, DedupeRefs([]int64{1}))
}

func TestDedupeRefs_NoDuplicates(t *testing.T) {
	result := DedupeRefs([]int64{3, 1, 2})
	assert.Equal(t, []int64{1, 2, 3}, result) // Sorted
}

func TestDedupeRefs_WithDuplicates(t *testing.T) {
	result := DedupeRefs([]int64{3, 1, 2, 1, 3, 2, 1})
	assert.Equal(t, []int64{1, 2, 3}, result)
}

func TestSelectTopExemplars_LessThanN(t *testing.T) {
	exemplars := []*queryv1.Exemplar{
		{ProfileId: "a", Value: 100},
		{ProfileId: "b", Value: 200},
	}
	result := SelectTopExemplars(exemplars, 5)
	assert.Len(t, result, 2)
}

func TestSelectTopExemplars_MoreThanN(t *testing.T) {
	exemplars := []*queryv1.Exemplar{
		{ProfileId: "a", Value: 100},
		{ProfileId: "b", Value: 300},
		{ProfileId: "c", Value: 200},
		{ProfileId: "d", Value: 400},
	}
	result := SelectTopExemplars(exemplars, 2)
	require.Len(t, result, 2)

	// Should keep top 2 by value (400, 300)
	assert.Equal(t, int64(400), result[0].Value)
	assert.Equal(t, int64(300), result[1].Value)
}

func TestAggregator_SameProfileIntersectsAndResets(t *testing.T) {
	var agg Aggregator

	agg.Add(1000, &CompactValue{
		Ts:             100,
		Value:          100,
		AnnotationRefs: []int64{1, 2},
		Exemplars: []*queryv1.Exemplar{
			{ProfileId: "prof-z", Value: 50, AttributeRefs: []int64{1}},
			{ProfileId: "prof-a", Value: 200, AttributeRefs: []int64{1, 2, 3}},
		},
	})
	agg.Add(1000, &CompactValue{
		Ts:             200,
		Value:          50,
		AnnotationRefs: []int64{2, 3},
		Exemplars: []*queryv1.Exemplar{
			{ProfileId: "prof-a", Value: 150, AttributeRefs: []int64{2, 3, 4}},
			{ProfileId: "prof-b", Value: 180, AttributeRefs: []int64{9}},
		},
	})

	pt1 := agg.GetAndReset()
	assert.True(t, agg.IsEmpty())
	assert.Equal(t, int64(1000), pt1.Timestamp)
	assert.Equal(t, 150.0, pt1.Value)
	assert.Equal(t, []int64{1, 2, 3}, pt1.AnnotationRefs)
	require.Len(t, pt1.Exemplars, 1)
	assert.Equal(t, "prof-a", pt1.Exemplars[0].ProfileId)
	assert.Equal(t, int64(200), pt1.Exemplars[0].Value)
	assert.Equal(t, []int64{2, 3}, pt1.Exemplars[0].AttributeRefs)

	// Subsequent step must not retain state from previous step.
	agg.Add(2000, &CompactValue{
		Ts:    1100,
		Value: 40,
		Exemplars: []*queryv1.Exemplar{
			{ProfileId: "prof-c", Value: 40, AttributeRefs: []int64{7}},
		},
	})
	pt2 := agg.GetAndReset()
	require.Len(t, pt2.Exemplars, 1)
	assert.Equal(t, "prof-c", pt2.Exemplars[0].ProfileId)
	assert.Equal(t, []int64{7}, pt2.Exemplars[0].AttributeRefs)
}

func TestSelectTopExemplars_TieBreakByProfileID(t *testing.T) {
	exemplars := []*queryv1.Exemplar{
		{ProfileId: "prof-b", Value: 200},
		{ProfileId: "prof-a", Value: 200},
	}
	result := SelectTopExemplars(exemplars, 1)
	require.Len(t, result, 1)
	assert.Equal(t, "prof-a", result[0].ProfileId)
}

func BenchmarkAggregator_ManyExemplarsInStep(b *testing.B) {
	for _, pointsPerStep := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("points=%d", pointsPerStep), func(b *testing.B) {
			values := make([]CompactValue, pointsPerStep)
			for i := range values {
				values[i] = CompactValue{
					Ts:             int64(i),
					Value:          float64(i + 1),
					AnnotationRefs: []int64{0},
					Exemplars: []*queryv1.Exemplar{{
						ProfileId:     fmt.Sprintf("00000000-0000-0000-0000-%012d", i),
						Timestamp:     int64(i),
						Value:         int64(i + 1),
						AttributeRefs: []int64{1, 2, 3},
					}},
				}
			}

			b.ReportAllocs()
			b.ResetTimer()
			var agg Aggregator
			for i := 0; i < b.N; i++ {
				for j := range values {
					agg.Add(1000, &values[j])
				}
				pt := agg.GetAndReset()
				if len(pt.Exemplars) != 1 || pt.Exemplars[0].Value != int64(pointsPerStep) {
					b.Fatalf("unexpected exemplar: %+v", pt.Exemplars)
				}
			}
		})
	}
}

package timeseries

import (
	"testing"

	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func TestAggregator_DeduplicatesAnnotations(t *testing.T) {
	for _, aggregation := range []typesv1.TimeSeriesAggregationType{
		typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_SUM,
		typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_AVERAGE,
	} {
		t.Run(aggregation.String(), func(t *testing.T) {
			a := NewAggregator(&aggregation)
			annotations := []*typesv1.ProfileAnnotation{
				{Key: "deployment", Value: "a"},
				{Key: "deployment", Value: "b"},
				{Key: "other", Value: "a"},
			}
			a.Add(1000, &Value{Value: 2, Annotations: append(annotations, &typesv1.ProfileAnnotation{Key: "deployment", Value: "a"})})
			// Separate objects with identical contents must also deduplicate.
			a.Add(1000, &Value{Value: 4, Annotations: []*typesv1.ProfileAnnotation{{Key: "deployment", Value: "a"}}})
			point := a.GetAndReset()
			require.Equal(t, annotations, point.Annotations)
			require.Equal(t, int64(1000), point.Timestamp)
			want := float64(6)
			if aggregation == typesv1.TimeSeriesAggregationType_TIME_SERIES_AGGREGATION_TYPE_AVERAGE {
				want = 3
			}
			require.Equal(t, want, point.Value)
			require.True(t, a.IsEmpty())

			// Reset must clear deduplication state without changing prior output.
			a.Add(2000, &Value{Value: 8, Annotations: annotations[:1]})
			next := a.GetAndReset()
			require.Equal(t, annotations[:1], next.Annotations)
			require.Equal(t, annotations, point.Annotations)
			a.Add(3000, &Value{Value: 1})
			require.Empty(t, a.GetAndReset().Annotations)
		})
	}
}

func TestAnnotationAccumulator_RepeatedAnnotations(t *testing.T) {
	var a annotationAccumulator
	for range 10000 {
		a.add([]*typesv1.ProfileAnnotation{{Key: "deployment", Value: "a"}})
	}
	require.Len(t, a.points, 1)
	require.Len(t, a.seen, 1)
	require.Len(t, a.getAndReset(), 1)
	require.Empty(t, a.seen)
}

package timeseries

import (
	"fmt"
	"testing"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

// buildBenchSeries builds series with points from two interleaved sources, so
// mergePoints has real sorting and summing work to do.
func buildBenchSeries(numSeries, numPoints int) [][]*typesv1.Series {
	sources := make([][]*typesv1.Series, 2)
	for src := range sources {
		series := make([]*typesv1.Series, numSeries)
		for i := range series {
			points := make([]*typesv1.Point, numPoints)
			for j := range points {
				points[j] = &typesv1.Point{
					Timestamp: int64(j * 15000),
					Value:     float64(i + j),
				}
			}
			series[i] = &typesv1.Series{
				Labels: []*typesv1.LabelPair{
					{Name: "__name__", Value: "process_cpu"},
					{Name: "pod", Value: fmt.Sprintf("pod-%d", i)},
					{Name: "namespace", Value: "default"},
				},
				Points: points,
			}
		}
		sources[src] = series
	}
	return sources
}

func BenchmarkMergeSeries(b *testing.B) {
	for _, tc := range []struct{ numSeries, numPoints int }{
		{10, 1000},
		{100, 100},
		{1000, 10},
	} {
		b.Run(fmt.Sprintf("series=%d/points=%d", tc.numSeries, tc.numPoints), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				sources := buildBenchSeries(tc.numSeries, tc.numPoints)
				b.StartTimer()
				MergeSeries(nil, sources...)
			}
		})
	}
}

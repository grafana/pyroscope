package timeseries

import (
	"testing"

	"github.com/grafana/pyroscope/v2/pkg/iter"
)

func BenchmarkRangeDenseBucket(b *testing.B) {
	points := make([]Value, 4096)
	for i := range points {
		points[i] = Value{Ts: int64(i), Value: 1}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RangeSeries(iter.NewSliceIterator(points), 4096, 4096, 4096, nil)
	}
}

package heatmap

import (
	"cmp"
	"slices"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/model"
)

// TopSeries returns the top k heatmap series by sum of counts.
func TopSeries(s []*typesv1.HeatmapSeries, k int) []*typesv1.HeatmapSeries {
	if k <= 0 || len(s) <= k {
		return s
	}

	type series struct {
		*typesv1.HeatmapSeries
		sum int64
	}

	aggregated := make([]series, len(s))
	for i, x := range s {
		var sum int64
		for _, slot := range x.Slots {
			for _, count := range slot.Counts {
				sum += int64(count)
			}
		}
		aggregated[i] = series{HeatmapSeries: x, sum: sum}
	}

	slices.SortFunc(aggregated, func(a, b series) int {
		c := cmp.Compare(a.sum, b.sum)
		if c == 0 {
			return model.CompareLabelPairs(a.Labels, b.Labels)
		}
		return -c // Descending order
	})

	for i, a := range aggregated {
		s[i] = a.HeatmapSeries
	}

	return s[:k]
}

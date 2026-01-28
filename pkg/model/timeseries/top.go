package timeseries

import (
	"cmp"
	"slices"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

// TopSeries returns the top k series by sum of values.
// If k is zero, all series are returned.
// Note that even if len(c) <= k or k == 0, the returned
// series are sorted by value in descending order and then
// lexicographically (in ascending order).
func TopSeries(s []*typesv1.Series, k int) []*typesv1.Series {
	type series struct {
		*typesv1.Series
		sum float64
	}
	aggregated := make([]series, len(s))
	for i, x := range s {
		var sum float64
		for _, p := range x.Points {
			sum += p.Value
		}
		aggregated[i] = series{Series: x, sum: sum}
	}
	slices.SortFunc(aggregated, func(a, b series) int {
		c := cmp.Compare(a.sum, b.sum)
		if c == 0 {
			return phlaremodel.CompareLabelPairs(a.Labels, b.Labels)
		}
		return -c // Invert to sort in descending order.
	})
	for i, a := range aggregated {
		s[i] = a.Series
	}
	if k > 0 && len(s) > k {
		return s[:k]
	}
	return s
}

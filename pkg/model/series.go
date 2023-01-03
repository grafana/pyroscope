package model

import (
	"sort"

	"github.com/samber/lo"

	typesv1alpha1 "github.com/grafana/phlare/api/gen/proto/go/types/v1alpha1"
)

func MergeSeries(series ...[]*typesv1alpha1.Series) []*typesv1alpha1.Series {
	if len(series) == 0 {
		return nil
	}
	if len(series) == 1 {
		return series[0]
	}
	seriesByFingerprint := map[uint64]*typesv1alpha1.Series{}
	for _, s := range series {
		for _, s := range s {
			hash := Labels(s.Labels).Hash()
			found, ok := seriesByFingerprint[hash]
			if !ok {
				seriesByFingerprint[hash] = s
				continue
			}
			found.Points = append(found.Points, s.Points...)
		}
	}
	result := lo.Values(seriesByFingerprint)
	sort.Slice(result, func(i, j int) bool {
		return CompareLabelPairs(result[i].Labels, result[j].Labels) < 0
	})
	for _, s := range result {
		sort.Slice(s.Points, func(i, j int) bool {
			return s.Points[i].Timestamp < s.Points[j].Timestamp
		})
	}
	return result
}

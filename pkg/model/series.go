package model

import (
	"sort"

	"github.com/samber/lo"

	commonv1 "github.com/grafana/fire/pkg/gen/common/v1"
)

func MergeSeries(series ...[]*commonv1.Series) []*commonv1.Series {
	if len(series) == 0 {
		return nil
	}
	if len(series) == 1 {
		return series[0]
	}
	seriesByFingerprint := map[uint64]*commonv1.Series{}
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

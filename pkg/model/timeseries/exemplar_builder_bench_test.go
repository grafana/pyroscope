package timeseries

import (
	"strconv"
	"testing"

	phlaremodel "github.com/grafana/pyroscope/v2/pkg/model"
)

// Measure label intersection separately from scanning, sorting and bucketing.
// Each group represents samples from one profile at one timestamp.
func BenchmarkExemplarLabelSamples(b *testing.B) {
	const samples = 4096
	for _, tc := range []struct {
		name            string
		groupSize, refs int
	}{
		{"unique", 1, 1},
		{"duplicates_same_labels", 8, 1},
		{"duplicates_overlapping_labels", 8, 8},
	} {
		b.Run(tc.name, func(b *testing.B) {
			eb := newExemplarBuilder()
			for i := 0; i < tc.refs; i++ {
				eb.labelSets = append(eb.labelSets, phlaremodel.LabelsFromStrings("cluster", "prod", "pod", strconv.Itoa(i), "service", "api"))
			}
			for i := 0; i < samples; i++ {
				group := i / tc.groupSize
				eb.exemplars = append(eb.exemplars, exemplar{timestamp: int64(group), profileID: strconv.Itoa(group), labelSetRef: i % tc.refs, value: 1})
			}
			b.ReportAllocs()
			for b.Loop() {
				eb.deduplicateAndIntersect()
			}
		})
	}
}

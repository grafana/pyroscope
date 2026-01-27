package heatmap

import (
	"slices"
	"sort"
	"strconv"
	"strings"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/model/attributetable"
)

type Merger struct {
	merger *attributetable.Merger[*queryv1.HeatmapPoint, string]
	sum    bool
}

// NewMerger creates a new heatmap merger. If sum is set, points
// with matching timestamps, profileIDs, and spanIDs have their values summed.
func NewMerger(sum bool) *Merger {
	return &Merger{
		merger: attributetable.NewMerger[*queryv1.HeatmapPoint, string](),
		sum:    sum,
	}
}

// MergeHeatmap adds a heatmap report to the merger
func (m *Merger) MergeHeatmap(report *queryv1.HeatmapReport) {
	if report == nil || len(report.HeatmapSeries) == 0 {
		return
	}

	m.merger.Lock()
	defer m.merger.Unlock()

	// Build a mapping from old attribute refs to new attribute refs
	refMap := m.merger.RemapAttributeTable(report.AttributeTable)

	for _, s := range report.HeatmapSeries {
		// Remap the series attribute refs
		remappedSeriesRefs := m.merger.RemapRefs(s.AttributeRefs, refMap)
		key := seriesKey(remappedSeriesRefs)

		existing := m.merger.GetOrCreateSeries(key, remappedSeriesRefs)

		// Ensure slice is big enough to hold all the points
		existing.Points = slices.Grow(existing.Points, len(s.Points))

		// Remap and add points
		for _, p := range s.Points {
			remappedPoint := &queryv1.HeatmapPoint{
				Timestamp:     p.Timestamp,
				ProfileId:     remapRef(p.ProfileId, refMap),
				SpanId:        p.SpanId,
				Value:         p.Value,
				AttributeRefs: m.merger.RemapRefs(p.AttributeRefs, refMap),
			}
			existing.Points = append(existing.Points, remappedPoint)
		}
	}
}

// remapRef remaps a single ref (like profile ID) using the provided mapping
func remapRef(ref int64, refMap map[int64]int64) int64 {
	// if we are the first report, we don't have a refMap yet
	if refMap == nil {
		return ref
	}
	if newRef, ok := refMap[ref]; ok {
		return newRef
	}
	panic("ref not found in attribute table")
}

// IsEmpty returns true if no series have been merged
func (m *Merger) IsEmpty() bool {
	return m.merger.IsEmpty()
}

// Build returns the merged heatmap report
func (m *Merger) Build() *queryv1.HeatmapReport {
	m.merger.Lock()
	defer m.merger.Unlock()

	series := m.merger.Series()
	if len(series) == 0 {
		return &queryv1.HeatmapReport{}
	}

	report := &queryv1.HeatmapReport{
		HeatmapSeries: make([]*queryv1.HeatmapSeries, 0, len(series)),
	}

	// Get sorted keys for deterministic output
	keys := make([]string, 0, len(series))
	for k := range series {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		ms := series[key]
		heatmapSeries := &queryv1.HeatmapSeries{
			AttributeRefs: ms.AttributeRefs,
			Points:        ms.Points[:m.mergePoints(ms.Points)],
		}
		report.HeatmapSeries = append(report.HeatmapSeries, heatmapSeries)
	}

	report.AttributeTable = m.merger.BuildAttributeTable(report.AttributeTable)

	return report
}

// Heatmaps returns the merged heatmap reports for iteration
func (m *Merger) Heatmaps() []*queryv1.HeatmapReport {
	return []*queryv1.HeatmapReport{m.Build()}
}

// seriesKey creates a string key from attribute refs
func seriesKey(refs []int64) string {
	var sb strings.Builder
	for i, ref := range refs {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.FormatInt(ref, 16))
	}
	return sb.String()
}

// mergePoints merges points with the same timestamp/profileID/spanID
func (m *Merger) mergePoints(points []*queryv1.HeatmapPoint) int {
	l := len(points)
	if l < 2 {
		return l
	}

	// Sort by timestamp, then spanID, then profileID
	slices.SortFunc(points, func(a, b *queryv1.HeatmapPoint) int {
		if a.Timestamp != b.Timestamp {
			return int(a.Timestamp - b.Timestamp)
		}
		if a.SpanId != b.SpanId {
			return int(a.SpanId - b.SpanId)
		}
		if a.ProfileId != b.ProfileId {
			return int(a.ProfileId - b.ProfileId)
		}
		return 0
	})

	var j int
	for i := 1; i < l; i++ {
		// Check if we should merge
		if points[j].Timestamp != points[i].Timestamp ||
			points[j].ProfileId != points[i].ProfileId ||
			points[j].SpanId != points[i].SpanId ||
			!m.sum {
			j++
			points[j] = points[i]
			continue
		}

		// Sum the values and merge attribute refs
		if m.sum {
			points[j].Value += points[i].Value
			points[j].AttributeRefs = mergeAttributeRefs(points[j].AttributeRefs, points[i].AttributeRefs)
		}
	}

	return j + 1
}

// mergeAttributeRefs merges two attribute ref slices, keeping unique refs
func mergeAttributeRefs(a, b []int64) []int64 {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}

	// Merge and deduplicate
	merged := append(append([]int64{}, a...), b...)
	slices.Sort(merged)

	j := 0
	for i := 1; i < len(merged); i++ {
		if merged[j] != merged[i] {
			j++
			merged[j] = merged[i]
		}
	}

	return merged[:j+1]
}

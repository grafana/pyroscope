package heatmap

import (
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/model/attributetable"
)

type Merger struct {
	mu       sync.Mutex
	atMerger *attributetable.Merger
	sum      bool
	series   map[string]*atHeatmapSeries
}

// NewMerger creates a new heatmap merger. If sum is set, points
// with matching timestamps, profileIDs, and spanIDs have their values summed.
func NewMerger(sum bool) *Merger {
	return &Merger{
		atMerger: attributetable.NewMerger(),
		sum:      sum,
		series:   make(map[string]*atHeatmapSeries),
	}
}

type atHeatmapSeries struct {
	attributeRefs []int64
	points        []*queryv1.HeatmapPoint
}

// MergeHeatmap adds a heatmap report to the merger
func (m *Merger) MergeHeatmap(report *queryv1.HeatmapReport) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if report == nil || len(report.HeatmapSeries) == 0 {
		return
	}

	// Build a mapping from old attribute refs to new attribute refs
	m.atMerger.Merge(report.AttributeTable, func(remapper *attributetable.Remapper) {
		for _, s := range report.HeatmapSeries {
			// Remap the series attribute refs
			remappedSeriesRefs := remapper.Refs(s.AttributeRefs)
			key := seriesKey(remappedSeriesRefs)

			existing, ok := m.series[key]
			if !ok {
				existing = &atHeatmapSeries{
					attributeRefs: remappedSeriesRefs,
				}
				m.series[key] = existing
			}

			// Ensure slice is big enough to hold all the points
			existing.points = slices.Grow(existing.points, len(s.Points))

			// Remap and add points
			for _, p := range s.Points {
				remappedPoint := &queryv1.HeatmapPoint{
					Timestamp:     p.Timestamp,
					ProfileId:     remapper.Ref(p.ProfileId),
					SpanId:        p.SpanId,
					Value:         p.Value,
					AttributeRefs: remapper.Refs(p.AttributeRefs),
				}
				existing.points = append(existing.points, remappedPoint)
			}
		}

	})
}

// IsEmpty returns true if no series have been merged
func (m *Merger) IsEmpty() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.series) == 0
}

// Build returns the merged heatmap report
func (m *Merger) Build() *queryv1.HeatmapReport {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.series) == 0 {
		return &queryv1.HeatmapReport{}
	}

	report := &queryv1.HeatmapReport{
		HeatmapSeries: make([]*queryv1.HeatmapSeries, 0, len(m.series)),
	}

	// Get sorted keys for deterministic output
	keys := make([]string, 0, len(m.series))
	for k := range m.series {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		ms := m.series[key]
		heatmapSeries := &queryv1.HeatmapSeries{
			AttributeRefs: ms.attributeRefs,
			Points:        ms.points[:m.mergePoints(ms.points)],
		}
		report.HeatmapSeries = append(report.HeatmapSeries, heatmapSeries)
	}

	report.AttributeTable = m.atMerger.BuildAttributeTable(report.AttributeTable)

	return report
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

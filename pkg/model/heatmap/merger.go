package heatmap

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/model"
)

type Merger struct {
	mu             sync.Mutex
	attributeTable *model.Table
	series         map[string]*mergedSeries // key is the serialized attribute refs
	sum            bool
}

type mergedSeries struct {
	attributeRefs []int64
	points        []*queryv1.HeatmapPoint
}

// NewMerger creates a new heatmap merger. If sum is set, points
// with matching timestamps, profileIDs, and spanIDs have their values summed.
func NewMerger(sum bool) *Merger {
	return &Merger{
		attributeTable: model.NewTable(),
		series:         make(map[string]*mergedSeries),
		sum:            sum,
	}
}

// MergeHeatmap adds a heatmap report to the merger
func (m *Merger) MergeHeatmap(report *queryv1.HeatmapReport) {
	if report == nil || len(report.HeatmapSeries) == 0 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Build a mapping from old attribute refs to new attribute refs
	refMap := m.remapAttributeTable(report.AttributeTable)

	for _, s := range report.HeatmapSeries {
		// Remap the series attribute refs
		remappedSeriesRefs := m.remapRefs(s.AttributeRefs, refMap)
		key := seriesKey(remappedSeriesRefs)

		existing, ok := m.series[key]
		if !ok {
			m.series[key] = &mergedSeries{
				attributeRefs: remappedSeriesRefs,
				points:        make([]*queryv1.HeatmapPoint, 0, len(s.Points)),
			}
			existing = m.series[key]
		} else {
			// Ensure slice is big enough to hold all the points
			existing.points = slices.Grow(existing.points, len(s.Points))
		}

		// Remap and add points
		for _, p := range s.Points {
			remappedPoint := &queryv1.HeatmapPoint{
				Timestamp:     p.Timestamp,
				ProfileId:     m.remapProfileId(p.ProfileId, refMap),
				SpanId:        p.SpanId,
				Value:         p.Value,
				AttributeRefs: m.remapRefs(p.AttributeRefs, refMap),
			}
			existing.points = append(existing.points, remappedPoint)
		}
	}
}

// remapAttributeTable adds entries from the input attribute table to the merger's
// attribute table and returns a mapping from old refs to new refs
func (m *Merger) remapAttributeTable(table *queryv1.AttributeTable) map[int64]int64 {
	// Keys and Values must have the same length - this is a data corruption bug
	if len(table.Keys) != len(table.Values) {
		panic(fmt.Sprintf("attribute table corruption: Keys length (%d) != Values length (%d)", len(table.Keys), len(table.Values)))
	}

	// only build the refMap if there we are not the first report
	var refMap map[int64]int64
	if len(table.Keys) > 0 {
		refMap = make(map[int64]int64, len(table.Keys))
	}
	for i := range table.Keys {
		oldRef := int64(i)
		key := table.Keys[i]
		value := table.Values[i]
		newRef := m.attributeTable.AddKeyValue(key, value)
		if refMap != nil {
			refMap[oldRef] = newRef
		}
	}
	return refMap
}

// remapRefs remaps a slice of attribute refs using the provided mapping
func (m *Merger) remapRefs(refs []int64, refMap map[int64]int64) []int64 {
	// if we are the first report, we don't have a refMap yet
	if refMap == nil {
		return refs
	}

	for i, ref := range refs {
		if newRef, ok := refMap[ref]; ok {
			refs[i] = newRef
		} else {
			panic(fmt.Sprintf("attribute ref %d not found in attribute table", ref))
		}
	}
	return refs
}

// remapProfileId remaps a profile ID (which is a reference into the attribute table)
func (m *Merger) remapProfileId(profileId int64, refMap map[int64]int64) int64 {
	// if we are the first report, we don't have a refMap yet
	if refMap == nil {
		return profileId
	}
	if newRef, ok := refMap[profileId]; ok {
		return newRef
	}
	panic(fmt.Sprintf("profile ID ref %d not found in attribute table", profileId))
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
		series := &queryv1.HeatmapSeries{
			AttributeRefs: ms.attributeRefs,
			Points:        ms.points[:m.mergePoints(ms.points)],
		}
		report.HeatmapSeries = append(report.HeatmapSeries, series)
	}

	report.AttributeTable = m.attributeTable.Build(report.AttributeTable)

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

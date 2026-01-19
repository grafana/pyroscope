package attributetable

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func TestSeriesMerger_RemapsAttributeRefs(t *testing.T) {
	merger := NewSeriesMerger(true)

	// Source A: AttributeTable with indices 0="pod"/"a", 1="version"/"1.0"
	table1 := &queryv1.AttributeTable{
		Keys:   []string{"pod", "version"},
		Values: []string{"a", "1.0"},
	}
	series1 := []*queryv1.Series{{
		Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "api"}},
		Points: []*queryv1.Point{{
			Timestamp: 1000,
			Value:     100,
			Exemplars: []*queryv1.Exemplar{{
				ProfileId:     "prof-1",
				Value:         100,
				Timestamp:     1000,
				AttributeRefs: []int64{0, 1}, // References: pod=a, version=1.0
			}},
		}},
	}}

	// Source B: AttributeTable with SAME indices but DIFFERENT values: 0="pod"/"b", 1="version"/"1.0"
	table2 := &queryv1.AttributeTable{
		Keys:   []string{"pod", "version"},
		Values: []string{"b", "1.0"},
	}
	series2 := []*queryv1.Series{{
		Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "api"}},
		Points: []*queryv1.Point{{
			Timestamp: 1000,
			Value:     200,
			Exemplars: []*queryv1.Exemplar{{
				ProfileId:     "prof-2",
				Value:         200,
				Timestamp:     1000,
				AttributeRefs: []int64{0, 1}, // References: pod=b, version=1.0 (same indices, different pod!)
			}},
		}},
	}}

	// Merge both sources
	merger.MergeWithAttributeTable(series1, table1)
	merger.MergeWithAttributeTable(series2, table2)

	// Get merged series
	result := merger.TimeSeries()
	require.Len(t, result, 1, "Should have 1 merged series")

	series := result[0]
	require.Len(t, series.Points, 1, "Should have 1 point (same timestamp)")

	// Points with same timestamp should be merged (sum)
	assert.Equal(t, float64(300), series.Points[0].Value, "Values should be summed: 100 + 200")

	// Both exemplars should be present
	require.Len(t, series.Points[0].Exemplars, 2, "Should have 2 exemplars")

	// Build the unified AttributeTable
	unifiedTable := merger.AttributeTable().Build(nil)

	// Verify the exemplars can be decoded correctly
	exemplarsByID := make(map[string]*queryv1.Exemplar)
	for _, ex := range series.Points[0].Exemplars {
		exemplarsByID[ex.ProfileId] = ex
	}

	// Check prof-1 (should have pod=a, version=1.0)
	prof1 := exemplarsByID["prof-1"]
	require.NotNil(t, prof1)
	require.Len(t, prof1.AttributeRefs, 2)
	labels1 := make(map[string]string)
	for _, ref := range prof1.AttributeRefs {
		labels1[unifiedTable.Keys[ref]] = unifiedTable.Values[ref]
	}
	assert.Equal(t, "a", labels1["pod"])
	assert.Equal(t, "1.0", labels1["version"])

	// Check prof-2 (should have pod=b, version=1.0)
	prof2 := exemplarsByID["prof-2"]
	require.NotNil(t, prof2)
	require.Len(t, prof2.AttributeRefs, 2)
	labels2 := make(map[string]string)
	for _, ref := range prof2.AttributeRefs {
		labels2[unifiedTable.Keys[ref]] = unifiedTable.Values[ref]
	}
	assert.Equal(t, "b", labels2["pod"])
	assert.Equal(t, "1.0", labels2["version"])

	// Verify string interning: version=1.0 should be deduplicated
	// The unified table should have 3 entries: pod=a, version=1.0, pod=b
	assert.Len(t, unifiedTable.Keys, 3, "Should have 3 unique key-value pairs")
	assert.Len(t, unifiedTable.Values, 3, "Should have 3 unique key-value pairs")
}

func TestSeriesMerger_StringInterning(t *testing.T) {
	merger := NewSeriesMerger(true)

	// Merge 3 sources with the same labels repeated
	for i := 0; i < 3; i++ {
		table := &queryv1.AttributeTable{
			Keys:   []string{"pod", "env"},
			Values: []string{"pod-1", "prod"},
		}
		series := []*queryv1.Series{{
			Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "api"}},
			Points: []*queryv1.Point{{
				Timestamp: 1000,
				Value:     100,
				Exemplars: []*queryv1.Exemplar{{
					ProfileId:     "prof-1",
					Value:         100,
					Timestamp:     1000,
					AttributeRefs: []int64{0, 1},
				}},
			}},
		}}
		merger.MergeWithAttributeTable(series, table)
	}

	// Build the unified AttributeTable
	unifiedTable := merger.AttributeTable().Build(nil)

	// Despite merging 3 sources with the same labels, the unified AttributeTable
	// should have only 2 unique entries (pod=pod-1, env=prod) thanks to string interning
	assert.Len(t, unifiedTable.Keys, 2, "Should have only 2 unique key-value pairs after string interning")
	assert.Len(t, unifiedTable.Values, 2, "Should have only 2 unique key-value pairs after string interning")

	// Verify the contents
	attrMap := make(map[string]string)
	for i := range unifiedTable.Keys {
		attrMap[unifiedTable.Keys[i]] = unifiedTable.Values[i]
	}
	assert.Equal(t, "pod-1", attrMap["pod"])
	assert.Equal(t, "prod", attrMap["env"])
}

func TestSeriesMerger_MergeExemplars(t *testing.T) {
	merger := NewSeriesMerger(true)

	table := &queryv1.AttributeTable{
		Keys:   []string{"pod"},
		Values: []string{"pod-1"},
	}

	// Add same profile ID twice with different values
	series1 := []*queryv1.Series{{
		Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "api"}},
		Points: []*queryv1.Point{{
			Timestamp: 1000,
			Value:     100,
			Exemplars: []*queryv1.Exemplar{{
				ProfileId:     "prof-1",
				Value:         100,
				Timestamp:     1000,
				AttributeRefs: []int64{0},
			}},
		}},
	}}

	series2 := []*queryv1.Series{{
		Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "api"}},
		Points: []*queryv1.Point{{
			Timestamp: 1000,
			Value:     200,
			Exemplars: []*queryv1.Exemplar{{
				ProfileId:     "prof-1",
				Value:         300, // Higher value
				Timestamp:     1000,
				AttributeRefs: []int64{0},
			}},
		}},
	}}

	merger.MergeWithAttributeTable(series1, table)
	merger.MergeWithAttributeTable(series2, table)

	result := merger.TimeSeries()
	require.Len(t, result, 1)
	require.Len(t, result[0].Points, 1)
	require.Len(t, result[0].Points[0].Exemplars, 1, "Should deduplicate by profile ID")

	// Should keep the exemplar with higher value (300)
	exemplar := result[0].Points[0].Exemplars[0]
	assert.Equal(t, "prof-1", exemplar.ProfileId)
	assert.Equal(t, uint64(300), exemplar.Value, "Should keep the exemplar with higher value")
}

func TestSeriesMerger_ExpandToFullLabels(t *testing.T) {
	merger := NewSeriesMerger(false)

	table := &queryv1.AttributeTable{
		Keys:   []string{"pod", "version"},
		Values: []string{"pod-1", "v1.0"},
	}

	series := []*queryv1.Series{{
		Labels: []*typesv1.LabelPair{{Name: "service_name", Value: "api"}},
		Points: []*queryv1.Point{{
			Timestamp: 1000,
			Value:     100,
			Exemplars: []*queryv1.Exemplar{{
				ProfileId:     "prof-1",
				Value:         100,
				Timestamp:     1000,
				AttributeRefs: []int64{0, 1}, // pod=pod-1, version=v1.0
			}},
		}},
	}}

	merger.MergeWithAttributeTable(series, table)

	// Convert to types.v1.Series
	typesV1Series := merger.ExpandToFullLabels()
	require.Len(t, typesV1Series, 1)
	require.Len(t, typesV1Series[0].Points, 1)
	require.Len(t, typesV1Series[0].Points[0].Exemplars, 1)

	// Verify labels are expanded
	exemplar := typesV1Series[0].Points[0].Exemplars[0]
	require.Len(t, exemplar.Labels, 2)

	labelMap := make(map[string]string)
	for _, lbl := range exemplar.Labels {
		labelMap[lbl.Name] = lbl.Value
	}

	assert.Equal(t, "pod-1", labelMap["pod"])
	assert.Equal(t, "v1.0", labelMap["version"])
}

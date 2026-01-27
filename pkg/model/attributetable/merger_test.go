package attributetable

import (
	"testing"

	"github.com/stretchr/testify/require"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

// Simple test point type
type testPoint struct {
	Timestamp int64
	Value     int64
}

func TestMerger_RemapAttributeTable(t *testing.T) {
	merger := NewMerger[testPoint, string]()

	table := &queryv1.AttributeTable{
		Keys:   []string{"foo", "bar"},
		Values: []string{"val1", "val2"},
	}

	refMap := merger.RemapAttributeTable(table)

	require.NotNil(t, refMap)
	require.Len(t, refMap, 2)
	require.Equal(t, int64(0), refMap[0])
	require.Equal(t, int64(1), refMap[1])
}

func TestMerger_RemapAttributeTable_Multiple(t *testing.T) {
	merger := NewMerger[testPoint, string]()

	// First table
	table1 := &queryv1.AttributeTable{
		Keys:   []string{"foo"},
		Values: []string{"val1"},
	}
	refMap1 := merger.RemapAttributeTable(table1)
	require.Equal(t, int64(0), refMap1[0])

	// Second table with same key
	table2 := &queryv1.AttributeTable{
		Keys:   []string{"foo"},
		Values: []string{"val1"},
	}
	refMap2 := merger.RemapAttributeTable(table2)
	require.Equal(t, int64(0), refMap2[0]) // Should map to same ref

	// Third table with different key
	table3 := &queryv1.AttributeTable{
		Keys:   []string{"bar"},
		Values: []string{"val2"},
	}
	refMap3 := merger.RemapAttributeTable(table3)
	require.Equal(t, int64(1), refMap3[0]) // Should get new ref
}

func TestMerger_RemapRefs(t *testing.T) {
	merger := NewMerger[testPoint, string]()

	table := &queryv1.AttributeTable{
		Keys:   []string{"foo", "bar", "baz"},
		Values: []string{"val1", "val2", "val3"},
	}
	refMap := merger.RemapAttributeTable(table)

	refs := []int64{0, 2}
	remapped := merger.RemapRefs(refs, refMap)

	require.Equal(t, []int64{0, 2}, remapped)
}

func TestMerger_RemapRefs_NilRefMap(t *testing.T) {
	merger := NewMerger[testPoint, string]()

	refs := []int64{0, 1, 2}
	remapped := merger.RemapRefs(refs, nil)

	require.Equal(t, refs, remapped)
}

func TestMerger_GetOrCreateSeries(t *testing.T) {
	merger := NewMerger[testPoint, string]()

	// Create first series
	series1 := merger.GetOrCreateSeries("key1", []int64{0, 1})
	require.NotNil(t, series1)
	require.Equal(t, []int64{0, 1}, series1.AttributeRefs)
	require.Empty(t, series1.Points)

	// Get existing series
	series1Again := merger.GetOrCreateSeries("key1", []int64{0, 1})
	require.Same(t, series1, series1Again)

	// Create second series
	series2 := merger.GetOrCreateSeries("key2", []int64{2, 3})
	require.NotNil(t, series2)
	require.NotSame(t, series1, series2)
}

func TestMerger_IsEmpty(t *testing.T) {
	merger := NewMerger[testPoint, string]()

	require.True(t, merger.IsEmpty())

	merger.GetOrCreateSeries("key1", []int64{0, 1})
	require.False(t, merger.IsEmpty())
}

func TestMerger_BuildAttributeTable(t *testing.T) {
	merger := NewMerger[testPoint, string]()

	table := &queryv1.AttributeTable{
		Keys:   []string{"foo", "bar"},
		Values: []string{"val1", "val2"},
	}
	merger.RemapAttributeTable(table)

	result := merger.BuildAttributeTable(nil)

	require.NotNil(t, result)
	require.Len(t, result.Keys, 2)
	require.Len(t, result.Values, 2)
	require.Equal(t, "foo", result.Keys[0])
	require.Equal(t, "val1", result.Values[0])
	require.Equal(t, "bar", result.Keys[1])
	require.Equal(t, "val2", result.Values[1])
}

func TestMerger_TableAccess(t *testing.T) {
	merger := NewMerger[testPoint, string]()

	table := merger.Table()
	require.NotNil(t, table)
}

func TestMerger_SeriesAccess(t *testing.T) {
	merger := NewMerger[testPoint, string]()

	merger.GetOrCreateSeries("key1", []int64{0, 1})
	merger.GetOrCreateSeries("key2", []int64{2, 3})

	series := merger.Series()
	require.Len(t, series, 2)
	require.Contains(t, series, "key1")
	require.Contains(t, series, "key2")
}

func TestMerger_ThreadSafety(t *testing.T) {
	merger := NewMerger[testPoint, string]()

	// Test that Lock/Unlock work
	merger.Lock()
	merger.GetOrCreateSeries("key1", []int64{0, 1})
	merger.Unlock()

	require.False(t, merger.IsEmpty())
}

func TestMerger_AttributeTableCorruption(t *testing.T) {
	merger := NewMerger[testPoint, string]()

	table := &queryv1.AttributeTable{
		Keys:   []string{"foo", "bar"},
		Values: []string{"val1"}, // Mismatched length
	}

	require.Panics(t, func() {
		merger.RemapAttributeTable(table)
	})
}

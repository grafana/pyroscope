package attributetable

import (
	"testing"

	"github.com/stretchr/testify/require"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

func TestMerger_Merge_SingleTable(t *testing.T) {
	merger := NewMerger()

	table := &queryv1.AttributeTable{
		Keys:   []string{"foo", "bar"},
		Values: []string{"val1", "val2"},
	}

	var remappedRefs []int64
	merger.Merge(table, func(r *Remapper) {
		// First merge should have no remapping
		remappedRefs = r.Refs([]int64{0, 1})
	})

	require.Equal(t, []int64{0, 1}, remappedRefs)
}

func TestMerger_Merge_MultipleTables(t *testing.T) {
	merger := NewMerger()

	// First table
	table1 := &queryv1.AttributeTable{
		Keys:   []string{"foo"},
		Values: []string{"val1"},
	}
	merger.Merge(table1, func(r *Remapper) {
		refs := r.Refs([]int64{0})
		require.Equal(t, []int64{0}, refs)
	})

	// Second table with same key-value pair
	table2 := &queryv1.AttributeTable{
		Keys:   []string{"foo"},
		Values: []string{"val1"},
	}
	merger.Merge(table2, func(r *Remapper) {
		// Should map to the same ref
		refs := r.Refs([]int64{0})
		require.Equal(t, []int64{0}, refs)
	})

	// Third table with different key-value pair
	table3 := &queryv1.AttributeTable{
		Keys:   []string{"bar"},
		Values: []string{"val2"},
	}
	merger.Merge(table3, func(r *Remapper) {
		// Should get a new ref
		refs := r.Refs([]int64{0})
		require.Equal(t, []int64{1}, refs)
	})

	// Verify final table has 2 entries
	result := merger.BuildAttributeTable(nil)
	require.Len(t, result.Keys, 2)
	require.Len(t, result.Values, 2)
}

func TestMerger_Merge_ComplexRemapping(t *testing.T) {
	merger := NewMerger()

	// First table with multiple entries
	table1 := &queryv1.AttributeTable{
		Keys:   []string{"foo", "bar", "baz"},
		Values: []string{"val1", "val2", "val3"},
	}
	merger.Merge(table1, func(r *Remapper) {
		refs := r.Refs([]int64{0, 1, 2})
		require.Equal(t, []int64{0, 1, 2}, refs)
	})

	// Second table with overlapping entries in different order
	table2 := &queryv1.AttributeTable{
		Keys:   []string{"baz", "foo", "new"},
		Values: []string{"val3", "val1", "val4"},
	}
	merger.Merge(table2, func(r *Remapper) {
		// table2[0] = "baz:val3" should map to table1[2]
		// table2[1] = "foo:val1" should map to table1[0]
		// table2[2] = "new:val4" should get a new ref (3)
		refs := r.Refs([]int64{0, 1, 2})
		require.Equal(t, []int64{2, 0, 3}, refs)
	})

	// Verify final table has 4 entries
	result := merger.BuildAttributeTable(nil)
	require.Len(t, result.Keys, 4)
	require.Len(t, result.Values, 4)
	require.Equal(t, "foo", result.Keys[0])
	require.Equal(t, "val1", result.Values[0])
	require.Equal(t, "bar", result.Keys[1])
	require.Equal(t, "val2", result.Values[1])
	require.Equal(t, "baz", result.Keys[2])
	require.Equal(t, "val3", result.Values[2])
	require.Equal(t, "new", result.Keys[3])
	require.Equal(t, "val4", result.Values[3])
}

func TestMerger_Merge_EmptyTable(t *testing.T) {
	merger := NewMerger()

	table := &queryv1.AttributeTable{
		Keys:   []string{},
		Values: []string{},
	}

	merger.Merge(table, func(r *Remapper) {
		refs := r.Refs([]int64{})
		require.Empty(t, refs)
	})

	result := merger.BuildAttributeTable(nil)
	require.Empty(t, result.Keys)
	require.Empty(t, result.Values)
}

func TestMerger_Merge_AttributeTableCorruption(t *testing.T) {
	merger := NewMerger()

	table := &queryv1.AttributeTable{
		Keys:   []string{"foo", "bar"},
		Values: []string{"val1"}, // Mismatched length
	}

	require.Panics(t, func() {
		merger.Merge(table, func(r *Remapper) {})
	})
}

func TestRemapper_Ref_SingleValue(t *testing.T) {
	merger := NewMerger()

	table := &queryv1.AttributeTable{
		Keys:   []string{"foo", "bar", "baz"},
		Values: []string{"val1", "val2", "val3"},
	}

	merger.Merge(table, func(r *Remapper) {
		require.Equal(t, int64(0), r.Ref(0))
		require.Equal(t, int64(1), r.Ref(1))
		require.Equal(t, int64(2), r.Ref(2))
	})

	// Second merge with overlapping data
	table2 := &queryv1.AttributeTable{
		Keys:   []string{"bar"},
		Values: []string{"val2"},
	}
	merger.Merge(table2, func(r *Remapper) {
		// "bar:val2" should map to ref 1 from the first table
		require.Equal(t, int64(1), r.Ref(0))
	})
}

func TestRemapper_Ref_Panic_UnknownRef(t *testing.T) {
	merger := NewMerger()

	table := &queryv1.AttributeTable{
		Keys:   []string{"foo"},
		Values: []string{"val1"},
	}

	// Second merge - should have a remapper with mapping
	table2 := &queryv1.AttributeTable{
		Keys:   []string{"bar"},
		Values: []string{"val2"},
	}

	merger.Merge(table, func(r *Remapper) {})

	require.Panics(t, func() {
		merger.Merge(table2, func(r *Remapper) {
			// Try to remap a ref that doesn't exist in table2
			r.Ref(99)
		})
	})
}

func TestMerger_BuildAttributeTable_ReuseSlices(t *testing.T) {
	merger := NewMerger()

	table := &queryv1.AttributeTable{
		Keys:   []string{"foo"},
		Values: []string{"val1"},
	}
	merger.Merge(table, func(r *Remapper) {})

	// Build with pre-allocated slices
	result := &queryv1.AttributeTable{
		Keys:   make([]string, 0, 10),
		Values: make([]string, 0, 10),
	}
	result = merger.BuildAttributeTable(result)

	// Verify capacity is reused
	require.Equal(t, 10, cap(result.Keys))
	require.Equal(t, 10, cap(result.Values))
	require.Len(t, result.Keys, 1)
	require.Len(t, result.Values, 1)
}

func TestMerger_Concurrency(t *testing.T) {
	merger := NewMerger()

	// Add some initial data
	table1 := &queryv1.AttributeTable{
		Keys:   []string{"foo"},
		Values: []string{"val1"},
	}
	merger.Merge(table1, func(r *Remapper) {})

	// The merger should be safe to use from multiple goroutines
	// as it uses a mutex internally
	done := make(chan bool)
	go func() {
		table2 := &queryv1.AttributeTable{
			Keys:   []string{"bar"},
			Values: []string{"val2"},
		}
		merger.Merge(table2, func(r *Remapper) {})
		done <- true
	}()

	go func() {
		merger.BuildAttributeTable(nil)
		done <- true
	}()

	<-done
	<-done

	result := merger.BuildAttributeTable(nil)
	require.Len(t, result.Keys, 2)
}

package symbolref_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/v2/pkg/model/symbolref"
)

// TestUnresolvedBinariesGrouping verifies UnresolvedBinaries groups
// unresolved refs by build ID with sorted, deduplicated addresses and the
// correct BinaryName, even when addresses were interned out of order or
// repeated.
func TestUnresolvedBinariesGrouping(t *testing.T) {
	table := symbolref.NewTable()
	table.InternUnresolved("b1", "bin-one", 0x300)
	table.InternUnresolved("b1", "bin-one", 0x100)
	table.InternUnresolved("b1", "bin-one", 0x300)
	table.InternUnresolved("b2", "bin-two", 0x50)

	rb := table.ResultBuilder()
	built := new(queryv1.SymbolRefTable)
	rb.Build(built)

	binaries, err := symbolref.UnresolvedBinaries(built)
	require.NoError(t, err)
	require.Len(t, binaries, 2)

	byBuildID := make(map[string]symbolref.UnresolvedBinary, len(binaries))
	for _, u := range binaries {
		byBuildID[u.BuildID] = u
	}

	b1, ok := byBuildID["b1"]
	require.True(t, ok)
	require.Equal(t, []uint64{0x100, 0x300}, b1.Addresses)
	require.Equal(t, "bin-one", b1.BinaryName)

	b2, ok := byBuildID["b2"]
	require.True(t, ok)
	require.Equal(t, []uint64{0x50}, b2.Addresses)
	require.Equal(t, "bin-two", b2.BinaryName)
}

// TestUnresolvedBinariesGroupingUnsorted verifies UnresolvedBinaries still
// groups correctly when its input is not already sorted by
// (buildID, address), as a table from a different producer might be.
func TestUnresolvedBinariesGroupingUnsorted(t *testing.T) {
	pb := &queryv1.SymbolRefTable{
		BuildIds:          []string{"aaa", "bbb"},
		BinaryNames:       []string{"bin-a", "bin-b"},
		UnresolvedBuildId: []uint32{1, 0, 1, 0},
		UnresolvedAddress: []uint64{0x20, 0x10, 0x10, 0x30},
	}

	binaries, err := symbolref.UnresolvedBinaries(pb)
	require.NoError(t, err)
	require.Len(t, binaries, 2)

	byBuildID := make(map[string]symbolref.UnresolvedBinary, len(binaries))
	for _, u := range binaries {
		byBuildID[u.BuildID] = u
	}

	require.Equal(t, []uint64{0x10, 0x30}, byBuildID["aaa"].Addresses)
	require.Equal(t, []uint64{0x10, 0x20}, byBuildID["bbb"].Addresses)
}

// TestUnresolvedBinariesEmpty verifies UnresolvedBinaries returns an empty
// slice for a table with no unresolved entries.
func TestUnresolvedBinariesEmpty(t *testing.T) {
	for name, pb := range map[string]*queryv1.SymbolRefTable{
		"nil":   nil,
		"empty": new(queryv1.SymbolRefTable),
	} {
		t.Run(name, func(t *testing.T) {
			binaries, err := symbolref.UnresolvedBinaries(pb)
			require.NoError(t, err)
			require.Empty(t, binaries)
		})
	}
}

// TestUnresolvedBinariesRejectsMalformed verifies UnresolvedBinaries rejects
// a structurally malformed table instead of grouping garbage.
func TestUnresolvedBinariesRejectsMalformed(t *testing.T) {
	t.Run("mismatched binary_names length", func(t *testing.T) {
		_, err := symbolref.UnresolvedBinaries(&queryv1.SymbolRefTable{
			BuildIds:    []string{"aaa"},
			BinaryNames: []string{"bin-a", "extra"},
		})
		require.Error(t, err)
	})

	t.Run("build ID index out of range", func(t *testing.T) {
		_, err := symbolref.UnresolvedBinaries(&queryv1.SymbolRefTable{
			BuildIds:          []string{"aaa"},
			BinaryNames:       []string{"bin-a"},
			UnresolvedBuildId: []uint32{1},
			UnresolvedAddress: []uint64{0x10},
		})
		require.Error(t, err)
	})

	t.Run("mismatched unresolved lengths", func(t *testing.T) {
		_, err := symbolref.UnresolvedBinaries(&queryv1.SymbolRefTable{
			BuildIds:          []string{"aaa"},
			BinaryNames:       []string{"bin-a"},
			UnresolvedBuildId: []uint32{0, 0},
			UnresolvedAddress: []uint64{0x10},
		})
		require.Error(t, err)
	})
}

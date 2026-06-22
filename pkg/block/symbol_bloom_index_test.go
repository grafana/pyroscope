package block

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSymbolBloomIndexWriter_RoundTrip(t *testing.T) {
	t.Parallel()

	w := NewSymbolBloomIndexWriter(0.01)
	w.Add(SymbolBloomIndexEntry{
		ServiceName:  "svc-a",
		DatasetIndex: 3,
		MinTime:      10,
		MaxTime:      20,
		Symbols: []string{
			"runtime.mallocgc",
			"github.com/grafana/pyroscope/pkg/foo.Bar",
			"runtime.mallocgc",
		},
	})
	w.Add(SymbolBloomIndexEntry{
		ServiceName:  "svc-b",
		DatasetIndex: 7,
		MinTime:      15,
		MaxTime:      25,
		Symbols:      []string{"main.main"},
	})

	var buf bytes.Buffer
	n, err := w.WriteTo(&buf)
	require.NoError(t, err)
	require.Positive(t, n)
	require.True(t, w.Empty())

	rows, err := ReadSymbolBloomIndex(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	require.Len(t, rows, 2)

	require.Equal(t, "svc-a", rows[0].ServiceName)
	require.Equal(t, uint32(3), rows[0].DatasetIndex)
	require.Equal(t, int64(10), rows[0].MinTime)
	require.Equal(t, int64(20), rows[0].MaxTime)
	require.Equal(t, uint32(2), rows[0].SymbolCountEstimate)

	contains, err := rows[0].MightContain("runtime.mallocgc")
	require.NoError(t, err)
	require.True(t, contains)
	contains, err = rows[0].MightContain("github.com/grafana/pyroscope/pkg/foo.Bar")
	require.NoError(t, err)
	require.True(t, contains)
	contains, err = rows[1].MightContain("runtime.mallocgc")
	require.NoError(t, err)
	require.False(t, contains)
}

func TestSymbolBloomIndexWriter_EmptySymbols(t *testing.T) {
	t.Parallel()

	w := NewSymbolBloomIndexWriter(0.01)
	w.Add(SymbolBloomIndexEntry{ServiceName: "svc"})

	var buf bytes.Buffer
	_, err := w.WriteTo(&buf)
	require.NoError(t, err)

	rows, err := ReadSymbolBloomIndex(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Zero(t, rows[0].BloomBitCount)
	require.Zero(t, rows[0].BloomHashCount)
	require.Empty(t, rows[0].BloomBits)

	contains, err := rows[0].MightContain("anything")
	require.NoError(t, err)
	require.False(t, contains)
}

func TestSymbolBloomIndexRow_MatchesFilters(t *testing.T) {
	t.Parallel()

	w := NewSymbolBloomIndexWriter(0.01)
	w.Add(SymbolBloomIndexEntry{
		ServiceName:  "svc-a",
		DatasetIndex: 1,
		MinTime:      100,
		MaxTime:      200,
		Symbols:      []string{"runtime.mallocgc"},
	})

	var buf bytes.Buffer
	_, err := w.WriteTo(&buf)
	require.NoError(t, err)
	rows, err := ReadSymbolBloomIndex(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	row := rows[0]

	matches, err := row.Matches(SymbolBloomLookupRequest{SymbolNames: []string{"main.main", "runtime.mallocgc"}, MinTime: 150, MaxTime: 160})
	require.NoError(t, err)
	require.True(t, matches)

	matches, err = row.Matches(SymbolBloomLookupRequest{SymbolName: "runtime.mallocgc", MinTime: 201})
	require.NoError(t, err)
	require.False(t, matches)
	matches, err = row.Matches(SymbolBloomLookupRequest{SymbolName: "runtime.mallocgc", MaxTime: 99})
	require.NoError(t, err)
	require.False(t, matches)
	matches, err = row.Matches(SymbolBloomLookupRequest{SymbolName: "main.main"})
	require.NoError(t, err)
	require.False(t, matches)
}

func TestSymbolBloomIndexRow_ValidateRejectsUnknownVersion(t *testing.T) {
	t.Parallel()

	row := SymbolBloomIndexRow{FormatVersion: symbolBloomIndexFormatVersion + 1}
	require.ErrorContains(t, row.Validate(), "unsupported symbol bloom index format version")

	contains, err := row.MightContain("runtime.mallocgc")
	require.ErrorContains(t, err, "unsupported symbol bloom index format version")
	require.False(t, contains)
}

func TestSymbolBloomIndexRow_ValidateRejectsCorruptBitset(t *testing.T) {
	t.Parallel()

	row := SymbolBloomIndexRow{
		BloomBits:      []byte{0xff},
		BloomHashCount: 1,
		BloomBitCount:  16,
		FormatVersion:  symbolBloomIndexFormatVersion,
	}
	require.ErrorContains(t, row.Validate(), "symbol bloom bitset too small")
}

func TestSymbolBloomIndexWriter_WriteToEmpty(t *testing.T) {
	t.Parallel()

	w := NewSymbolBloomIndexWriter(0.01)
	var buf bytes.Buffer
	n, err := w.WriteTo(&buf)
	require.NoError(t, err)
	require.Zero(t, n)
	require.Empty(t, buf.Bytes())
}

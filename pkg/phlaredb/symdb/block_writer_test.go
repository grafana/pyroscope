package symdb

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
)

func Test_Writer_IndexFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), DefaultDirName)
	db := NewSymDB(&Config{
		Dir: dir,
		Stacktraces: StacktracesConfig{
			MaxNodesPerChunk: 7,
		},
		Parquet: ParquetConfig{
			MaxBufferRowCount: 100 << 10,
		},
	})

	sids := make([]uint32, 5)

	w := db.SymbolsWriter(0)
	w.AppendStacktraces(sids, []*schemav1.Stacktrace{
		{LocationIDs: []uint64{3, 2, 1}},
		{LocationIDs: []uint64{2, 1}},
		{LocationIDs: []uint64{4, 3, 2, 1}},
		{LocationIDs: []uint64{3, 1}},
		{LocationIDs: []uint64{5, 2, 1}},
	})
	assert.Equal(t, []uint32{3, 2, 11, 16, 18}, sids)

	w = db.SymbolsWriter(1)
	w.AppendStacktraces(sids, []*schemav1.Stacktrace{
		{LocationIDs: []uint64{3, 2, 1}},
		{LocationIDs: []uint64{2, 1}},
		{LocationIDs: []uint64{4, 3, 2, 1}},
		{LocationIDs: []uint64{3, 1}},
		{LocationIDs: []uint64{5, 2, 1}},
	})
	assert.Equal(t, []uint32{3, 2, 11, 16, 18}, sids)

	require.Len(t, db.partitions, 2)
	require.Len(t, db.partitions[0].stacktraces.chunks, 3)
	require.Len(t, db.partitions[1].stacktraces.chunks, 3)

	require.NoError(t, db.Flush())

	b, err := os.ReadFile(filepath.Join(dir, IndexFileName))
	require.NoError(t, err)

	idx, err := ReadIndexFile(b)
	require.NoError(t, err)
	assert.Len(t, idx.PartitionHeaders[0].StacktraceChunks, 3)
	assert.Len(t, idx.PartitionHeaders[1].StacktraceChunks, 3)

	// t.Log(pretty.Sprint(idx))
	/*
		expected := IndexFile{
			Header: Header{
				Magic:   symdbMagic,
				Version: 2,
			},
			TOC: TOC{
				Entries: []TOCEntry{
					{Offset: 32, Size: 384},
				},
			},
			CRC: 0x6418eaed,
		}

		assert.Equal(t, expected, idx)
	*/
}

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
	require.Len(t, db.partitions[0].stacktraceChunks, 3)
	require.Len(t, db.partitions[1].stacktraceChunks, 3)

	require.NoError(t, db.Flush())

	b, err := os.ReadFile(filepath.Join(dir, IndexFileName))
	require.NoError(t, err)

	idx, err := OpenIndexFile(b)
	require.NoError(t, err)
	assert.Len(t, idx.StacktraceChunkHeaders.Entries, 6)

	// t.Log(pretty.Sprint(idx))
	expected := IndexFile{
		Header: Header{
			Magic:   symdbMagic,
			Version: 1,
		},
		TOC: TOC{
			Entries: []TOCEntry{
				{Offset: 32, Size: 384},
			},
		},
		StacktraceChunkHeaders: StacktraceChunkHeaders{
			Entries: []StacktraceChunkHeader{
				{
					Offset:             0,
					Size:               10,
					Partition:          0x0,
					ChunkIndex:         0x0,
					ChunkEncoding:      0x1,
					Stacktraces:        0x2,
					StacktraceNodes:    0x4,
					StacktraceMaxDepth: 0x0,
					StacktraceMaxNodes: 0x7,
					CRC:                0xecc8a63c,
				},
				{
					Offset:             10,
					Size:               15,
					Partition:          0x0,
					ChunkIndex:         0x1,
					ChunkEncoding:      0x1,
					Stacktraces:        0x1,
					StacktraceNodes:    0x5,
					StacktraceMaxDepth: 0x0,
					StacktraceMaxNodes: 0x7,
					CRC:                0x4871eb09,
				},
				{
					Offset:             25,
					Size:               15,
					Partition:          0x0,
					ChunkIndex:         0x2,
					ChunkEncoding:      0x1,
					Stacktraces:        0x3,
					StacktraceNodes:    0x5,
					StacktraceMaxDepth: 0x0,
					StacktraceMaxNodes: 0x7,
					CRC:                0x5f7f24ab,
				},
				{
					Offset:             40,
					Size:               10,
					Partition:          0x1,
					ChunkIndex:         0x0,
					ChunkEncoding:      0x1,
					Stacktraces:        0x2,
					StacktraceNodes:    0x4,
					StacktraceMaxDepth: 0x0,
					StacktraceMaxNodes: 0x7,
					CRC:                0xecc8a63c,
				},
				{
					Offset:             50,
					Size:               15,
					Partition:          0x1,
					ChunkIndex:         0x1,
					ChunkEncoding:      0x1,
					Stacktraces:        0x1,
					StacktraceNodes:    0x5,
					StacktraceMaxDepth: 0x0,
					StacktraceMaxNodes: 0x7,
					CRC:                0x4871eb09,
				},
				{
					Offset:             65,
					Size:               15,
					Partition:          0x1,
					ChunkIndex:         0x2,
					ChunkEncoding:      0x1,
					Stacktraces:        0x3,
					StacktraceNodes:    0x5,
					StacktraceMaxDepth: 0x0,
					StacktraceMaxNodes: 0x7,
					CRC:                0x5f7f24ab,
				},
			},
		},
		CRC: 0x6418eaed,
	}

	assert.Equal(t, expected, idx)
}

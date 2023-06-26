package symdb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/phlare/pkg/objstore/providers/filesystem"
	schemav1 "github.com/grafana/phlare/pkg/phlaredb/schemas/v1"
)

func Test_Reader_Open(t *testing.T) {
	cfg := &Config{
		Dir: t.TempDir(),
		Stacktraces: StacktracesConfig{
			MaxNodesPerChunk: 7,
		},
	}

	db := NewSymDB(cfg)
	w := db.MappingWriter(1)
	a := w.StacktraceAppender()
	sids := make([]uint32, 5)
	a.AppendStacktrace(sids, []*schemav1.Stacktrace{
		{LocationIDs: []uint64{3, 2, 1}},
		{LocationIDs: []uint64{2, 1}},
		{LocationIDs: []uint64{4, 3, 2, 1}},
		{LocationIDs: []uint64{3, 1}},
		{LocationIDs: []uint64{5, 2, 1}},
	})
	require.Equal(t, []uint32{3, 2, 11, 16, 18}, sids)
	a.Release()
	require.NoError(t, db.Flush())

	b, err := filesystem.NewBucket(cfg.Dir)
	require.NoError(t, err)
	x, err := Open(context.Background(), b)
	require.NoError(t, err)
	mr, ok := x.MappingReader(1)
	require.True(t, ok)

	dst := new(mockStacktraceInserter)
	dst.On("InsertStacktrace", uint32(2), []int32{2, 1})
	dst.On("InsertStacktrace", uint32(3), []int32{3, 2, 1})
	dst.On("InsertStacktrace", uint32(11), []int32{4, 3, 2, 1})
	dst.On("InsertStacktrace", uint32(16), []int32{3, 1})
	dst.On("InsertStacktrace", uint32(18), []int32{5, 2, 1})

	r := mr.StacktraceResolver()
	err = r.ResolveStacktraces(context.Background(), dst, sids)
	require.NoError(t, err)
	r.Release()
}

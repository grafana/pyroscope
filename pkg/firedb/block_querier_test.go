package firedb

import (
	"context"
	"fmt"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	schemav1 "github.com/grafana/fire/pkg/firedb/schemas/v1"
	"github.com/grafana/fire/pkg/iter"
	"github.com/grafana/fire/pkg/objstore/providers/filesystem"
)

func newTestHead(t testing.TB) *Head {
	dataPath := t.TempDir()
	head, err := NewHead(Config{DataPath: dataPath})
	require.NoError(t, err)
	return head
}

func TestInMemoryReader(t *testing.T) {
	path := t.TempDir()
	st := deduplicatingSlice[string, string, *stringsHelper, *schemav1.StringPersister]{}
	require.NoError(t, st.Init(path))
	rewrites := &rewriter{}
	rgCount := 5
	for i := 0; i < rgCount*maxBufferRowCount; i++ {
		require.NoError(t, st.ingest(context.Background(), []string{fmt.Sprintf("foobar %d", i)}, rewrites))
	}
	numRows, numRg, err := st.Flush()
	require.NoError(t, err)
	require.Equal(t, uint64(rgCount*maxBufferRowCount), numRows)
	require.Equal(t, uint64(rgCount), numRg)
	require.NoError(t, st.Close())
	reader := inMemoryparquetReader[*schemav1.StoredString, *schemav1.StringPersister]{}
	fs, err := filesystem.NewBucket(path)
	require.NoError(t, err)

	require.NoError(t, reader.open(context.Background(), fs))
	it := reader.retrieveRows(context.Background(), iter.NewSliceIterator(lo.Times(int(numRows), func(i int) int64 { return int64(i) })))
	var j int
	for it.Next() {
		require.Equal(t, it.At().Result.String, fmt.Sprintf("foobar %d", j))
		require.Equal(t, it.At().RowNum, int64(j))
		require.Equal(t, it.At().Result.ID, uint64(j))
		j++
	}

	rows := []int64{0, 1000, 2000}
	it = reader.retrieveRows(context.Background(), iter.NewSliceIterator(rows))
	j = 0
	for it.Next() {
		require.Equal(t, it.At().Result.String, fmt.Sprintf("foobar %d", rows[j]))
		require.Equal(t, it.At().RowNum, rows[j])
		require.Equal(t, it.At().Result.ID, uint64(rows[j]))
		j++
	}
}

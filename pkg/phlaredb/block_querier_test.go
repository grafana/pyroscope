package phlaredb

import (
	"context"
	"fmt"
	"testing"

	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/grafana/phlare/pkg/iter"
	"github.com/grafana/phlare/pkg/objstore/providers/filesystem"
	"github.com/grafana/phlare/pkg/phlaredb/block"
	schemav1 "github.com/grafana/phlare/pkg/phlaredb/schemas/v1"
)

func TestInMemoryReader(t *testing.T) {
	path := t.TempDir()
	st := deduplicatingSlice[string, string, *stringsHelper, *schemav1.StringPersister]{}
	require.NoError(t, st.Init(path, &ParquetConfig{
		MaxBufferRowCount: defaultParquetConfig.MaxBufferRowCount / 1024,
		MaxRowGroupBytes:  defaultParquetConfig.MaxRowGroupBytes / 1024,
		MaxBlockBytes:     defaultParquetConfig.MaxBlockBytes,
	}, newHeadMetrics(prometheus.NewRegistry())))
	rewrites := &rewriter{}
	rgCount := 5
	for i := 0; i < rgCount*st.cfg.MaxBufferRowCount; i++ {
		require.NoError(t, st.ingest(context.Background(), []string{fmt.Sprintf("foobar %d", i)}, rewrites))
	}
	numRows, numRg, err := st.Flush(context.Background())
	require.NoError(t, err)
	require.Equal(t, uint64(rgCount*st.cfg.MaxBufferRowCount), numRows)
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

func TestQuerierBlockEviction(t *testing.T) {
	type testCase struct {
		blocks     []string
		expected   []string
		notEvicted bool
	}

	blockToEvict := "01H002D4Z9PKWSS17Q3XY1VEM9"
	testCases := []testCase{
		{
			notEvicted: true,
		},
		{
			blocks:     []string{"01H002D4Z9ES0DHMMSD18H5J5M"},
			expected:   []string{"01H002D4Z9ES0DHMMSD18H5J5M"},
			notEvicted: true,
		},
		{
			blocks:   []string{blockToEvict},
			expected: []string{},
		},
		{
			blocks:   []string{blockToEvict, "01H002D4Z9ES0DHMMSD18H5J5M"},
			expected: []string{"01H002D4Z9ES0DHMMSD18H5J5M"},
		},
		{
			blocks:   []string{"01H002D4Z9ES0DHMMSD18H5J5M", blockToEvict},
			expected: []string{"01H002D4Z9ES0DHMMSD18H5J5M"},
		},
		{
			blocks:   []string{"01H002D4Z9ES0DHMMSD18H5J5M", blockToEvict, "01H003A2QTY5JF30Z441CDQE70"},
			expected: []string{"01H002D4Z9ES0DHMMSD18H5J5M", "01H003A2QTY5JF30Z441CDQE70"},
		},
		{
			blocks:   []string{"01H003A2QTY5JF30Z441CDQE70", blockToEvict, "01H002D4Z9ES0DHMMSD18H5J5M"},
			expected: []string{"01H003A2QTY5JF30Z441CDQE70", "01H002D4Z9ES0DHMMSD18H5J5M"},
		},
	}

	for _, tc := range testCases {
		q := BlockQuerier{queriers: make([]*singleBlockQuerier, len(tc.blocks))}
		for i, b := range tc.blocks {
			q.queriers[i] = &singleBlockQuerier{meta: &block.Meta{ULID: ulid.MustParse(b)}}
		}

		evicted, err := q.evict(ulid.MustParse(blockToEvict))
		require.NoError(t, err)
		require.Equal(t, !tc.notEvicted, evicted)

		actual := make([]string, 0, len(tc.expected))
		for _, b := range q.queriers {
			actual = append(actual, b.meta.ULID.String())
		}

		require.ElementsMatch(t, tc.expected, actual)
	}
}

package phlaredb

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	ingestv1 "github.com/grafana/phlare/api/gen/proto/go/ingester/v1"
	typesv1 "github.com/grafana/phlare/api/gen/proto/go/types/v1"
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
			q.queriers[i] = &singleBlockQuerier{
				meta:    &block.Meta{ULID: ulid.MustParse(b)},
				metrics: newBlocksMetrics(nil),
			}
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

type profileCounter struct {
	iter.Iterator[Profile]
	count int
}

func (p *profileCounter) Next() bool {
	r := p.Iterator.Next()
	if r {
		p.count++
	}

	return r

}

func TestBlockCompatability(t *testing.T) {
	path := "./block/testdata/"
	bucket, err := filesystem.NewBucket(path)
	require.NoError(t, err)

	ctx := context.Background()
	metas, err := NewBlockQuerier(ctx, bucket).BlockMetas(ctx)
	require.NoError(t, err)

	for _, meta := range metas {
		t.Run(fmt.Sprintf("block-v%d-%s", meta.Version, meta.ULID.String()), func(t *testing.T) {

			q := NewSingleBlockQuerierFromMeta(ctx, bucket, meta)
			require.NoError(t, q.Open(ctx))

			profilesTypes, err := q.index.LabelValues("__profile_type__")
			require.NoError(t, err)

			profileCount := 0

			for _, profileType := range profilesTypes {
				t.Log(profileType)
				profileTypeParts := strings.Split(profileType, ":")

				it, err := q.SelectMatchingProfiles(ctx, &ingestv1.SelectProfilesRequest{
					LabelSelector: "{}",
					Start:         0,
					End:           time.Now().UnixMilli(),
					Type: &typesv1.ProfileType{
						Name:       profileTypeParts[0],
						SampleType: profileTypeParts[1],
						SampleUnit: profileTypeParts[2],
						PeriodType: profileTypeParts[3],
						PeriodUnit: profileTypeParts[4],
					},
				})
				require.NoError(t, err)

				pcIt := &profileCounter{Iterator: it}

				// TODO: It would be nice actually comparing the whole profile, but at present the result is not deterministic.
				_, err = q.MergePprof(ctx, pcIt)
				require.NoError(t, err)

				profileCount += pcIt.count
			}

			require.Equal(t, int(meta.Stats.NumProfiles), profileCount)
		})

	}
}

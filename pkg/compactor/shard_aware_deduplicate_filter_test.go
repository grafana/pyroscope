// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/grafana/mimir/blob/main/pkg/compactor/shard_aware_deduplicate_filter_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package compactor

import (
	"context"
	"fmt"
	"testing"

	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/phlaredb/sharding"
	"github.com/grafana/pyroscope/pkg/util/extprom"
)

func ULID(i int) ulid.ULID { return ulid.MustNew(uint64(i), nil) }

type sourcesAndResolution struct {
	sources    []ulid.ULID
	resolution int64
	shardID    string
}

func TestShardAwareDeduplicateFilter_Filter(t *testing.T) {
	testcases := map[string]struct {
		input    map[ulid.ULID]sourcesAndResolution
		expected []ulid.ULID // blocks in the output after duplicates are removed.
	}{
		"3 non compacted blocks in bucket": {
			input: map[ulid.ULID]sourcesAndResolution{
				ULID(1): {sources: []ulid.ULID{ULID(1)}},
				ULID(2): {sources: []ulid.ULID{ULID(2)}},
				ULID(3): {sources: []ulid.ULID{ULID(3)}},
			},
			expected: []ulid.ULID{
				ULID(1),
				ULID(2),
				ULID(3),
			},
		},
		"compacted block without sources in bucket": {
			input: map[ulid.ULID]sourcesAndResolution{
				ULID(6): {sources: []ulid.ULID{ULID(6)}},
				ULID(4): {sources: []ulid.ULID{ULID(1), ULID(3), ULID(2)}},
				ULID(5): {sources: []ulid.ULID{ULID(5)}},
			},
			expected: []ulid.ULID{
				ULID(4),
				ULID(5),
				ULID(6),
			},
		},
		"two compacted blocks with same sources": {
			input: map[ulid.ULID]sourcesAndResolution{
				ULID(3): {sources: []ulid.ULID{ULID(1), ULID(2)}},
				ULID(4): {sources: []ulid.ULID{ULID(1), ULID(2)}},
				ULID(5): {sources: []ulid.ULID{ULID(5)}},
				ULID(6): {sources: []ulid.ULID{ULID(6)}},
			},
			expected: []ulid.ULID{
				// ULID(4) is added after ULID(3), so ULID(4) becomes a "successor" of ULID(3),
				// which makes ULID(3) to be considered a duplicate.
				ULID(4),
				ULID(5),
				ULID(6),
			},
		},
		"two compacted blocks with overlapping sources": {
			input: map[ulid.ULID]sourcesAndResolution{
				ULID(4): {sources: []ulid.ULID{ULID(1), ULID(2)}},
				ULID(6): {sources: []ulid.ULID{ULID(6)}},
				ULID(5): {sources: []ulid.ULID{ULID(1), ULID(3), ULID(2)}},
			},
			expected: []ulid.ULID{
				ULID(5),
				ULID(6),
			},
		},
		"4 non compacted blocks and compacted block of level 2 in bucket": {
			input: map[ulid.ULID]sourcesAndResolution{
				ULID(6): {sources: []ulid.ULID{ULID(6)}},
				ULID(1): {sources: []ulid.ULID{ULID(1)}},
				ULID(2): {sources: []ulid.ULID{ULID(2)}},
				ULID(3): {sources: []ulid.ULID{ULID(3)}},
				ULID(4): {sources: []ulid.ULID{ULID(2), ULID(1), ULID(3)}},
			},
			expected: []ulid.ULID{
				ULID(4),
				ULID(6),
			},
		},
		"3 compacted blocks of level 2 and one compacted block of level 3 in bucket": {
			input: map[ulid.ULID]sourcesAndResolution{
				ULID(10): {sources: []ulid.ULID{ULID(1), ULID(2), ULID(3)}},
				ULID(11): {sources: []ulid.ULID{ULID(6), ULID(4), ULID(5)}},
				ULID(14): {sources: []ulid.ULID{ULID(14)}},
				ULID(1):  {sources: []ulid.ULID{ULID(1)}},
				ULID(13): {sources: []ulid.ULID{ULID(1), ULID(6), ULID(2), ULID(3), ULID(5), ULID(7), ULID(4), ULID(8), ULID(9)}},
				ULID(12): {sources: []ulid.ULID{ULID(7), ULID(9), ULID(8)}},
			},
			expected: []ulid.ULID{
				ULID(14),
				ULID(13),
			},
		},
		"compacted blocks with overlapping sources": {
			input: map[ulid.ULID]sourcesAndResolution{
				ULID(8): {sources: []ulid.ULID{ULID(1), ULID(3), ULID(2), ULID(4)}},
				ULID(1): {sources: []ulid.ULID{ULID(1)}},
				ULID(5): {sources: []ulid.ULID{ULID(1), ULID(2)}},
				ULID(6): {sources: []ulid.ULID{ULID(1), ULID(3), ULID(2), ULID(4)}},
				ULID(7): {sources: []ulid.ULID{ULID(3), ULID(1), ULID(2)}},
			},
			expected: []ulid.ULID{
				ULID(8),
			},
		},
		"compacted blocks of level 3 with overlapping sources of equal length": {
			input: map[ulid.ULID]sourcesAndResolution{
				ULID(10): {sources: []ulid.ULID{ULID(1), ULID(2), ULID(6), ULID(7)}},
				ULID(1):  {sources: []ulid.ULID{ULID(1)}},
				ULID(11): {sources: []ulid.ULID{ULID(6), ULID(8), ULID(1), ULID(2)}},
			},
			expected: []ulid.ULID{
				ULID(10),
				ULID(11),
			},
		},
		"compacted blocks of level 3 with overlapping sources of different length": {
			input: map[ulid.ULID]sourcesAndResolution{
				ULID(10): {sources: []ulid.ULID{ULID(6), ULID(7), ULID(1), ULID(2)}},
				ULID(1):  {sources: []ulid.ULID{ULID(1)}},
				ULID(5):  {sources: []ulid.ULID{ULID(1), ULID(2)}},
				ULID(11): {sources: []ulid.ULID{ULID(2), ULID(3), ULID(1)}},
			},
			expected: []ulid.ULID{
				ULID(10),
				ULID(11),
			},
		},
		"blocks with same sources and different resolutions": {
			input: map[ulid.ULID]sourcesAndResolution{
				ULID(1): {sources: []ulid.ULID{ULID(1)}, resolution: 0},
				ULID(2): {sources: []ulid.ULID{ULID(1)}, resolution: 1000},
				ULID(3): {sources: []ulid.ULID{ULID(1)}, resolution: 10000},
			},
			expected: []ulid.ULID{
				ULID(1),
				ULID(2),
				ULID(3),
			},
		},
		"compacted blocks with overlapping sources and different resolutions": {
			input: map[ulid.ULID]sourcesAndResolution{
				ULID(1): {sources: []ulid.ULID{ULID(1)}, resolution: 0},
				ULID(6): {sources: []ulid.ULID{ULID(6)}, resolution: 10000},
				ULID(4): {sources: []ulid.ULID{ULID(1), ULID(3), ULID(2)}, resolution: 0},
				ULID(5): {sources: []ulid.ULID{ULID(2), ULID(3), ULID(1)}, resolution: 1000},
			},
			expected: []ulid.ULID{
				ULID(4),
				ULID(5),
				ULID(6),
			},
		},
		"compacted blocks of level 3 with overlapping sources of different length and different resolutions": {
			input: map[ulid.ULID]sourcesAndResolution{
				ULID(10): {sources: []ulid.ULID{ULID(7), ULID(5), ULID(1), ULID(2)}, resolution: 0},
				ULID(12): {sources: []ulid.ULID{ULID(6), ULID(7), ULID(1)}, resolution: 10000},
				ULID(1):  {sources: []ulid.ULID{ULID(1)}, resolution: 0},
				ULID(13): {sources: []ulid.ULID{ULID(1)}, resolution: 10000},
				ULID(5):  {sources: []ulid.ULID{ULID(1), ULID(2)}, resolution: 0},
				ULID(11): {sources: []ulid.ULID{ULID(2), ULID(3), ULID(1)}, resolution: 0},
			},
			expected: []ulid.ULID{
				ULID(10),
				ULID(11),
				ULID(12),
			},
		},

		// Blocks with ShardID
		"two blocks merged and split, with single shard": {
			input: map[ulid.ULID]sourcesAndResolution{
				ULID(1): {sources: []ulid.ULID{ULID(1)}},
				ULID(2): {sources: []ulid.ULID{ULID(2)}},
				ULID(3): {sources: []ulid.ULID{ULID(1), ULID(2)}, shardID: "1_of_1"},
			},
			expected: []ulid.ULID{
				ULID(3),
			},
		},

		"block with invalid shardID cannot 'include' its source blocks": {
			input: map[ulid.ULID]sourcesAndResolution{
				ULID(1): {sources: []ulid.ULID{ULID(1)}},
				ULID(2): {sources: []ulid.ULID{ULID(2)}},
				ULID(3): {sources: []ulid.ULID{ULID(1), ULID(2)}, shardID: "invalid"},
				ULID(4): {sources: []ulid.ULID{ULID(1), ULID(2)}, shardID: "0_of_5"},
				ULID(5): {sources: []ulid.ULID{ULID(1), ULID(2)}, shardID: "3_of_2"},
			},
			// No blocks are removed as duplicates.
			expected: []ulid.ULID{
				ULID(1),
				ULID(2),
				ULID(3),
				ULID(4),
				ULID(5),
			},
		},

		"when invalid shard IDs present, no deduplication happens for source blocks": {
			input: map[ulid.ULID]sourcesAndResolution{
				ULID(1): {sources: []ulid.ULID{ULID(1)}},
				ULID(2): {sources: []ulid.ULID{ULID(2)}},
				// invalid
				ULID(3): {sources: []ulid.ULID{ULID(1), ULID(2)}, shardID: "invalid"},
				ULID(4): {sources: []ulid.ULID{ULID(1), ULID(2)}, shardID: "0_of_5"},
				ULID(5): {sources: []ulid.ULID{ULID(1), ULID(2)}, shardID: "3_of_2"},
				// good shards
				ULID(6): {sources: []ulid.ULID{ULID(1), ULID(2)}, shardID: "1_of_2"},
				ULID(7): {sources: []ulid.ULID{ULID(1), ULID(2)}, shardID: "2_of_2"},
			},
			// Presence of invalid shards means that even valid shards are not
			expected: []ulid.ULID{
				ULID(1),
				ULID(2),
				ULID(3),
				ULID(4),
				ULID(5),
				ULID(6),
				ULID(7),
			},
		},

		"two blocks merged and split, with two shards": {
			input: map[ulid.ULID]sourcesAndResolution{
				ULID(1): {sources: []ulid.ULID{ULID(1)}},
				ULID(2): {sources: []ulid.ULID{ULID(2)}},
				ULID(3): {sources: []ulid.ULID{ULID(1), ULID(2)}, shardID: "1_of_2"},
				ULID(4): {sources: []ulid.ULID{ULID(1), ULID(2)}, shardID: "2_of_2"},
			},
			expected: []ulid.ULID{
				ULID(3),
				ULID(4),
			},
		},

		"two blocks merged and split into two, one shard missing": {
			input: map[ulid.ULID]sourcesAndResolution{
				ULID(1): {sources: []ulid.ULID{ULID(1)}},
				ULID(2): {sources: []ulid.ULID{ULID(2)}},
				ULID(3): {sources: []ulid.ULID{ULID(1), ULID(2)}, shardID: "1_of_2"},
			},
			expected: []ulid.ULID{
				ULID(1),
				ULID(2),
				ULID(3),
			},
		},
		"four base blocks merged and split into 2 separate shards": {
			input: map[ulid.ULID]sourcesAndResolution{
				ULID(1): {sources: []ulid.ULID{ULID(1)}},
				ULID(2): {sources: []ulid.ULID{ULID(2)}},
				ULID(3): {sources: []ulid.ULID{ULID(3)}},
				ULID(4): {sources: []ulid.ULID{ULID(4)}},

				// shards of 1+2
				ULID(5): {sources: []ulid.ULID{ULID(1), ULID(2)}, shardID: "1_of_2"},
				ULID(6): {sources: []ulid.ULID{ULID(1), ULID(2)}, shardID: "2_of_2"},

				// shards of 3+4
				ULID(7): {sources: []ulid.ULID{ULID(3), ULID(4)}, shardID: "1_of_2"},
				ULID(8): {sources: []ulid.ULID{ULID(3), ULID(4)}, shardID: "2_of_2"},
			},
			expected: []ulid.ULID{
				ULID(5),
				ULID(6),
				ULID(7),
				ULID(8),
			},
		},

		"four base blocks merged and split into 2 separate shards, plus another level": {
			input: map[ulid.ULID]sourcesAndResolution{
				ULID(1): {sources: []ulid.ULID{ULID(1)}},
				ULID(2): {sources: []ulid.ULID{ULID(2)}},
				ULID(3): {sources: []ulid.ULID{ULID(3)}},
				ULID(4): {sources: []ulid.ULID{ULID(4)}},

				// shards of 1+2
				ULID(5): {sources: []ulid.ULID{ULID(1), ULID(2)}, shardID: "1_of_2"},
				ULID(6): {sources: []ulid.ULID{ULID(1), ULID(2)}, shardID: "2_of_2"},

				// shards of 3+4
				ULID(7): {sources: []ulid.ULID{ULID(3), ULID(4)}, shardID: "1_of_2"},
				ULID(8): {sources: []ulid.ULID{ULID(3), ULID(4)}, shardID: "2_of_2"},

				// Two shards of 1+2+3+4 blocks. These "win".
				ULID(9):  {sources: []ulid.ULID{ULID(1), ULID(2), ULID(3), ULID(4)}, shardID: "1_of_2"},
				ULID(10): {sources: []ulid.ULID{ULID(1), ULID(2), ULID(3), ULID(4)}, shardID: "2_of_2"},
			},
			expected: []ulid.ULID{
				ULID(9),
				ULID(10),
			},
		},

		"four base blocks merged and split into 2 separate shards, plus another level, with various resolutions": {
			input: map[ulid.ULID]sourcesAndResolution{
				ULID(1): {sources: []ulid.ULID{ULID(1)}},
				ULID(2): {sources: []ulid.ULID{ULID(2)}},
				ULID(3): {sources: []ulid.ULID{ULID(3)}, resolution: 100},
				ULID(4): {sources: []ulid.ULID{ULID(4)}, resolution: 100},

				// shards of 1+2
				ULID(5): {sources: []ulid.ULID{ULID(1), ULID(2)}, shardID: "1_of_2"},
				ULID(6): {sources: []ulid.ULID{ULID(1), ULID(2)}, shardID: "2_of_2"},

				// shards of 3+4
				ULID(7): {sources: []ulid.ULID{ULID(3), ULID(4)}, shardID: "1_of_2", resolution: 100},
				ULID(8): {sources: []ulid.ULID{ULID(3), ULID(4)}, shardID: "2_of_2", resolution: 100},
			},
			expected: []ulid.ULID{
				ULID(5),
				ULID(6),
				ULID(7),
				ULID(8),
			},
		},
	}

	for name, tcase := range testcases {
		t.Run(name, func(t *testing.T) {
			f := NewShardAwareDeduplicateFilter()
			m := newTestFetcherMetrics()

			metas := make(map[ulid.ULID]*block.Meta, len(tcase.input))

			inputLen := len(tcase.input)
			for id, metaInfo := range tcase.input {
				metas[id] = &block.Meta{
					ULID: id,
					Compaction: block.BlockMetaCompaction{
						Sources: metaInfo.sources,
					},

					Downsample: block.Downsample{
						Resolution: metaInfo.resolution,
					},
					Labels: map[string]string{
						sharding.CompactorShardIDLabel: metaInfo.shardID,
					},
				}
			}

			expected := make(map[ulid.ULID]*block.Meta, len(tcase.expected))
			for _, id := range tcase.expected {
				m := metas[id]
				require.NotNil(t, m)
				expected[id] = m
			}

			require.NoError(t, f.Filter(context.Background(), metas, m.Synced))
			require.Equal(t, expected, metas)
			require.Equal(t, float64(inputLen-len(tcase.expected)), promtest.ToFloat64(m.Synced.WithLabelValues(duplicateMeta)))

			for _, id := range f.duplicateIDs {
				require.NotNil(t, tcase.input[id])
				require.Nil(t, metas[id])
			}
		})
	}
}

func newTestFetcherMetrics() *block.FetcherMetrics {
	return &block.FetcherMetrics{
		Synced: extprom.NewTxGaugeVec(nil, prometheus.GaugeOpts{}, []string{"state"}),
	}
}

func BenchmarkDeduplicateFilter_Filter(b *testing.B) {
	var (
		reg   prometheus.Registerer
		count uint64
	)

	dedupFilter := NewShardAwareDeduplicateFilter()
	synced := extprom.NewTxGaugeVec(reg, prometheus.GaugeOpts{}, []string{"state"})

	for blocksNum := 10; blocksNum <= 10000; blocksNum *= 10 {
		var cases []map[ulid.ULID]*block.Meta
		// blocksNum number of blocks with all of them unique ULID and unique 100 sources.
		cases = append(cases, make(map[ulid.ULID]*block.Meta, blocksNum))
		for i := 0; i < blocksNum; i++ {

			id := ulid.MustNew(count, nil)
			count++

			cases[0][id] = &block.Meta{
				ULID: id,
			}

			for j := 0; j < 100; j++ {
				cases[0][id].Compaction.Sources = append(cases[0][id].Compaction.Sources, ulid.MustNew(count, nil))
				count++
			}
		}

		// Case for running 3x resolution as they can be run concurrently.
		// blocksNum number of blocks. all of them with unique ULID and unique 100 cases.
		cases = append(cases, make(map[ulid.ULID]*block.Meta, 3*blocksNum))

		for i := 0; i < blocksNum; i++ {
			for _, res := range []int64{0, 5 * 60 * 1000, 60 * 60 * 1000} {

				id := ulid.MustNew(count, nil)
				count++
				cases[1][id] = &block.Meta{
					ULID: id,

					Downsample: block.Downsample{Resolution: res},
				}
				for j := 0; j < 100; j++ {
					cases[1][id].Compaction.Sources = append(cases[1][id].Compaction.Sources, ulid.MustNew(count, nil))
					count++
				}

			}
		}

		b.Run(fmt.Sprintf("Block-%d", blocksNum), func(b *testing.B) {
			for _, tcase := range cases {
				b.ResetTimer()
				b.Run("", func(b *testing.B) {
					for n := 0; n <= b.N; n++ {
						_ = dedupFilter.Filter(context.Background(), tcase, synced)
						require.Equal(b, 0, len(dedupFilter.DuplicateIDs()))
					}
				})
			}
		})
	}
}

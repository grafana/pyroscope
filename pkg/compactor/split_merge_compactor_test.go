// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/grafana/mimir/blob/main/pkg/compactor/split_merge_compactor_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package compactor

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/services"
	"github.com/grafana/dskit/test"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/objstore/client"
	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/phlaredb/sharding"
)

func TestMultitenantCompactor_ShouldSupportSplitAndMergeCompactor(t *testing.T) {
	const (
		userID     = "user-1"
		numSeries  = 100
		blockRange = 2 * time.Hour
	)

	var (
		blockRangeMillis = blockRange.Milliseconds()
		compactionRanges = DurationList{blockRange, 2 * blockRange, 4 * blockRange}
	)

	externalLabels := func(shardID string) map[string]string {
		labels := map[string]string{}

		if shardID != "" {
			labels[sharding.CompactorShardIDLabel] = shardID
		}
		return labels
	}

	tests := map[string]struct {
		numShards int
		setup     func(t *testing.T, bkt objstore.Bucket) []block.Meta
	}{
		"overlapping blocks matching the 1st compaction range should be merged and split": {
			numShards: 2,
			setup: func(t *testing.T, bkt objstore.Bucket) []block.Meta {
				block1 := createDBBlock(t, bkt, userID, blockRangeMillis, 2*blockRangeMillis, numSeries, externalLabels(""))
				block2 := createDBBlock(t, bkt, userID, blockRangeMillis, 2*blockRangeMillis, numSeries, externalLabels(""))

				return []block.Meta{
					{
						MinTime: model.Time(1 * blockRangeMillis),
						MaxTime: model.Time(2 * blockRangeMillis),
						Compaction: block.BlockMetaCompaction{
							Sources: []ulid.ULID{block1, block2},
						},

						Labels: map[string]string{
							sharding.CompactorShardIDLabel: "1_of_2",
						},
					}, {
						MinTime: model.Time(1 * blockRangeMillis),
						MaxTime: model.Time(2 * blockRangeMillis),
						Compaction: block.BlockMetaCompaction{
							Sources: []ulid.ULID{block1, block2},
						},

						Labels: map[string]string{
							sharding.CompactorShardIDLabel: "2_of_2",
						},
					},
				}
			},
		},
		"overlapping blocks matching the beginning of the 1st compaction range should be merged and split": {
			numShards: 2,
			setup: func(t *testing.T, bkt objstore.Bucket) []block.Meta {
				block1 := createDBBlock(t, bkt, userID, 0, (5 * time.Minute).Milliseconds(), numSeries, externalLabels(""))
				block2 := createDBBlock(t, bkt, userID, time.Minute.Milliseconds(), (7 * time.Minute).Milliseconds(), numSeries, externalLabels(""))

				// Add another block as "most recent one" otherwise the previous blocks are not compacted
				// because the most recent blocks must cover the full range to be compacted.
				block3 := createDBBlock(t, bkt, userID, blockRangeMillis, blockRangeMillis+time.Minute.Milliseconds(), numSeries, externalLabels(""))

				return []block.Meta{
					{
						MinTime: 0,
						MaxTime: model.Time((7 * time.Minute).Milliseconds()),
						Compaction: block.BlockMetaCompaction{
							Sources: []ulid.ULID{block1, block2},
						},

						Labels: map[string]string{
							sharding.CompactorShardIDLabel: "1_of_2",
						},
					}, {
						MinTime: 0,
						MaxTime: model.Time((7 * time.Minute).Milliseconds()),
						Compaction: block.BlockMetaCompaction{
							Sources: []ulid.ULID{block1, block2},
						},

						Labels: map[string]string{
							sharding.CompactorShardIDLabel: "2_of_2",
						},
					}, {
						// Not compacted.
						MinTime: model.Time(blockRangeMillis),
						MaxTime: model.Time(blockRangeMillis + time.Minute.Milliseconds()),
						Compaction: block.BlockMetaCompaction{
							Sources: []ulid.ULID{block3},
						},

						Labels: map[string]string{},
					},
				}
			},
		},
		"non-overlapping blocks matching the beginning of the 1st compaction range (without gaps) should be merged and split": {
			numShards: 2,
			setup: func(t *testing.T, bkt objstore.Bucket) []block.Meta {
				block1 := createDBBlock(t, bkt, userID, 0, (5 * time.Minute).Milliseconds(), numSeries, externalLabels(""))
				block2 := createDBBlock(t, bkt, userID, (5 * time.Minute).Milliseconds(), (10 * time.Minute).Milliseconds(), numSeries, externalLabels(""))

				// Add another block as "most recent one" otherwise the previous blocks are not compacted
				// because the most recent blocks must cover the full range to be compacted.
				block3 := createDBBlock(t, bkt, userID, blockRangeMillis, blockRangeMillis+time.Minute.Milliseconds(), numSeries, externalLabels(""))

				return []block.Meta{
					{
						MinTime: 0,
						MaxTime: model.Time((10 * time.Minute).Milliseconds()),
						Compaction: block.BlockMetaCompaction{
							Sources: []ulid.ULID{block1, block2},
						},

						Labels: map[string]string{
							sharding.CompactorShardIDLabel: "1_of_2",
						},
					}, {
						MinTime: 0,
						MaxTime: model.Time((10 * time.Minute).Milliseconds()),
						Compaction: block.BlockMetaCompaction{
							Sources: []ulid.ULID{block1, block2},
						},

						Labels: map[string]string{
							sharding.CompactorShardIDLabel: "2_of_2",
						},
					}, {
						// Not compacted.
						MinTime: model.Time(blockRangeMillis),
						MaxTime: model.Time(blockRangeMillis + time.Minute.Milliseconds()),
						Compaction: block.BlockMetaCompaction{
							Sources: []ulid.ULID{block3},
						},

						Labels: map[string]string{},
					},
				}
			},
		},
		"non-overlapping blocks matching the beginning of the 1st compaction range (with gaps) should be merged and split": {
			numShards: 2,
			setup: func(t *testing.T, bkt objstore.Bucket) []block.Meta {
				block1 := createDBBlock(t, bkt, userID, 0, (5 * time.Minute).Milliseconds(), numSeries, externalLabels(""))
				block2 := createDBBlock(t, bkt, userID, (7 * time.Minute).Milliseconds(), (10 * time.Minute).Milliseconds(), numSeries, externalLabels(""))

				// Add another block as "most recent one" otherwise the previous blocks are not compacted
				// because the most recent blocks must cover the full range to be compacted.
				block3 := createDBBlock(t, bkt, userID, blockRangeMillis, blockRangeMillis+time.Minute.Milliseconds(), numSeries, externalLabels(""))

				return []block.Meta{
					{
						MinTime: 0,
						MaxTime: model.Time((10 * time.Minute).Milliseconds()),
						Compaction: block.BlockMetaCompaction{
							Sources: []ulid.ULID{block1, block2},
						},

						Labels: map[string]string{
							sharding.CompactorShardIDLabel: "1_of_2",
						},
					}, {
						MinTime: 0,
						MaxTime: model.Time((10 * time.Minute).Milliseconds()),
						Compaction: block.BlockMetaCompaction{
							Sources: []ulid.ULID{block1, block2},
						},

						Labels: map[string]string{
							sharding.CompactorShardIDLabel: "2_of_2",
						},
					}, {
						// Not compacted.
						MinTime: model.Time(blockRangeMillis),
						MaxTime: model.Time(blockRangeMillis + time.Minute.Milliseconds()),
						Compaction: block.BlockMetaCompaction{
							Sources: []ulid.ULID{block3},
						},

						Labels: map[string]string{},
					},
				}
			},
		},
		"smaller compaction ranges should take precedence over larger ones, and then re-iterate in subsequent compactions of increasing ranges": {
			numShards: 2,
			setup: func(t *testing.T, bkt objstore.Bucket) []block.Meta {
				// Two split blocks in the 1st compaction range.
				block1a := createDBBlock(t, bkt, userID, 1, blockRangeMillis, numSeries, externalLabels("1_of_2"))
				block1b := createDBBlock(t, bkt, userID, 1, blockRangeMillis, numSeries, externalLabels("2_of_2"))

				// Two non-split overlapping blocks in the 1st compaction range.
				block2 := createDBBlock(t, bkt, userID, blockRangeMillis, 2*blockRangeMillis, numSeries, externalLabels(""))
				block3 := createDBBlock(t, bkt, userID, blockRangeMillis, 2*blockRangeMillis, numSeries, externalLabels(""))

				// Two split adjacent blocks in the 2nd compaction range.
				block4a := createDBBlock(t, bkt, userID, 2*blockRangeMillis, 3*blockRangeMillis, numSeries, externalLabels("1_of_2"))
				block4b := createDBBlock(t, bkt, userID, 2*blockRangeMillis, 3*blockRangeMillis, numSeries, externalLabels("2_of_2"))
				block5a := createDBBlock(t, bkt, userID, 3*blockRangeMillis, 4*blockRangeMillis, numSeries, externalLabels("1_of_2"))
				block5b := createDBBlock(t, bkt, userID, 3*blockRangeMillis, 4*blockRangeMillis, numSeries, externalLabels("2_of_2"))

				// Two non-adjacent non-split blocks in the 1st compaction range.
				block6 := createDBBlock(t, bkt, userID, 4*blockRangeMillis+1, 5*blockRangeMillis, numSeries, externalLabels(""))
				block7 := createDBBlock(t, bkt, userID, 7*blockRangeMillis, 8*blockRangeMillis, numSeries, externalLabels(""))

				return []block.Meta{
					// The two overlapping blocks (block2, block3) have been merged and split in the 1st range,
					// and then compacted with block1 in 2nd range. Finally, they've been compacted with
					// block4 and block5 in the 3rd range compaction (total levels: 4).
					{
						MinTime: 1,
						MaxTime: model.Time(4 * blockRangeMillis),
						Compaction: block.BlockMetaCompaction{
							Sources: []ulid.ULID{block1a, block2, block3, block4a, block5a},
						},

						Labels: map[string]string{
							sharding.CompactorShardIDLabel: "1_of_2",
						},
					},
					{
						MinTime: 1,
						MaxTime: model.Time(4 * blockRangeMillis),
						Compaction: block.BlockMetaCompaction{
							Sources: []ulid.ULID{block1b, block2, block3, block4b, block5b},
						},

						Labels: map[string]string{
							sharding.CompactorShardIDLabel: "2_of_2",
						},
					},
					// The two non-adjacent blocks block6 and block7 are split individually first and then merged
					// together in the 3rd range.
					{
						MinTime: model.Time(4*blockRangeMillis + 1),
						MaxTime: model.Time(8 * blockRangeMillis),
						Compaction: block.BlockMetaCompaction{
							Sources: []ulid.ULID{block6, block7},
						},

						Labels: map[string]string{
							sharding.CompactorShardIDLabel: "1_of_2",
						},
					},
					{
						MinTime: model.Time(4*blockRangeMillis + 1),
						MaxTime: model.Time(8 * blockRangeMillis),
						Compaction: block.BlockMetaCompaction{
							Sources: []ulid.ULID{block6, block7},
						},

						Labels: map[string]string{
							sharding.CompactorShardIDLabel: "2_of_2",
						},
					},
				}
			},
		},
		"overlapping and non-overlapping blocks within the same range should be split and compacted together": {
			numShards: 2,
			setup: func(t *testing.T, bkt objstore.Bucket) []block.Meta {
				// Overlapping.
				block1 := createDBBlock(t, bkt, userID, 0, (5 * time.Minute).Milliseconds(), numSeries, externalLabels(""))
				block2 := createDBBlock(t, bkt, userID, time.Minute.Milliseconds(), (7 * time.Minute).Milliseconds(), numSeries, externalLabels(""))

				// Not overlapping.
				block3 := createDBBlock(t, bkt, userID, time.Hour.Milliseconds(), (2 * time.Hour).Milliseconds(), numSeries, externalLabels(""))

				return []block.Meta{
					{
						MinTime: 0,
						MaxTime: model.Time((2 * time.Hour).Milliseconds()),
						Compaction: block.BlockMetaCompaction{
							Sources: []ulid.ULID{block1, block2, block3},
						},

						Labels: map[string]string{
							sharding.CompactorShardIDLabel: "1_of_2",
						},
					}, {
						MinTime: 0,
						MaxTime: model.Time((2 * time.Hour).Milliseconds()),
						Compaction: block.BlockMetaCompaction{
							Sources: []ulid.ULID{block1, block2, block3},
						},

						Labels: map[string]string{
							sharding.CompactorShardIDLabel: "2_of_2",
						},
					},
				}
			},
		},
		"should correctly handle empty blocks generated in the splitting stage": {
			numShards: 2,
			setup: func(t *testing.T, bkt objstore.Bucket) []block.Meta {
				// Generate a block with only 1 series. This block will be split into 1 split block only,
				// because the source block only has 1 series.
				block1 := createDBBlock(t, bkt, userID, blockRangeMillis, 2*blockRangeMillis, 1, externalLabels(""))

				return []block.Meta{
					{
						MinTime: model.Time(blockRangeMillis), // Because there's only 1 sample with timestamp=maxT-1
						MaxTime: model.Time(2 * blockRangeMillis),
						Compaction: block.BlockMetaCompaction{
							Sources: []ulid.ULID{block1},
						},

						Labels: map[string]string{
							sharding.CompactorShardIDLabel: "1_of_2",
						},
					},
				}
			},
		},
		"splitting should be disabled if configured shards = 0": {
			numShards: 0,
			setup: func(t *testing.T, bkt objstore.Bucket) []block.Meta {
				block1 := createDBBlock(t, bkt, userID, 0, (5 * time.Minute).Milliseconds(), numSeries, externalLabels(""))
				block2 := createDBBlock(t, bkt, userID, (5 * time.Minute).Milliseconds(), (10 * time.Minute).Milliseconds(), numSeries, externalLabels(""))

				// Add another block as "most recent one" otherwise the previous blocks are not compacted
				// because the most recent blocks must cover the full range to be compacted.
				block3 := createDBBlock(t, bkt, userID, blockRangeMillis, blockRangeMillis+time.Minute.Milliseconds(), numSeries, externalLabels(""))

				return []block.Meta{
					// Compacted but not split.
					{
						MinTime: 0,
						MaxTime: model.Time((10 * time.Minute).Milliseconds()),
						Compaction: block.BlockMetaCompaction{
							Sources: []ulid.ULID{block1, block2},
						},

						Labels: map[string]string{},
					}, {
						// Not compacted.
						MinTime: model.Time(blockRangeMillis),
						MaxTime: model.Time(blockRangeMillis + time.Minute.Milliseconds()),
						Compaction: block.BlockMetaCompaction{
							Sources: []ulid.ULID{block3},
						},

						Labels: map[string]string{},
					},
				}
			},
		},
		"splitting should be disabled but already split blocks should be merged correctly (respecting the shard) if configured shards = 0": {
			numShards: 0,
			setup: func(t *testing.T, bkt objstore.Bucket) []block.Meta {
				// Two split blocks in the 1st compaction range.
				block1a := createDBBlock(t, bkt, userID, 1, blockRangeMillis, numSeries, externalLabels("1_of_2"))
				block1b := createDBBlock(t, bkt, userID, 1, blockRangeMillis, numSeries, externalLabels("2_of_2"))

				// Two non-split overlapping blocks in the 1st compaction range.
				block2 := createDBBlock(t, bkt, userID, blockRangeMillis, 2*blockRangeMillis, numSeries, externalLabels(""))
				block3 := createDBBlock(t, bkt, userID, blockRangeMillis, 2*blockRangeMillis, numSeries, externalLabels(""))

				// // Two split adjacent blocks in the 2nd compaction range.
				block4a := createDBBlock(t, bkt, userID, 2*blockRangeMillis, 3*blockRangeMillis, numSeries, externalLabels("1_of_2"))
				block4b := createDBBlock(t, bkt, userID, 2*blockRangeMillis, 3*blockRangeMillis, numSeries, externalLabels("2_of_2"))
				block5a := createDBBlock(t, bkt, userID, 3*blockRangeMillis, 4*blockRangeMillis, numSeries, externalLabels("1_of_2"))
				block5b := createDBBlock(t, bkt, userID, 3*blockRangeMillis, 4*blockRangeMillis, numSeries, externalLabels("2_of_2"))

				// Two non-adjacent non-split blocks in the 1st compaction range.
				block6 := createDBBlock(t, bkt, userID, 4*blockRangeMillis+1, 5*blockRangeMillis, numSeries, externalLabels(""))
				block7 := createDBBlock(t, bkt, userID, 7*blockRangeMillis, 8*blockRangeMillis, numSeries, externalLabels(""))

				return []block.Meta{
					// Block1 have been compacted with block4 and block5 in the 3rd range compaction.
					{
						MinTime: 1,
						MaxTime: model.Time(4 * blockRangeMillis),
						Compaction: block.BlockMetaCompaction{
							Sources: []ulid.ULID{block1a, block4a, block5a},
						},

						Labels: map[string]string{
							sharding.CompactorShardIDLabel: "1_of_2",
						},
					},
					{
						MinTime: 1,
						MaxTime: model.Time(4 * blockRangeMillis),
						Compaction: block.BlockMetaCompaction{
							Sources: []ulid.ULID{block1b, block4b, block5b},
						},

						Labels: map[string]string{
							sharding.CompactorShardIDLabel: "2_of_2",
						},
					},
					// The two overlapping blocks (block2, block3) have been merged in the 1st range.
					{
						MinTime: model.Time(blockRangeMillis),
						MaxTime: model.Time(2 * blockRangeMillis),
						Compaction: block.BlockMetaCompaction{
							Sources: []ulid.ULID{block2, block3},
						},

						Labels: map[string]string{},
					},
					// The two non-adjacent blocks block6 and block7 are merged together in the 3rd range.
					{
						MinTime: model.Time(4*blockRangeMillis) + 1,
						MaxTime: model.Time(8 * blockRangeMillis),
						Compaction: block.BlockMetaCompaction{
							Sources: []ulid.ULID{block6, block7},
						},

						Labels: map[string]string{},
					},
				}
			},
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			workDir := t.TempDir()
			storageDir := t.TempDir()
			fetcherDir := t.TempDir()

			storageCfg := client.Config{
				StorageBackendConfig: client.StorageBackendConfig{
					Backend: client.Filesystem,
					Filesystem: filesystem.Config{
						Directory: storageDir,
					},
				},
			}

			compactorCfg := prepareConfig(t)
			compactorCfg.DataDir = workDir
			compactorCfg.BlockRanges = compactionRanges

			cfgProvider := newMockConfigProvider()
			cfgProvider.splitAndMergeShards[userID] = testData.numShards

			logger := log.NewLogfmtLogger(os.Stdout)
			reg := prometheus.NewPedanticRegistry()
			ctx := context.Background()

			// Create TSDB blocks in the storage and get the expected blocks.
			bkt, err := client.NewBucket(context.Background(), storageCfg, "test")
			require.NoError(t, err)

			defer bkt.Close()
			expected := testData.setup(t, bkt)

			c, err := NewMultitenantCompactor(compactorCfg, bkt, cfgProvider, logger, reg)
			require.NoError(t, err)
			require.NoError(t, services.StartAndAwaitRunning(context.Background(), c))
			t.Cleanup(func() {
				require.NoError(t, services.StopAndAwaitTerminated(context.Background(), c))
			})

			// Wait until the first compaction run completed.
			test.Poll(t, 15*time.Second, nil, func() interface{} {
				return testutil.GatherAndCompare(reg, strings.NewReader(`
					# HELP pyroscope_compactor_runs_completed_total Total number of compaction runs successfully completed.
					# TYPE pyroscope_compactor_runs_completed_total counter
					pyroscope_compactor_runs_completed_total 1
				`), "pyroscope_compactor_runs_completed_total")
			})

			// List back any (non deleted) block from the storage.
			userBucket := objstore.NewTenantBucketClient(userID, bkt, nil)
			fetcher, err := block.NewMetaFetcher(logger,
				1,
				userBucket,
				fetcherDir,
				reg,
				nil,
			)
			require.NoError(t, err)
			metas, partials, err := fetcher.FetchWithoutMarkedForDeletion(ctx)
			require.NoError(t, err)
			require.Empty(t, partials)

			// Sort blocks by MinTime and labels so that we get a stable comparison.
			actual := sortMetasByMinTime(convertMetasMapToSlice(metas))

			// Compare actual blocks with the expected ones.
			require.Len(t, actual, len(expected))
			for i, e := range expected {
				delete(actual[i].Labels, block.HostnameLabel)
				assert.Equal(t, e.MinTime, actual[i].MinTime)
				assert.Equal(t, e.MaxTime, actual[i].MaxTime)
				assert.Equal(t, e.Compaction.Sources, actual[i].Compaction.Sources)
				assert.Equal(t, e.Labels, actual[i].Labels)
			}
		})
	}
}

func TestMultitenantCompactor_ShouldGuaranteeSeriesShardingConsistencyOverTheTime(t *testing.T) {
	const (
		userID     = "user-1"
		numSeries  = 100
		blockRange = 2 * time.Hour
		numShards  = 2
	)

	var (
		blockRangeMillis = blockRange.Milliseconds()
		compactionRanges = DurationList{blockRange}

		// You should NEVER CHANGE the expected series here, otherwise it means you're introducing
		// a backward incompatible change.
		expectedSeriesIDByShard = map[string][]int{
			"1_of_2": {0, 1, 3, 4, 5, 6, 7, 11, 12, 15, 16, 17, 18, 19, 20, 21, 24, 25, 27, 31, 36, 37, 38, 40, 42, 45, 47, 50, 51, 52, 53, 54, 55, 57, 59, 60, 61, 63, 68, 70, 71, 72, 74, 77, 79, 80, 81, 82, 83, 84, 85, 86, 88, 89, 90, 91, 92, 94, 98, 100},
			"2_of_2": {2, 8, 9, 10, 13, 14, 22, 23, 26, 28, 29, 30, 32, 33, 34, 35, 39, 41, 43, 44, 46, 48, 49, 56, 58, 62, 64, 65, 66, 67, 69, 73, 75, 76, 78, 87, 93, 95, 96, 97, 99},
		}
	)

	workDir := t.TempDir()
	storageDir := t.TempDir()
	fetcherDir := t.TempDir()

	storageCfg := client.Config{
		StorageBackendConfig: client.StorageBackendConfig{
			Backend: client.Filesystem,
			Filesystem: filesystem.Config{
				Directory: storageDir,
			},
		},
	}

	compactorCfg := prepareConfig(t)
	compactorCfg.DataDir = workDir
	compactorCfg.BlockRanges = compactionRanges

	cfgProvider := newMockConfigProvider()
	cfgProvider.splitAndMergeShards[userID] = numShards

	logger := log.NewLogfmtLogger(os.Stdout)
	reg := prometheus.NewPedanticRegistry()
	ctx := context.Background()

	bucketClient, err := client.NewBucket(ctx, storageCfg, "test")
	require.NoError(t, err)

	// Create a TSDB block in the storage.
	blockID := createDBBlock(t, bucketClient, userID, blockRangeMillis, 2*blockRangeMillis, numSeries, nil)

	c, err := NewMultitenantCompactor(compactorCfg, bucketClient, cfgProvider, logger, reg)
	require.NoError(t, err)
	require.NoError(t, services.StartAndAwaitRunning(context.Background(), c))
	t.Cleanup(func() {
		require.NoError(t, services.StopAndAwaitTerminated(context.Background(), c))
	})

	// Wait until the first compaction run completed.
	test.Poll(t, 15*time.Second, nil, func() interface{} {
		return testutil.GatherAndCompare(reg, strings.NewReader(`
					# HELP pyroscope_compactor_runs_completed_total Total number of compaction runs successfully completed.
					# TYPE pyroscope_compactor_runs_completed_total counter
					pyroscope_compactor_runs_completed_total 1
				`), "pyroscope_compactor_runs_completed_total")
	})

	// List back any (non deleted) block from the storage.
	userBucket := objstore.NewTenantBucketClient(userID, bucketClient, nil)
	fetcher, err := block.NewMetaFetcher(logger,
		1,
		userBucket,
		fetcherDir,
		reg,
		nil,
	)
	require.NoError(t, err)
	metas, partials, err := fetcher.FetchWithoutMarkedForDeletion(ctx)
	require.NoError(t, err)
	require.Empty(t, partials)

	// Sort blocks by MinTime and labels so that we get a stable comparison.
	actualMetas := sortMetasByMinTime(convertMetasMapToSlice(metas))

	// Ensure the input block has been split.
	require.Len(t, actualMetas, numShards)
	for idx, actualMeta := range actualMetas {
		assert.Equal(t, model.Time(blockRangeMillis), actualMeta.MinTime)
		assert.Equal(t, model.Time(2*blockRangeMillis), actualMeta.MaxTime)
		assert.Equal(t, []ulid.ULID{blockID}, actualMeta.Compaction.Sources)
		assert.Equal(t, sharding.FormatShardIDLabelValue(uint64(idx), numShards), actualMeta.Labels[sharding.CompactorShardIDLabel])
	}

	// Ensure each split block contains the right series, based on a series labels
	// hashing function which doesn't change over time.
	for _, actualMeta := range actualMetas {
		expectedSeriesIDs := expectedSeriesIDByShard[actualMeta.Labels[sharding.CompactorShardIDLabel]]

		b := phlaredb.NewSingleBlockQuerierFromMeta(ctx, userBucket, actualMeta)
		require.NoError(t, b.Open(ctx))
		indexReader := b.Index()

		// Find all series in the block.
		postings, err := indexReader.Postings("series_id", nil)
		require.NoError(t, err)

		lbls := make(phlaremodel.Labels, 0, 6)

		for postings.Next() {
			_, err := indexReader.Series(postings.At(), &lbls, nil)
			// Symbolize the series labels.
			require.NoError(t, err)

			// Ensure the series below to the right shard.
			seriesLabels := lbls.Clone()
			seriesID, err := strconv.Atoi(seriesLabels.Get("series_id"))
			require.NoError(t, err)
			assert.Contains(t, expectedSeriesIDs, seriesID, "series:", seriesLabels.ToPrometheusLabels().String())
		}

		require.NoError(t, postings.Err())
	}
}

func convertMetasMapToSlice(metas map[ulid.ULID]*block.Meta) []*block.Meta {
	var out []*block.Meta
	for _, m := range metas {
		out = append(out, m)
	}
	return out
}

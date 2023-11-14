// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/grafana/mimir/blob/main/pkg/compactor/bucket_compactor_e2e_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package compactor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"

	phlareobj "github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
)

func TestSyncer_GarbageCollect_e2e(t *testing.T) {
	foreachStore(t, func(t *testing.T, bkt phlareobj.Bucket) {
		// Use bucket with global markers to make sure that our custom filters work correctly.
		bkt = block.BucketWithGlobalMarkers(bkt)

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		// Generate 10 source block metas and construct higher level blocks
		// that are higher compactions of them.
		var metas []*block.Meta
		var ids []ulid.ULID

		for i := 0; i < 10; i++ {
			var m block.Meta

			m.Version = 1
			m.ULID = ulid.MustNew(uint64(i), nil)
			m.Compaction.Sources = []ulid.ULID{m.ULID}
			m.Compaction.Level = 1
			m.MinTime = 0
			m.MaxTime = model.Time(2 * time.Hour.Milliseconds())

			ids = append(ids, m.ULID)
			metas = append(metas, &m)
		}

		var m1 block.Meta
		m1.Version = 1
		m1.ULID = ulid.MustNew(100, nil)
		m1.Compaction.Level = 2
		m1.Compaction.Sources = ids[:4]
		m1.Downsample.Resolution = 0

		var m2 block.Meta
		m2.Version = 1
		m2.ULID = ulid.MustNew(200, nil)
		m2.Compaction.Level = 2
		m2.Compaction.Sources = ids[4:8] // last two source IDs is not part of a level 2 block.
		m2.Downsample.Resolution = 0

		var m3 block.Meta
		m3.Version = 1
		m3.ULID = ulid.MustNew(300, nil)
		m3.Compaction.Level = 3
		m3.Compaction.Sources = ids[:9] // last source ID is not part of level 3 block.
		m3.Downsample.Resolution = 0
		m3.MinTime = 0
		m3.MaxTime = model.Time(2 * time.Hour.Milliseconds())

		var m4 block.Meta
		m4.Version = 1
		m4.ULID = ulid.MustNew(400, nil)
		m4.Compaction.Level = 2
		m4.Compaction.Sources = ids[9:] // covers the last block but is a different resolution. Must not trigger deletion.
		m4.Downsample.Resolution = 1000
		m4.MinTime = 0
		m4.MaxTime = model.Time(2 * time.Hour.Milliseconds())

		var m5 block.Meta
		m5.Version = 1
		m5.ULID = ulid.MustNew(500, nil)
		m5.Compaction.Level = 2
		m5.Compaction.Sources = ids[8:9] // built from block 8, but different resolution. Block 8 is already included in m3, can be deleted.
		m5.Downsample.Resolution = 1000
		m5.MinTime = 0
		m5.MaxTime = model.Time(2 * time.Hour.Milliseconds())

		// Create all blocks in the bucket.
		for _, m := range append(metas, &m1, &m2, &m3, &m4, &m5) {
			fmt.Println("create", m.ULID)
			var buf bytes.Buffer
			require.NoError(t, json.NewEncoder(&buf).Encode(&m))
			require.NoError(t, bkt.Upload(ctx, path.Join(m.ULID.String(), block.MetaFilename), &buf))
		}

		duplicateBlocksFilter := NewShardAwareDeduplicateFilter()
		metaFetcher, err := block.NewMetaFetcher(nil, 32, bkt, "", nil, []block.MetadataFilter{
			duplicateBlocksFilter,
		})
		require.NoError(t, err)

		blocksMarkedForDeletion := promauto.With(nil).NewCounter(prometheus.CounterOpts{})
		sy, err := NewMetaSyncer(nil, nil, bkt, metaFetcher, duplicateBlocksFilter, blocksMarkedForDeletion)
		require.NoError(t, err)

		// Do one initial synchronization with the bucket.
		require.NoError(t, sy.SyncMetas(ctx))
		require.NoError(t, sy.GarbageCollect(ctx))

		var rem []ulid.ULID
		err = bkt.Iter(ctx, "", func(n string) error {
			id, ok := block.IsBlockDir(n)
			if !ok {
				return nil
			}
			deletionMarkFile := path.Join(id.String(), block.DeletionMarkFilename)

			exists, err := bkt.Exists(ctx, deletionMarkFile)
			if err != nil {
				return err
			}
			if !exists {
				rem = append(rem, id)
			}
			return nil
		})
		require.NoError(t, err)

		sort.Slice(rem, func(i, j int) bool {
			return rem[i].Compare(rem[j]) < 0
		})

		// Only the level 3 block, the last source block in both resolutions should be left.
		assert.Equal(t, []ulid.ULID{metas[9].ULID, m3.ULID, m4.ULID, m5.ULID}, rem)

		// After another sync the changes should also be reflected in the local groups.
		require.NoError(t, sy.SyncMetas(ctx))
		require.NoError(t, sy.GarbageCollect(ctx))

		// Only the level 3 block, the last source block in both resolutions should be left.
		grouper := NewSplitAndMergeGrouper("user-1", []int64{2 * time.Hour.Milliseconds()}, 0, 0, 0, log.NewNopLogger())
		groups, err := grouper.Groups(sy.Metas())
		require.NoError(t, err)

		assert.Equal(t, "0@17241709254077376921-merge--0-7200000", groups[0].Key())
		assert.Equal(t, []ulid.ULID{metas[9].ULID, m3.ULID}, groups[0].IDs())

		assert.Equal(t, "1000@17241709254077376921-merge--0-7200000", groups[1].Key())
		assert.Equal(t, []ulid.ULID{m4.ULID, m5.ULID}, groups[1].IDs())
	})
}

func TestGroupCompactE2E(t *testing.T) {
	foreachStore(t, func(t *testing.T, bkt phlareobj.Bucket) {
		userbkt := phlareobj.NewTenantBucketClient("user-1", bkt, nil).(phlareobj.Bucket)
		// Use bucket with global markers to make sure that our custom filters work correctly.
		userbkt = block.BucketWithGlobalMarkers(userbkt)

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		// Create fresh, empty directory for actual test.
		dir := t.TempDir()

		// Start dir checker... we make sure that "dir" only contains group subdirectories during compaction,
		// and not any block directories. Dir checker stops when context is canceled, or on first error,
		// in which case error is logger and test is failed. (We cannot use Fatal or FailNow from a goroutine).
		go func() {
			for ctx.Err() == nil {
				fs, err := os.ReadDir(dir)
				if err != nil && !os.IsNotExist(err) {
					t.Log("error while listing directory", dir)
					t.Fail()
					return
				}

				for _, fi := range fs {
					// Suffix used by Prometheus LeveledCompactor when doing compaction.
					toCheck := strings.TrimSuffix(fi.Name(), ".tmp-for-creation")

					_, err := ulid.Parse(toCheck)
					if err == nil {
						t.Log("found block directory in main compaction directory", fi.Name())
						t.Fail()
						return
					}
				}

				select {
				case <-time.After(100 * time.Millisecond):
					continue
				case <-ctx.Done():
					return
				}
			}
		}()

		logger := log.NewLogfmtLogger(os.Stderr)

		duplicateBlocksFilter := NewShardAwareDeduplicateFilter()
		noCompactMarkerFilter := NewNoCompactionMarkFilter(userbkt, true)
		metaFetcher, err := block.NewMetaFetcher(nil, 32, userbkt, "", nil, []block.MetadataFilter{
			duplicateBlocksFilter,
			noCompactMarkerFilter,
		})
		require.NoError(t, err)

		blocksMarkedForDeletion := promauto.With(nil).NewCounter(prometheus.CounterOpts{})
		sy, err := NewMetaSyncer(nil, nil, userbkt, metaFetcher, duplicateBlocksFilter, blocksMarkedForDeletion)
		require.NoError(t, err)

		planner := NewSplitAndMergePlanner([]int64{1000, 3000})
		grouper := NewSplitAndMergeGrouper("user-1", []int64{1000, 3000}, 0, 0, 0, logger)
		metrics := NewBucketCompactorMetrics(blocksMarkedForDeletion, prometheus.NewPedanticRegistry())
		bComp, err := NewBucketCompactor(logger, sy, grouper, planner, &BlockCompactor{
			blockOpenConcurrency: 100,
			splitBy:              phlaredb.SplitByFingerprint,
			logger:               logger,
			metrics:              newCompactorMetrics(nil),
		}, dir, userbkt, 2, ownAllJobs, sortJobsByNewestBlocksFirst, 0, 4, metrics)
		require.NoError(t, err)

		// Compaction on empty should not fail.
		require.NoError(t, bComp.Compact(ctx, 0), 0)
		assert.Equal(t, 0.0, promtest.ToFloat64(sy.metrics.blocksMarkedForDeletion))
		assert.Equal(t, 0.0, promtest.ToFloat64(sy.metrics.garbageCollectionFailures))
		assert.Equal(t, 0.0, promtest.ToFloat64(metrics.blocksMarkedForNoCompact))
		assert.Equal(t, 0.0, promtest.ToFloat64(metrics.groupCompactions))
		assert.Equal(t, 0.0, promtest.ToFloat64(metrics.groupCompactionRunsStarted))
		assert.Equal(t, 0.0, promtest.ToFloat64(metrics.groupCompactionRunsCompleted))
		assert.Equal(t, 0.0, promtest.ToFloat64(metrics.groupCompactionRunsFailed))

		_, err = os.Stat(dir)
		assert.True(t, os.IsNotExist(err), "dir %s should be remove after compaction.", dir)

		m1 := createDBBlock(t, bkt, "user-1", 500, 1000, 10, nil)
		m2 := createDBBlock(t, bkt, "user-1", 500, 1000, 10, nil)

		m3 := createDBBlock(t, bkt, "user-1", 1001, 2000, 10, nil)
		m4 := createDBBlock(t, bkt, "user-1", 1001, 3000, 10, nil)

		require.NoError(t, bComp.Compact(ctx, 0), 0)
		assert.Equal(t, 5.0, promtest.ToFloat64(sy.metrics.blocksMarkedForDeletion))
		assert.Equal(t, 0.0, promtest.ToFloat64(metrics.blocksMarkedForNoCompact))
		assert.Equal(t, 0.0, promtest.ToFloat64(sy.metrics.garbageCollectionFailures))
		assert.Equal(t, 2.0, promtest.ToFloat64(metrics.groupCompactions))
		assert.Equal(t, 2.0, promtest.ToFloat64(metrics.groupCompactionRunsStarted))
		assert.Equal(t, 2.0, promtest.ToFloat64(metrics.groupCompactionRunsCompleted))
		assert.Equal(t, 0.0, promtest.ToFloat64(metrics.groupCompactionRunsFailed))

		_, err = os.Stat(dir)
		assert.True(t, os.IsNotExist(err), "dir %s should be remove after compaction.", dir)

		metas, _, err := metaFetcher.FetchWithoutMarkedForDeletion(context.Background())
		require.NoError(t, err)
		require.Len(t, metas, 1)
		var meta block.Meta
		for _, m := range metas {
			meta = *m
		}
		require.Equal(t, []ulid.ULID{m1, m2, m3, m4}, meta.Compaction.Sources)
		require.Equal(t, 3, meta.Compaction.Level)
		require.Equal(t, model.Time(500), meta.MinTime)
		require.Equal(t, model.Time(3000), meta.MaxTime)
	})
}

func foreachStore(t *testing.T, testFn func(t *testing.T, bkt phlareobj.Bucket)) {
	t.Parallel()

	// Mandatory Inmem. Not parallel, to detect problem early.
	if ok := t.Run("inmem", func(t *testing.T) {
		testFn(t, phlareobj.NewBucket(objstore.NewInMemBucket()))
	}); !ok {
		return
	}

	// Mandatory Filesystem.
	t.Run("filesystem", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		b, err := filesystem.NewBucket(dir)
		require.NoError(t, err)
		testFn(t, b)
	})
}

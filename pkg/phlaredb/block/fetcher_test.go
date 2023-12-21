// SPDX-License-Identifier: AGPL-3.0-only

package block_test

import (
	"context"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	objstore_testutil "github.com/grafana/pyroscope/pkg/objstore/testutil"
	phlarecontext "github.com/grafana/pyroscope/pkg/phlare/context"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	block_testutil "github.com/grafana/pyroscope/pkg/phlaredb/block/testutil"
	"github.com/grafana/pyroscope/pkg/pprof/testhelper"
)

func TestMetaFetcher_Fetch_ShouldReturnDiscoveredBlocksIncludingMarkedForDeletion(t *testing.T) {
	var (
		ctx    = context.Background()
		reg    = prometheus.NewPedanticRegistry()
		logger = log.NewNopLogger()
	)

	// Create a bucket client with global markers.
	bkt, _ := objstore_testutil.NewFilesystemBucket(t, ctx, t.TempDir())
	bkt = block.BucketWithGlobalMarkers(bkt)

	f, err := block.NewMetaFetcher(logger, 10, bkt, t.TempDir(), reg, nil)
	require.NoError(t, err)

	t.Run("should return no metas and no partials on no block in the storage", func(t *testing.T) {
		actualMetas, actualPartials, actualErr := f.Fetch(ctx)
		require.NoError(t, actualErr)
		require.Empty(t, actualMetas)
		require.Empty(t, actualPartials)

		assert.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
			# HELP blocks_meta_sync_failures_total Total blocks metadata synchronization failures
			# TYPE blocks_meta_sync_failures_total counter
			blocks_meta_sync_failures_total 0

			# HELP blocks_meta_synced Number of block metadata synced
			# TYPE blocks_meta_synced gauge
			blocks_meta_synced{state="corrupted-meta-json"} 0
			blocks_meta_synced{state="duplicate"} 0
			blocks_meta_synced{state="failed"} 0
			blocks_meta_synced{state="label-excluded"} 0
			blocks_meta_synced{state="loaded"} 0
			blocks_meta_synced{state="marked-for-deletion"} 0
			blocks_meta_synced{state="marked-for-no-compact"} 0
			blocks_meta_synced{state="no-meta-json"} 0
			blocks_meta_synced{state="time-excluded"} 0

			# HELP blocks_meta_syncs_total Total blocks metadata synchronization attempts
			# TYPE blocks_meta_syncs_total counter
			blocks_meta_syncs_total 1
		`), "blocks_meta_syncs_total", "blocks_meta_sync_failures_total", "blocks_meta_synced"))
	})

	// Upload a block.
	block1ID, block1Dir := createTestBlock(t)
	require.NoError(t, block.Upload(ctx, logger, bkt, block1Dir))

	// Upload a partial block.
	block2ID, block2Dir := createTestBlock(t)
	require.NoError(t, block.Upload(ctx, logger, bkt, block2Dir))
	require.NoError(t, bkt.Delete(ctx, path.Join(block2ID.String(), block.MetaFilename)))

	t.Run("should return metas and partials on some blocks in the storage", func(t *testing.T) {
		actualMetas, actualPartials, actualErr := f.Fetch(ctx)
		require.NoError(t, actualErr)
		require.Len(t, actualMetas, 1)
		require.Contains(t, actualMetas, block1ID)
		require.Len(t, actualPartials, 1)
		require.Contains(t, actualPartials, block2ID)

		assert.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
			# HELP blocks_meta_sync_failures_total Total blocks metadata synchronization failures
			# TYPE blocks_meta_sync_failures_total counter
			blocks_meta_sync_failures_total 0

			# HELP blocks_meta_synced Number of block metadata synced
			# TYPE blocks_meta_synced gauge
			blocks_meta_synced{state="corrupted-meta-json"} 0
			blocks_meta_synced{state="duplicate"} 0
			blocks_meta_synced{state="failed"} 0
			blocks_meta_synced{state="label-excluded"} 0
			blocks_meta_synced{state="loaded"} 1
			blocks_meta_synced{state="marked-for-deletion"} 0
			blocks_meta_synced{state="marked-for-no-compact"} 0
			blocks_meta_synced{state="no-meta-json"} 1
			blocks_meta_synced{state="time-excluded"} 0

			# HELP blocks_meta_syncs_total Total blocks metadata synchronization attempts
			# TYPE blocks_meta_syncs_total counter
			blocks_meta_syncs_total 2
		`), "blocks_meta_syncs_total", "blocks_meta_sync_failures_total", "blocks_meta_synced"))
	})

	// Upload a block and mark it for deletion.
	block3ID, block3Dir := createTestBlock(t)
	require.NoError(t, block.Upload(ctx, logger, bkt, block3Dir))
	require.NoError(t, block.MarkForDeletion(ctx, logger, bkt, block3ID, "", false, promauto.With(nil).NewCounter(prometheus.CounterOpts{})))

	t.Run("should include blocks marked for deletion", func(t *testing.T) {
		actualMetas, actualPartials, actualErr := f.Fetch(ctx)
		require.NoError(t, actualErr)
		require.Len(t, actualMetas, 2)
		require.Contains(t, actualMetas, block1ID)
		require.Contains(t, actualMetas, block3ID)
		require.Len(t, actualPartials, 1)
		require.Contains(t, actualPartials, block2ID)

		assert.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
			# HELP blocks_meta_sync_failures_total Total blocks metadata synchronization failures
			# TYPE blocks_meta_sync_failures_total counter
			blocks_meta_sync_failures_total 0

			# HELP blocks_meta_synced Number of block metadata synced
			# TYPE blocks_meta_synced gauge
			blocks_meta_synced{state="corrupted-meta-json"} 0
			blocks_meta_synced{state="duplicate"} 0
			blocks_meta_synced{state="failed"} 0
			blocks_meta_synced{state="label-excluded"} 0
			blocks_meta_synced{state="loaded"} 2
			blocks_meta_synced{state="marked-for-deletion"} 0
			blocks_meta_synced{state="marked-for-no-compact"} 0
			blocks_meta_synced{state="no-meta-json"} 1
			blocks_meta_synced{state="time-excluded"} 0

			# HELP blocks_meta_syncs_total Total blocks metadata synchronization attempts
			# TYPE blocks_meta_syncs_total counter
			blocks_meta_syncs_total 3
		`), "blocks_meta_syncs_total", "blocks_meta_sync_failures_total", "blocks_meta_synced"))
	})
}

func TestMetaFetcher_FetchWithoutMarkedForDeletion_ShouldReturnDiscoveredBlocksExcludingMarkedForDeletion(t *testing.T) {
	var (
		ctx    = context.Background()
		reg    = prometheus.NewPedanticRegistry()
		logger = log.NewNopLogger()
	)

	// Create a bucket client with global markers.
	bkt, _ := objstore_testutil.NewFilesystemBucket(t, ctx, t.TempDir())
	bkt = block.BucketWithGlobalMarkers(bkt)

	f, err := block.NewMetaFetcher(logger, 10, bkt, t.TempDir(), reg, nil)
	require.NoError(t, err)

	t.Run("should return no metas and no partials on no block in the storage", func(t *testing.T) {
		actualMetas, actualPartials, actualErr := f.FetchWithoutMarkedForDeletion(ctx)
		require.NoError(t, actualErr)
		require.Empty(t, actualMetas)
		require.Empty(t, actualPartials)

		assert.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
			# HELP blocks_meta_sync_failures_total Total blocks metadata synchronization failures
			# TYPE blocks_meta_sync_failures_total counter
			blocks_meta_sync_failures_total 0

			# HELP blocks_meta_synced Number of block metadata synced
			# TYPE blocks_meta_synced gauge
			blocks_meta_synced{state="corrupted-meta-json"} 0
			blocks_meta_synced{state="duplicate"} 0
			blocks_meta_synced{state="failed"} 0
			blocks_meta_synced{state="label-excluded"} 0
			blocks_meta_synced{state="loaded"} 0
			blocks_meta_synced{state="marked-for-deletion"} 0
			blocks_meta_synced{state="marked-for-no-compact"} 0
			blocks_meta_synced{state="no-meta-json"} 0
			blocks_meta_synced{state="time-excluded"} 0

			# HELP blocks_meta_syncs_total Total blocks metadata synchronization attempts
			# TYPE blocks_meta_syncs_total counter
			blocks_meta_syncs_total 1
		`), "blocks_meta_syncs_total", "blocks_meta_sync_failures_total", "blocks_meta_synced"))
	})

	// Upload a block.
	block1ID, block1Dir := createTestBlock(t)
	require.NoError(t, block.Upload(ctx, logger, bkt, block1Dir))

	// Upload a partial block.
	block2ID, block2Dir := createTestBlock(t)
	require.NoError(t, block.Upload(ctx, logger, bkt, block2Dir))
	require.NoError(t, bkt.Delete(ctx, path.Join(block2ID.String(), block.MetaFilename)))

	t.Run("should return metas and partials on some blocks in the storage", func(t *testing.T) {
		actualMetas, actualPartials, actualErr := f.FetchWithoutMarkedForDeletion(ctx)
		require.NoError(t, actualErr)
		require.Len(t, actualMetas, 1)
		require.Contains(t, actualMetas, block1ID)
		require.Len(t, actualPartials, 1)
		require.Contains(t, actualPartials, block2ID)

		assert.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
			# HELP blocks_meta_sync_failures_total Total blocks metadata synchronization failures
			# TYPE blocks_meta_sync_failures_total counter
			blocks_meta_sync_failures_total 0

			# HELP blocks_meta_synced Number of block metadata synced
			# TYPE blocks_meta_synced gauge
			blocks_meta_synced{state="corrupted-meta-json"} 0
			blocks_meta_synced{state="duplicate"} 0
			blocks_meta_synced{state="failed"} 0
			blocks_meta_synced{state="label-excluded"} 0
			blocks_meta_synced{state="loaded"} 1
			blocks_meta_synced{state="marked-for-deletion"} 0
			blocks_meta_synced{state="marked-for-no-compact"} 0
			blocks_meta_synced{state="no-meta-json"} 1
			blocks_meta_synced{state="time-excluded"} 0

			# HELP blocks_meta_syncs_total Total blocks metadata synchronization attempts
			# TYPE blocks_meta_syncs_total counter
			blocks_meta_syncs_total 2
		`), "blocks_meta_syncs_total", "blocks_meta_sync_failures_total", "blocks_meta_synced"))
	})

	// Upload a block and mark it for deletion.
	block3ID, block3Dir := createTestBlock(t)
	require.NoError(t, block.Upload(ctx, logger, bkt, block3Dir))
	require.NoError(t, block.MarkForDeletion(ctx, logger, bkt, block3ID, "", false, promauto.With(nil).NewCounter(prometheus.CounterOpts{})))

	t.Run("should include blocks marked for deletion", func(t *testing.T) {
		actualMetas, actualPartials, actualErr := f.FetchWithoutMarkedForDeletion(ctx)
		require.NoError(t, actualErr)
		require.Len(t, actualMetas, 1)
		require.Contains(t, actualMetas, block1ID)
		require.Len(t, actualPartials, 1)
		require.Contains(t, actualPartials, block2ID)

		assert.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
			# HELP blocks_meta_sync_failures_total Total blocks metadata synchronization failures
			# TYPE blocks_meta_sync_failures_total counter
			blocks_meta_sync_failures_total 0

			# HELP blocks_meta_synced Number of block metadata synced
			# TYPE blocks_meta_synced gauge
			blocks_meta_synced{state="corrupted-meta-json"} 0
			blocks_meta_synced{state="duplicate"} 0
			blocks_meta_synced{state="failed"} 0
			blocks_meta_synced{state="label-excluded"} 0
			blocks_meta_synced{state="loaded"} 1
			blocks_meta_synced{state="marked-for-deletion"} 1
			blocks_meta_synced{state="marked-for-no-compact"} 0
			blocks_meta_synced{state="no-meta-json"} 1
			blocks_meta_synced{state="time-excluded"} 0

			# HELP blocks_meta_syncs_total Total blocks metadata synchronization attempts
			# TYPE blocks_meta_syncs_total counter
			blocks_meta_syncs_total 3
		`), "blocks_meta_syncs_total", "blocks_meta_sync_failures_total", "blocks_meta_synced"))
	})
}

func TestMetaFetcher_ShouldNotIssueAnyAPICallToObjectStorageIfAllBlockMetasAreCached(t *testing.T) {
	var (
		ctx        = context.Background()
		logger     = log.NewNopLogger()
		fetcherDir = t.TempDir()
	)
	bkt, _ := objstore_testutil.NewFilesystemBucket(t, ctx, fetcherDir)

	// Upload few blocks.
	block1ID, block1Dir := createTestBlock(t)
	require.NoError(t, block.Upload(ctx, logger, bkt, block1Dir))
	block2ID, block2Dir := createTestBlock(t)
	require.NoError(t, block.Upload(ctx, logger, bkt, block2Dir))

	// Create a fetcher and fetch block metas to populate the cache on disk.
	reg1 := prometheus.NewPedanticRegistry()
	ctx = phlarecontext.WithRegistry(ctx, reg1)
	bkt, _ = objstore_testutil.NewFilesystemBucket(t, ctx, fetcherDir)
	fetcher1, err := block.NewMetaFetcher(logger, 10, bkt, fetcherDir, nil, nil)
	require.NoError(t, err)
	actualMetas, _, actualErr := fetcher1.Fetch(ctx)
	require.NoError(t, actualErr)
	require.Len(t, actualMetas, 2)
	require.Contains(t, actualMetas, block1ID)
	require.Contains(t, actualMetas, block2ID)

	assert.NoError(t, testutil.GatherAndCompare(reg1, strings.NewReader(`
		# HELP objstore_bucket_operations_total Total number of all attempted operations against a bucket.
		# TYPE objstore_bucket_operations_total counter
		objstore_bucket_operations_total{bucket="test",operation="attributes"} 0
		objstore_bucket_operations_total{bucket="test",operation="exists"} 0
		objstore_bucket_operations_total{bucket="test",operation="delete"} 0
		objstore_bucket_operations_total{bucket="test",operation="upload"} 0
		objstore_bucket_operations_total{bucket="test",operation="get"} 2
		objstore_bucket_operations_total{bucket="test",operation="get_range"} 0
		objstore_bucket_operations_total{bucket="test",operation="iter"} 1
	`), "objstore_bucket_operations_total"))

	// Create a new fetcher and fetch blocks again. This time we expect all meta.json to be loaded from cache.
	reg2 := prometheus.NewPedanticRegistry()
	ctx = phlarecontext.WithRegistry(ctx, reg2)
	bkt, _ = objstore_testutil.NewFilesystemBucket(t, ctx, fetcherDir)
	fetcher2, err := block.NewMetaFetcher(logger, 10, bkt, fetcherDir, nil, nil)
	require.NoError(t, err)
	actualMetas, _, actualErr = fetcher2.Fetch(ctx)
	require.NoError(t, actualErr)
	require.Len(t, actualMetas, 2)
	require.Contains(t, actualMetas, block1ID)
	require.Contains(t, actualMetas, block2ID)

	assert.NoError(t, testutil.GatherAndCompare(reg2, strings.NewReader(`
		# HELP objstore_bucket_operations_total Total number of all attempted operations against a bucket.
		# TYPE objstore_bucket_operations_total counter
		objstore_bucket_operations_total{bucket="test",operation="attributes"} 0
		objstore_bucket_operations_total{bucket="test",operation="delete"} 0
		objstore_bucket_operations_total{bucket="test",operation="exists"} 0
		objstore_bucket_operations_total{bucket="test",operation="get"} 0
		objstore_bucket_operations_total{bucket="test",operation="get_range"} 0
		objstore_bucket_operations_total{bucket="test",operation="iter"} 1
		objstore_bucket_operations_total{bucket="test",operation="upload"} 0
	`), "objstore_bucket_operations_total"))
}

func createTestBlock(t *testing.T) (blockID ulid.ULID, blockDir string) {
	meta, dir := block_testutil.CreateBlock(t, func() []*testhelper.ProfileBuilder {
		return []*testhelper.ProfileBuilder{
			testhelper.NewProfileBuilder(int64(1)).
				CPUProfile().
				WithLabels(
					"job", "a",
				).ForStacktraceString("foo", "bar", "baz").AddSamples(1),
		}
	})
	blockID = meta.ULID
	blockDir = filepath.Join(dir, blockID.String())
	return
}

// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/grafana/mimir/blob/main/pkg/compactor/blocks_cleaner_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package compactor

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/concurrency"
	"github.com/grafana/dskit/services"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/objstore"
	objstore_testutil "github.com/grafana/pyroscope/pkg/objstore/testutil"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/phlaredb/bucket"
	"github.com/grafana/pyroscope/pkg/phlaredb/bucketindex"
	"github.com/grafana/pyroscope/pkg/test"
	"github.com/grafana/pyroscope/pkg/util"
)

type testBlocksCleanerOptions struct {
	concurrency         int
	tenantDeletionDelay time.Duration
	user4FilesExist     bool // User 4 has "FinishedTime" in tenant deletion marker set to "1h" ago.
}

func (o testBlocksCleanerOptions) String() string {
	return fmt.Sprintf("concurrency=%d, tenant deletion delay=%v",
		o.concurrency, o.tenantDeletionDelay)
}

func TestBlocksCleaner(t *testing.T) {
	for _, options := range []testBlocksCleanerOptions{
		{concurrency: 1, tenantDeletionDelay: 0, user4FilesExist: false},
		{concurrency: 1, tenantDeletionDelay: 2 * time.Hour, user4FilesExist: true},
		{concurrency: 2},
		{concurrency: 10},
	} {
		options := options

		t.Run(options.String(), func(t *testing.T) {
			t.Parallel()
			testBlocksCleanerWithOptions(t, options)
		})
	}
}

func testBlocksCleanerWithOptions(t *testing.T, options testBlocksCleanerOptions) {
	bucketClient, _ := objstore_testutil.NewFilesystemBucket(t, context.Background(), t.TempDir())
	bucketClient = block.BucketWithGlobalMarkers(bucketClient)

	// Create blocks.
	ctx := context.Background()
	now := time.Now()
	deletionDelay := 12 * time.Hour
	block1 := createDBBlock(t, bucketClient, "user-1", 10, 20, 2, nil)
	block2 := createDBBlock(t, bucketClient, "user-1", 20, 30, 2, nil)
	block3 := createDBBlock(t, bucketClient, "user-1", 30, 40, 2, nil)
	block4 := ulid.MustNew(4, rand.Reader)
	block5 := ulid.MustNew(5, rand.Reader)
	block6 := createDBBlock(t, bucketClient, "user-1", 40, 50, 2, nil)
	block7 := createDBBlock(t, bucketClient, "user-2", 10, 20, 2, nil)
	block8 := createDBBlock(t, bucketClient, "user-2", 40, 50, 2, nil)
	createDeletionMark(t, bucketClient, "user-1", block2, now.Add(-deletionDelay).Add(time.Hour))                      // Block hasn't reached the deletion threshold yet.
	createDeletionMark(t, bucketClient, "user-1", block3, now.Add(-deletionDelay).Add(-time.Hour))                     // Block reached the deletion threshold.
	createDeletionMark(t, bucketClient, "user-1", block4, now.Add(-deletionDelay).Add(time.Hour))                      // Partial block hasn't reached the deletion threshold yet.
	createDeletionMark(t, bucketClient, "user-1", block5, now.Add(-deletionDelay).Add(-time.Hour))                     // Partial block reached the deletion threshold.
	require.NoError(t, bucketClient.Delete(ctx, path.Join("user-1", "phlaredb", block6.String(), block.MetaFilename))) // Partial block without deletion mark.
	createDeletionMark(t, bucketClient, "user-2", block7, now.Add(-deletionDelay).Add(-time.Hour))                     // Block reached the deletion threshold.

	// Blocks for user-3, marked for deletion.
	require.NoError(t, bucket.WriteTenantDeletionMark(context.Background(), bucketClient, "user-3", nil, bucket.NewTenantDeletionMark(time.Now())))
	block9 := createDBBlock(t, bucketClient, "user-3", 10, 30, 2, nil)
	block10 := createDBBlock(t, bucketClient, "user-3", 30, 50, 2, nil)

	// User-4 with no more blocks, but couple of mark and debug files. Should be fully deleted.
	user4Mark := bucket.NewTenantDeletionMark(time.Now())
	user4Mark.FinishedTime = time.Now().Unix() - 60 // Set to check final user cleanup.
	require.NoError(t, bucket.WriteTenantDeletionMark(context.Background(), bucketClient, "user-4", nil, user4Mark))

	cfg := BlocksCleanerConfig{
		DeletionDelay:           deletionDelay,
		CleanupInterval:         time.Minute,
		CleanupConcurrency:      options.concurrency,
		TenantCleanupDelay:      options.tenantDeletionDelay,
		DeleteBlocksConcurrency: 1,
	}

	reg := prometheus.NewPedanticRegistry()
	logger := log.NewNopLogger()
	cfgProvider := newMockConfigProvider()

	cleaner := NewBlocksCleaner(cfg, bucketClient, bucket.AllTenants, cfgProvider, logger, reg)
	require.NoError(t, services.StartAndAwaitRunning(ctx, cleaner))
	defer services.StopAndAwaitTerminated(ctx, cleaner) //nolint:errcheck

	for _, tc := range []struct {
		path           string
		expectedExists bool
	}{
		// Check the storage to ensure only the block which has reached the deletion threshold
		// has been effectively deleted.
		{path: path.Join("user-1", "phlaredb/", block1.String(), block.MetaFilename), expectedExists: true},
		{path: path.Join("user-1", "phlaredb/", block3.String(), block.MetaFilename), expectedExists: false},
		{path: path.Join("user-2", "phlaredb/", block7.String(), block.MetaFilename), expectedExists: false},
		{path: path.Join("user-2", "phlaredb/", block8.String(), block.MetaFilename), expectedExists: true},
		// Should not delete a block with deletion mark who hasn't reached the deletion threshold yet.
		{path: path.Join("user-1", "phlaredb/", block2.String(), block.MetaFilename), expectedExists: true},
		{path: path.Join("user-1", "phlaredb/", block.DeletionMarkFilepath(block2)), expectedExists: true},
		// Should delete a partial block with deletion mark who hasn't reached the deletion threshold yet.
		{path: path.Join("user-1", "phlaredb/", block4.String(), block.DeletionMarkFilename), expectedExists: false},
		{path: path.Join("user-1", "phlaredb/", block.DeletionMarkFilepath(block4)), expectedExists: false},
		// Should delete a partial block with deletion mark who has reached the deletion threshold.
		{path: path.Join("user-1", "phlaredb/", block5.String(), block.DeletionMarkFilename), expectedExists: false},
		{path: path.Join("user-1", "phlaredb/", block.DeletionMarkFilepath(block5)), expectedExists: false},
		// Should not delete a partial block without deletion mark.
		{path: path.Join("user-1", "phlaredb/", block6.String(), block.IndexFilename), expectedExists: true},
		// Should completely delete blocks for user-3, marked for deletion
		{path: path.Join("user-3", "phlaredb/", block9.String(), block.MetaFilename), expectedExists: false},
		{path: path.Join("user-3", "phlaredb/", block9.String(), block.IndexFilename), expectedExists: false},
		{path: path.Join("user-3", "phlaredb/", block10.String(), block.MetaFilename), expectedExists: false},
		{path: path.Join("user-3", "phlaredb/", block10.String(), block.IndexFilename), expectedExists: false},
		// Tenant deletion mark is not removed.
		{path: path.Join("user-3", "phlaredb/", bucket.TenantDeletionMarkPath), expectedExists: true},
		// User-4 is removed fully.
		{path: path.Join("user-4", "phlaredb/", bucket.TenantDeletionMarkPath), expectedExists: options.user4FilesExist},
	} {
		exists, err := bucketClient.Exists(ctx, tc.path)
		require.NoError(t, err)
		assert.Equal(t, tc.expectedExists, exists, tc.path)
	}

	assert.Equal(t, float64(1), testutil.ToFloat64(cleaner.runsStarted))
	assert.Equal(t, float64(1), testutil.ToFloat64(cleaner.runsCompleted))
	assert.Equal(t, float64(0), testutil.ToFloat64(cleaner.runsFailed))
	assert.Equal(t, float64(6), testutil.ToFloat64(cleaner.blocksCleanedTotal))
	assert.Equal(t, float64(0), testutil.ToFloat64(cleaner.blocksFailedTotal))

	// Check the updated bucket index.
	for _, tc := range []struct {
		userID         string
		expectedIndex  bool
		expectedBlocks []ulid.ULID
		expectedMarks  []ulid.ULID
	}{
		{
			userID:         "user-1",
			expectedIndex:  true,
			expectedBlocks: []ulid.ULID{block1, block2 /* deleted: block3, block4, block5, partial: block6 */},
			expectedMarks:  []ulid.ULID{block2},
		}, {
			userID:         "user-2",
			expectedIndex:  true,
			expectedBlocks: []ulid.ULID{block8},
			expectedMarks:  []ulid.ULID{},
		}, {
			userID:        "user-3",
			expectedIndex: false,
		},
	} {
		idx, err := bucketindex.ReadIndex(ctx, bucketClient, tc.userID, nil, logger)
		if !tc.expectedIndex {
			assert.Equal(t, bucketindex.ErrIndexNotFound, err)
			continue
		}

		require.NoError(t, err)
		assert.ElementsMatch(t, tc.expectedBlocks, idx.Blocks.GetULIDs())
		assert.ElementsMatch(t, tc.expectedMarks, idx.BlockDeletionMarks.GetULIDs())
	}

	assert.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
		# HELP pyroscope_bucket_blocks_count Total number of blocks in the bucket. Includes blocks marked for deletion, but not partial blocks.
		# TYPE pyroscope_bucket_blocks_count gauge
		pyroscope_bucket_blocks_count{compaction_level="1", user="user-1"} 2
		pyroscope_bucket_blocks_count{compaction_level="1", user="user-2"} 1
		# HELP pyroscope_bucket_blocks_marked_for_deletion_count Total number of blocks marked for deletion in the bucket.
		# TYPE pyroscope_bucket_blocks_marked_for_deletion_count gauge
		pyroscope_bucket_blocks_marked_for_deletion_count{user="user-1"} 1
		pyroscope_bucket_blocks_marked_for_deletion_count{user="user-2"} 0
		# HELP pyroscope_bucket_blocks_partials_count Total number of partial blocks.
		# TYPE pyroscope_bucket_blocks_partials_count gauge
		pyroscope_bucket_blocks_partials_count{user="user-1"} 2
		pyroscope_bucket_blocks_partials_count{user="user-2"} 0
	`),
		"pyroscope_bucket_blocks_count",
		"pyroscope_bucket_blocks_marked_for_deletion_count",
		"pyroscope_bucket_blocks_partials_count",
	))
}

func TestBlocksCleaner_ShouldContinueOnBlockDeletionFailure(t *testing.T) {
	const userID = "user-1"

	bucketClient, _ := objstore_testutil.NewFilesystemBucket(t, context.Background(), t.TempDir())
	bucketClient = block.BucketWithGlobalMarkers(bucketClient)

	// Create blocks.
	ctx := context.Background()
	now := time.Now()
	deletionDelay := 12 * time.Hour
	block1 := createDBBlock(t, bucketClient, userID, 10, 20, 2, nil)
	block2 := createDBBlock(t, bucketClient, userID, 20, 30, 2, nil)
	block3 := createDBBlock(t, bucketClient, userID, 30, 40, 2, nil)
	block4 := createDBBlock(t, bucketClient, userID, 40, 50, 2, nil)
	createDeletionMark(t, bucketClient, userID, block2, now.Add(-deletionDelay).Add(-time.Hour))
	createDeletionMark(t, bucketClient, userID, block3, now.Add(-deletionDelay).Add(-time.Hour))
	createDeletionMark(t, bucketClient, userID, block4, now.Add(-deletionDelay).Add(-time.Hour))

	// To emulate a failure deleting a block, we wrap the bucket client in a mocked one.
	bucketClient = &mockBucketFailure{
		Bucket:         bucketClient,
		DeleteFailures: []string{path.Join(userID, "phlaredb/", block3.String(), block.MetaFilename)},
	}

	cfg := BlocksCleanerConfig{
		DeletionDelay:           deletionDelay,
		CleanupInterval:         time.Minute,
		CleanupConcurrency:      1,
		DeleteBlocksConcurrency: 1,
	}

	logger := log.NewNopLogger()
	cfgProvider := newMockConfigProvider()

	cleaner := NewBlocksCleaner(cfg, bucketClient, bucket.AllTenants, cfgProvider, logger, nil)
	require.NoError(t, services.StartAndAwaitRunning(ctx, cleaner))
	defer services.StopAndAwaitTerminated(ctx, cleaner) //nolint:errcheck

	for _, tc := range []struct {
		path           string
		expectedExists bool
	}{
		{path: path.Join(userID, "phlaredb/", block1.String(), block.MetaFilename), expectedExists: true},
		{path: path.Join(userID, "phlaredb/", block2.String(), block.MetaFilename), expectedExists: false},
		{path: path.Join(userID, "phlaredb/", block3.String(), block.MetaFilename), expectedExists: true},
		{path: path.Join(userID, "phlaredb/", block4.String(), block.MetaFilename), expectedExists: false},
	} {
		exists, err := bucketClient.Exists(ctx, tc.path)
		require.NoError(t, err)
		assert.Equal(t, tc.expectedExists, exists, tc.path)
	}

	assert.Equal(t, float64(1), testutil.ToFloat64(cleaner.runsStarted))
	assert.Equal(t, float64(1), testutil.ToFloat64(cleaner.runsCompleted))
	assert.Equal(t, float64(0), testutil.ToFloat64(cleaner.runsFailed))
	assert.Equal(t, float64(2), testutil.ToFloat64(cleaner.blocksCleanedTotal))
	assert.Equal(t, float64(1), testutil.ToFloat64(cleaner.blocksFailedTotal))

	// Check the updated bucket index.
	idx, err := bucketindex.ReadIndex(ctx, bucketClient, userID, nil, logger)
	require.NoError(t, err)
	assert.ElementsMatch(t, []ulid.ULID{block1, block3}, idx.Blocks.GetULIDs())
	assert.ElementsMatch(t, []ulid.ULID{block3}, idx.BlockDeletionMarks.GetULIDs())
}

func TestBlocksCleaner_ShouldRebuildBucketIndexOnCorruptedOne(t *testing.T) {
	const userID = "user-1"

	bucketClient, _ := objstore_testutil.NewFilesystemBucket(t, context.Background(), t.TempDir())
	bucketClient = block.BucketWithGlobalMarkers(bucketClient)

	// Create blocks.
	ctx := context.Background()
	now := time.Now()
	deletionDelay := 12 * time.Hour
	block1 := createDBBlock(t, bucketClient, userID, 10, 20, 2, nil)
	block2 := createDBBlock(t, bucketClient, userID, 20, 30, 2, nil)
	block3 := createDBBlock(t, bucketClient, userID, 30, 40, 2, nil)
	createDeletionMark(t, bucketClient, userID, block2, now.Add(-deletionDelay).Add(-time.Hour))
	createDeletionMark(t, bucketClient, userID, block3, now.Add(-deletionDelay).Add(time.Hour))

	// Write a corrupted bucket index.
	require.NoError(t, bucketClient.Upload(ctx, path.Join(userID, "phlaredb/", bucketindex.IndexCompressedFilename), strings.NewReader("invalid!}")))

	cfg := BlocksCleanerConfig{
		DeletionDelay:           deletionDelay,
		CleanupInterval:         time.Minute,
		CleanupConcurrency:      1,
		DeleteBlocksConcurrency: 1,
	}

	logger := log.NewNopLogger()
	cfgProvider := newMockConfigProvider()

	cleaner := NewBlocksCleaner(cfg, bucketClient, bucket.AllTenants, cfgProvider, logger, nil)
	require.NoError(t, services.StartAndAwaitRunning(ctx, cleaner))
	defer services.StopAndAwaitTerminated(ctx, cleaner) //nolint:errcheck

	for _, tc := range []struct {
		path           string
		expectedExists bool
	}{
		{path: path.Join(userID, "phlaredb/", block1.String(), block.MetaFilename), expectedExists: true},
		{path: path.Join(userID, "phlaredb/", block2.String(), block.MetaFilename), expectedExists: false},
		{path: path.Join(userID, "phlaredb/", block3.String(), block.MetaFilename), expectedExists: true},
	} {
		exists, err := bucketClient.Exists(ctx, tc.path)
		require.NoError(t, err)
		assert.Equal(t, tc.expectedExists, exists, tc.path)
	}

	assert.Equal(t, float64(1), testutil.ToFloat64(cleaner.runsStarted))
	assert.Equal(t, float64(1), testutil.ToFloat64(cleaner.runsCompleted))
	assert.Equal(t, float64(0), testutil.ToFloat64(cleaner.runsFailed))
	assert.Equal(t, float64(1), testutil.ToFloat64(cleaner.blocksCleanedTotal))
	assert.Equal(t, float64(0), testutil.ToFloat64(cleaner.blocksFailedTotal))

	// Check the updated bucket index.
	idx, err := bucketindex.ReadIndex(ctx, bucketClient, userID, nil, logger)
	require.NoError(t, err)
	assert.ElementsMatch(t, []ulid.ULID{block1, block3}, idx.Blocks.GetULIDs())
	assert.ElementsMatch(t, []ulid.ULID{block3}, idx.BlockDeletionMarks.GetULIDs())
}

func TestBlocksCleaner_ShouldRemoveMetricsForTenantsNotBelongingAnymoreToTheShard(t *testing.T) {
	bucketClient, _ := objstore_testutil.NewFilesystemBucket(t, context.Background(), t.TempDir())
	bucketClient = block.BucketWithGlobalMarkers(bucketClient)

	// Create blocks.
	createDBBlock(t, bucketClient, "user-1", 10, 20, 2, nil)
	createDBBlock(t, bucketClient, "user-1", 20, 30, 2, nil)
	createDBBlock(t, bucketClient, "user-2", 30, 40, 2, nil)

	cfg := BlocksCleanerConfig{
		DeletionDelay:           time.Hour,
		CleanupInterval:         time.Minute,
		CleanupConcurrency:      1,
		DeleteBlocksConcurrency: 1,
	}

	ctx := context.Background()
	logger := log.NewNopLogger()
	reg := prometheus.NewPedanticRegistry()
	cfgProvider := newMockConfigProvider()

	cleaner := NewBlocksCleaner(cfg, bucketClient, bucket.AllTenants, cfgProvider, logger, reg)
	require.NoError(t, cleaner.runCleanupWithErr(ctx))

	assert.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
		# HELP pyroscope_bucket_blocks_count Total number of blocks in the bucket. Includes blocks marked for deletion, but not partial blocks.
		# TYPE pyroscope_bucket_blocks_count gauge
		pyroscope_bucket_blocks_count{compaction_level="1", user="user-1"} 2
		pyroscope_bucket_blocks_count{compaction_level="1", user="user-2"} 1
		# HELP pyroscope_bucket_blocks_marked_for_deletion_count Total number of blocks marked for deletion in the bucket.
		# TYPE pyroscope_bucket_blocks_marked_for_deletion_count gauge
		pyroscope_bucket_blocks_marked_for_deletion_count{user="user-1"} 0
		pyroscope_bucket_blocks_marked_for_deletion_count{user="user-2"} 0
		# HELP pyroscope_bucket_blocks_partials_count Total number of partial blocks.
		# TYPE pyroscope_bucket_blocks_partials_count gauge
		pyroscope_bucket_blocks_partials_count{user="user-1"} 0
		pyroscope_bucket_blocks_partials_count{user="user-2"} 0
	`),
		"pyroscope_bucket_blocks_count",
		"pyroscope_bucket_blocks_marked_for_deletion_count",
		"pyroscope_bucket_blocks_partials_count",
	))

	// Override the users scanner to reconfigure it to only return a subset of users.
	cleaner.tenantsScanner = bucket.NewTenantsScanner(bucketClient, func(userID string) (bool, error) { return userID == "user-1", nil }, logger)

	// Create new blocks, to double check expected metrics have changed.
	createDBBlock(t, bucketClient, "user-1", 40, 50, 2, nil)
	createDBBlock(t, bucketClient, "user-2", 50, 60, 2, nil)

	require.NoError(t, cleaner.runCleanupWithErr(ctx))

	assert.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
		# HELP pyroscope_bucket_blocks_count Total number of blocks in the bucket. Includes blocks marked for deletion, but not partial blocks.
		# TYPE pyroscope_bucket_blocks_count gauge
		pyroscope_bucket_blocks_count{compaction_level="1", user="user-1"} 3
		pyroscope_bucket_blocks_count{compaction_level="1", user="user-2"} 1
		# HELP pyroscope_bucket_blocks_marked_for_deletion_count Total number of blocks marked for deletion in the bucket.
		# TYPE pyroscope_bucket_blocks_marked_for_deletion_count gauge
		pyroscope_bucket_blocks_marked_for_deletion_count{user="user-1"} 0
		# HELP pyroscope_bucket_blocks_partials_count Total number of partial blocks.
		# TYPE pyroscope_bucket_blocks_partials_count gauge
		pyroscope_bucket_blocks_partials_count{user="user-1"} 0
	`),
		"pyroscope_bucket_blocks_count",
		"pyroscope_bucket_blocks_marked_for_deletion_count",
		"pyroscope_bucket_blocks_partials_count",
	))
}

func TestBlocksCleaner_ShouldNotCleanupUserThatDoesntBelongToShardAnymore(t *testing.T) {
	bucketClient, _ := objstore_testutil.NewFilesystemBucket(t, context.Background(), t.TempDir())
	bucketClient = block.BucketWithGlobalMarkers(bucketClient)

	// Create blocks.
	createDBBlock(t, bucketClient, "user-1", 10, 20, 2, nil)
	createDBBlock(t, bucketClient, "user-2", 20, 30, 2, nil)

	cfg := BlocksCleanerConfig{
		DeletionDelay:           time.Hour,
		CleanupInterval:         time.Minute,
		CleanupConcurrency:      1,
		DeleteBlocksConcurrency: 1,
	}

	ctx := context.Background()
	logger := log.NewNopLogger()
	reg := prometheus.NewPedanticRegistry()
	cfgProvider := newMockConfigProvider()

	// We will simulate change of "ownUser" by counting number of replies per user. First reply will be "true",
	// all subsequent replies will be false.

	userSeen := map[string]bool{}
	ownUser := func(user string) (bool, error) {
		if userSeen[user] {
			return false, nil
		}
		userSeen[user] = true
		return true, nil
	}

	cleaner := NewBlocksCleaner(cfg, bucketClient, ownUser, cfgProvider, logger, reg)
	require.NoError(t, cleaner.runCleanupWithErr(ctx))

	// Verify that we have seen the users
	require.ElementsMatch(t, []string{"user-1", "user-2"}, cleaner.lastOwnedUsers)

	// But there are no metrics for any user, because we did not in fact clean them.
	assert.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
		# HELP pyroscope_bucket_blocks_count Total number of blocks in the bucket. Includes blocks marked for deletion, but not partial blocks.
		# TYPE pyroscope_bucket_blocks_count gauge
	`),
		"pyroscope_bucket_blocks_count",
	))

	// Running cleanUsers again will see that users are no longer owned.
	require.NoError(t, cleaner.runCleanupWithErr(ctx))
	require.ElementsMatch(t, []string{}, cleaner.lastOwnedUsers)
}

func TestBlocksCleaner_ListBlocksOutsideRetentionPeriod(t *testing.T) {
	bucketClient, _ := objstore_testutil.NewFilesystemBucket(t, context.Background(), t.TempDir())
	bucketClient = block.BucketWithGlobalMarkers(bucketClient)
	ctx := context.Background()
	logger := log.NewNopLogger()

	id1 := createDBBlock(t, bucketClient, "user-1", 5000, 6000, 2, nil)
	id2 := createDBBlock(t, bucketClient, "user-1", 6000, 7000, 2, nil)
	id3 := createDBBlock(t, bucketClient, "user-1", 7000, 8000, 2, nil)

	w := bucketindex.NewUpdater(bucketClient, "user-1", nil, logger)
	idx, _, err := w.UpdateIndex(ctx, nil)
	require.NoError(t, err)

	assert.ElementsMatch(t, []ulid.ULID{id1, id2, id3}, idx.Blocks.GetULIDs())

	// Excessive retention period (wrapping epoch)
	result := listBlocksOutsideRetentionPeriod(idx, time.Unix(10, 0).Add(-time.Hour))
	assert.ElementsMatch(t, []ulid.ULID{}, result.GetULIDs())

	// Normal operation - varying retention period.
	result = listBlocksOutsideRetentionPeriod(idx, time.Unix(6, 0))
	assert.ElementsMatch(t, []ulid.ULID{}, result.GetULIDs())

	result = listBlocksOutsideRetentionPeriod(idx, time.Unix(7, 0))
	assert.ElementsMatch(t, []ulid.ULID{id1}, result.GetULIDs())

	result = listBlocksOutsideRetentionPeriod(idx, time.Unix(8, 0))
	assert.ElementsMatch(t, []ulid.ULID{id1, id2}, result.GetULIDs())

	result = listBlocksOutsideRetentionPeriod(idx, time.Unix(9, 0))
	assert.ElementsMatch(t, []ulid.ULID{id1, id2, id3}, result.GetULIDs())

	// Avoiding redundant marking - blocks already marked for deletion.

	mark1 := &bucketindex.BlockDeletionMark{ID: id1}
	mark2 := &bucketindex.BlockDeletionMark{ID: id2}

	idx.BlockDeletionMarks = bucketindex.BlockDeletionMarks{mark1}

	result = listBlocksOutsideRetentionPeriod(idx, time.Unix(7, 0))
	assert.ElementsMatch(t, []ulid.ULID{}, result.GetULIDs())

	result = listBlocksOutsideRetentionPeriod(idx, time.Unix(8, 0))
	assert.ElementsMatch(t, []ulid.ULID{id2}, result.GetULIDs())

	idx.BlockDeletionMarks = bucketindex.BlockDeletionMarks{mark1, mark2}

	result = listBlocksOutsideRetentionPeriod(idx, time.Unix(7, 0))
	assert.ElementsMatch(t, []ulid.ULID{}, result.GetULIDs())

	result = listBlocksOutsideRetentionPeriod(idx, time.Unix(8, 0))
	assert.ElementsMatch(t, []ulid.ULID{}, result.GetULIDs())

	result = listBlocksOutsideRetentionPeriod(idx, time.Unix(9, 0))
	assert.ElementsMatch(t, []ulid.ULID{id3}, result.GetULIDs())
}

func TestBlocksCleaner_ShouldRemoveBlocksOutsideRetentionPeriod(t *testing.T) {
	bucketClient, _ := objstore_testutil.NewFilesystemBucket(t, context.Background(), t.TempDir())
	bucketClient = block.BucketWithGlobalMarkers(bucketClient)

	ts := func(hours int) int64 {
		return time.Now().Add(time.Duration(hours)*time.Hour).Unix() * 1000
	}

	block1 := createDBBlock(t, bucketClient, "user-1", ts(-10), ts(-8), 2, nil)
	block2 := createDBBlock(t, bucketClient, "user-1", ts(-8), ts(-6), 2, nil)
	block3 := createDBBlock(t, bucketClient, "user-2", ts(-10), ts(-8), 2, nil)
	block4 := createDBBlock(t, bucketClient, "user-2", ts(-8), ts(-6), 2, nil)

	cfg := BlocksCleanerConfig{
		DeletionDelay:           time.Hour,
		CleanupInterval:         time.Minute,
		CleanupConcurrency:      1,
		DeleteBlocksConcurrency: 1,
	}

	ctx := context.Background()
	logger := test.NewTestingLogger(t)
	reg := prometheus.NewPedanticRegistry()
	cfgProvider := newMockConfigProvider()

	cleaner := NewBlocksCleaner(cfg, bucketClient, bucket.AllTenants, cfgProvider, logger, reg)

	assertBlockExists := func(user string, blockID ulid.ULID, expectExists bool) {
		exists, err := bucketClient.Exists(ctx, path.Join(user, "phlaredb/", blockID.String(), block.MetaFilename))
		require.NoError(t, err)
		assert.Equal(t, expectExists, exists)
	}

	// Existing behaviour - retention period disabled.
	{
		cfgProvider.userRetentionPeriods["user-1"] = 0
		cfgProvider.userRetentionPeriods["user-2"] = 0

		require.NoError(t, cleaner.runCleanupWithErr(ctx))
		assertBlockExists("user-1", block1, true)
		assertBlockExists("user-1", block2, true)
		assertBlockExists("user-2", block3, true)
		assertBlockExists("user-2", block4, true)

		assert.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
			# HELP pyroscope_bucket_blocks_count Total number of blocks in the bucket. Includes blocks marked for deletion, but not partial blocks.
			# TYPE pyroscope_bucket_blocks_count gauge
			pyroscope_bucket_blocks_count{compaction_level="1", user="user-1"} 2
			pyroscope_bucket_blocks_count{compaction_level="1", user="user-2"} 2
			# HELP pyroscope_bucket_blocks_marked_for_deletion_count Total number of blocks marked for deletion in the bucket.
			# TYPE pyroscope_bucket_blocks_marked_for_deletion_count gauge
			pyroscope_bucket_blocks_marked_for_deletion_count{user="user-1"} 0
			pyroscope_bucket_blocks_marked_for_deletion_count{user="user-2"} 0
			# HELP pyroscope_compactor_blocks_marked_for_deletion_total Total number of blocks marked for deletion in compactor.
			# TYPE pyroscope_compactor_blocks_marked_for_deletion_total counter
			pyroscope_compactor_blocks_marked_for_deletion_total{reason="partial"} 0
			pyroscope_compactor_blocks_marked_for_deletion_total{reason="retention"} 0
			`),
			"pyroscope_bucket_blocks_count",
			"pyroscope_bucket_blocks_marked_for_deletion_count",
			"pyroscope_compactor_blocks_marked_for_deletion_total",
		))
	}

	// Retention enabled only for a single user, but does nothing.
	{
		cfgProvider.userRetentionPeriods["user-1"] = 9 * time.Hour

		require.NoError(t, cleaner.runCleanupWithErr(ctx))
		assertBlockExists("user-1", block1, true)
		assertBlockExists("user-1", block2, true)
		assertBlockExists("user-2", block3, true)
		assertBlockExists("user-2", block4, true)
	}

	// Retention enabled only for a single user, marking a single block.
	// Note the block won't be deleted yet due to deletion delay.
	{
		cfgProvider.userRetentionPeriods["user-1"] = 7 * time.Hour

		require.NoError(t, cleaner.runCleanupWithErr(ctx))
		assertBlockExists("user-1", block1, true)
		assertBlockExists("user-1", block2, true)
		assertBlockExists("user-2", block3, true)
		assertBlockExists("user-2", block4, true)

		assert.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
			# HELP pyroscope_bucket_blocks_count Total number of blocks in the bucket. Includes blocks marked for deletion, but not partial blocks.
			# TYPE pyroscope_bucket_blocks_count gauge
			pyroscope_bucket_blocks_count{compaction_level="1", user="user-1"} 2
			pyroscope_bucket_blocks_count{compaction_level="1", user="user-2"} 2
			# HELP pyroscope_bucket_blocks_marked_for_deletion_count Total number of blocks marked for deletion in the bucket.
			# TYPE pyroscope_bucket_blocks_marked_for_deletion_count gauge
			pyroscope_bucket_blocks_marked_for_deletion_count{user="user-1"} 1
			pyroscope_bucket_blocks_marked_for_deletion_count{user="user-2"} 0
			# HELP pyroscope_compactor_blocks_marked_for_deletion_total Total number of blocks marked for deletion in compactor.
			# TYPE pyroscope_compactor_blocks_marked_for_deletion_total counter
			pyroscope_compactor_blocks_marked_for_deletion_total{reason="partial"} 0
			pyroscope_compactor_blocks_marked_for_deletion_total{reason="retention"} 1
			`),
			"pyroscope_bucket_blocks_count",
			"pyroscope_bucket_blocks_marked_for_deletion_count",
			"pyroscope_compactor_blocks_marked_for_deletion_total",
		))
	}

	// Marking the block again, before the deletion occurs, should not cause an error.
	{
		require.NoError(t, cleaner.runCleanupWithErr(ctx))
		assertBlockExists("user-1", block1, true)
		assertBlockExists("user-1", block2, true)
		assertBlockExists("user-2", block3, true)
		assertBlockExists("user-2", block4, true)
	}

	// Reduce the deletion delay. Now the block will be deleted.
	{
		cleaner.cfg.DeletionDelay = 0

		require.NoError(t, cleaner.runCleanupWithErr(ctx))
		assertBlockExists("user-1", block1, false)
		assertBlockExists("user-1", block2, true)
		assertBlockExists("user-2", block3, true)
		assertBlockExists("user-2", block4, true)

		assert.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
			# HELP pyroscope_bucket_blocks_count Total number of blocks in the bucket. Includes blocks marked for deletion, but not partial blocks.
			# TYPE pyroscope_bucket_blocks_count gauge
			pyroscope_bucket_blocks_count{compaction_level="1", user="user-1"} 1
			pyroscope_bucket_blocks_count{compaction_level="1", user="user-2"} 2
			# HELP pyroscope_bucket_blocks_marked_for_deletion_count Total number of blocks marked for deletion in the bucket.
			# TYPE pyroscope_bucket_blocks_marked_for_deletion_count gauge
			pyroscope_bucket_blocks_marked_for_deletion_count{user="user-1"} 0
			pyroscope_bucket_blocks_marked_for_deletion_count{user="user-2"} 0
			# HELP pyroscope_compactor_blocks_marked_for_deletion_total Total number of blocks marked for deletion in compactor.
			# TYPE pyroscope_compactor_blocks_marked_for_deletion_total counter
			pyroscope_compactor_blocks_marked_for_deletion_total{reason="partial"} 0
			pyroscope_compactor_blocks_marked_for_deletion_total{reason="retention"} 1
			`),
			"pyroscope_bucket_blocks_count",
			"pyroscope_bucket_blocks_marked_for_deletion_count",
			"pyroscope_compactor_blocks_marked_for_deletion_total",
		))
	}

	// Retention enabled for other user; test deleting multiple blocks.
	{
		cfgProvider.userRetentionPeriods["user-2"] = 5 * time.Hour

		require.NoError(t, cleaner.runCleanupWithErr(ctx))
		assertBlockExists("user-1", block1, false)
		assertBlockExists("user-1", block2, true)
		assertBlockExists("user-2", block3, false)
		assertBlockExists("user-2", block4, false)

		assert.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
			# HELP pyroscope_bucket_blocks_count Total number of blocks in the bucket. Includes blocks marked for deletion, but not partial blocks.
			# TYPE pyroscope_bucket_blocks_count gauge
			pyroscope_bucket_blocks_count{compaction_level="1", user="user-1"} 1
			# HELP pyroscope_bucket_blocks_marked_for_deletion_count Total number of blocks marked for deletion in the bucket.
			# TYPE pyroscope_bucket_blocks_marked_for_deletion_count gauge
			pyroscope_bucket_blocks_marked_for_deletion_count{user="user-1"} 0
			pyroscope_bucket_blocks_marked_for_deletion_count{user="user-2"} 0
			# HELP pyroscope_compactor_blocks_marked_for_deletion_total Total number of blocks marked for deletion in compactor.
			# TYPE pyroscope_compactor_blocks_marked_for_deletion_total counter
			pyroscope_compactor_blocks_marked_for_deletion_total{reason="partial"} 0
			pyroscope_compactor_blocks_marked_for_deletion_total{reason="retention"} 3
			`),
			"pyroscope_bucket_blocks_count",
			"pyroscope_bucket_blocks_marked_for_deletion_count",
			"pyroscope_compactor_blocks_marked_for_deletion_total",
		))
	}
}

func checkBlock(t *testing.T, user string, bucketClient objstore.Bucket, blockID ulid.ULID, metaJSONExists bool, markedForDeletion bool) {
	exists, err := bucketClient.Exists(context.Background(), path.Join(user, "phlaredb/", blockID.String(), block.MetaFilename))
	require.NoError(t, err)
	require.Equal(t, metaJSONExists, exists)

	exists, err = bucketClient.Exists(context.Background(), path.Join(user, "phlaredb/", blockID.String(), block.DeletionMarkFilename))
	require.NoError(t, err)
	require.Equal(t, markedForDeletion, exists)
}

func TestBlocksCleaner_ShouldCleanUpFilesWhenNoMoreBlocksRemain(t *testing.T) {
	bucketClient, _ := objstore_testutil.NewFilesystemBucket(t, context.Background(), t.TempDir())
	bucketClient = block.BucketWithGlobalMarkers(bucketClient)

	const userID = "user-1"
	ctx := context.Background()
	now := time.Now()
	deletionDelay := 12 * time.Hour

	// Create two blocks and mark them for deletion at a time before the deletionDelay
	block1 := createDBBlock(t, bucketClient, userID, 10, 20, 2, nil)
	block2 := createDBBlock(t, bucketClient, userID, 20, 30, 2, nil)

	createDeletionMark(t, bucketClient, userID, block1, now.Add(-deletionDelay).Add(-time.Hour))
	createDeletionMark(t, bucketClient, userID, block2, now.Add(-deletionDelay).Add(-time.Hour))

	checkBlock(t, "user-1", bucketClient, block1, true, true)
	checkBlock(t, "user-1", bucketClient, block2, true, true)

	// Create a deletion mark within the deletionDelay period that won't correspond to any block
	randomULID := ulid.MustNew(ulid.Now(), rand.Reader)
	createDeletionMark(t, bucketClient, userID, randomULID, now.Add(-deletionDelay).Add(time.Hour))
	blockDeletionMarkFile := path.Join(userID, "phlaredb/", block.DeletionMarkFilepath(randomULID))
	exists, err := bucketClient.Exists(ctx, blockDeletionMarkFile)
	require.NoError(t, err)
	assert.True(t, exists)

	cfg := BlocksCleanerConfig{
		DeletionDelay:              deletionDelay,
		CleanupInterval:            time.Minute,
		CleanupConcurrency:         1,
		DeleteBlocksConcurrency:    1,
		NoBlocksFileCleanupEnabled: true,
	}

	logger := test.NewTestingLogger(t)
	reg := prometheus.NewPedanticRegistry()
	cfgProvider := newMockConfigProvider()

	cleaner := NewBlocksCleaner(cfg, bucketClient, bucket.AllTenants, cfgProvider, logger, reg)
	require.NoError(t, cleaner.runCleanupWithErr(ctx))

	// Check bucket index, markers and debug files have been deleted.
	exists, err = bucketClient.Exists(ctx, blockDeletionMarkFile)
	require.NoError(t, err)
	assert.False(t, exists)

	_, err = bucketindex.ReadIndex(ctx, bucketClient, userID, nil, logger)
	require.ErrorIs(t, err, bucketindex.ErrIndexNotFound)
}

func TestBlocksCleaner_ShouldRemovePartialBlocksOutsideDelayPeriod(t *testing.T) {
	bucketClient, _ := objstore_testutil.NewFilesystemBucket(t, context.Background(), t.TempDir())
	bucketClient = block.BucketWithGlobalMarkers(bucketClient)

	ts := func(hours int) int64 {
		return time.Now().Add(time.Duration(hours)*time.Hour).Unix() * 1000
	}

	block1 := createDBBlock(t, bucketClient, "user-1", ts(-10), ts(-8), 2, nil)
	block2 := createDBBlock(t, bucketClient, "user-1", ts(-8), ts(-6), 2, nil)

	cfg := BlocksCleanerConfig{
		DeletionDelay:           time.Hour,
		CleanupInterval:         time.Minute,
		CleanupConcurrency:      1,
		DeleteBlocksConcurrency: 1,
	}

	ctx := context.Background()
	logger := test.NewTestingLogger(t)
	reg := prometheus.NewPedanticRegistry()
	cfgProvider := newMockConfigProvider()

	cleaner := NewBlocksCleaner(cfg, bucketClient, bucket.AllTenants, cfgProvider, logger, reg)

	makeBlockPartial := func(user string, blockID ulid.ULID) {
		err := bucketClient.Delete(ctx, path.Join(user, "phlaredb/", blockID.String(), block.MetaFilename))
		require.NoError(t, err)
	}

	checkBlock(t, "user-1", bucketClient, block1, true, false)
	checkBlock(t, "user-1", bucketClient, block2, true, false)
	makeBlockPartial("user-1", block1)
	checkBlock(t, "user-1", bucketClient, block1, false, false)
	checkBlock(t, "user-1", bucketClient, block2, true, false)

	require.NoError(t, cleaner.cleanUser(ctx, "user-1", logger))

	// check that no blocks were marked for deletion, because deletion delay is set to 0.
	checkBlock(t, "user-1", bucketClient, block1, false, false)
	checkBlock(t, "user-1", bucketClient, block2, true, false)

	// Test that partial block does get marked for deletion
	// The delay time must be very short since these temporary files were just created
	cfgProvider.userPartialBlockDelay["user-1"] = 1 * time.Nanosecond

	require.NoError(t, cleaner.cleanUser(ctx, "user-1", logger))

	// check that first block was marked for deletion (partial block updated far in the past), but not the second one, because it's not partial.
	checkBlock(t, "user-1", bucketClient, block1, false, true)
	checkBlock(t, "user-1", bucketClient, block2, true, false)

	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
			# HELP pyroscope_bucket_blocks_count Total number of blocks in the bucket. Includes blocks marked for deletion, but not partial blocks.
			# TYPE pyroscope_bucket_blocks_count gauge
			pyroscope_bucket_blocks_count{compaction_level="1", user="user-1"} 1
			# HELP pyroscope_bucket_blocks_marked_for_deletion_count Total number of blocks marked for deletion in the bucket.
			# TYPE pyroscope_bucket_blocks_marked_for_deletion_count gauge
			pyroscope_bucket_blocks_marked_for_deletion_count{user="user-1"} 0
			# HELP pyroscope_compactor_blocks_marked_for_deletion_total Total number of blocks marked for deletion in compactor.
			# TYPE pyroscope_compactor_blocks_marked_for_deletion_total counter
			pyroscope_compactor_blocks_marked_for_deletion_total{reason="partial"} 1
			pyroscope_compactor_blocks_marked_for_deletion_total{reason="retention"} 0
			`),
		"pyroscope_bucket_blocks_count",
		"pyroscope_bucket_blocks_marked_for_deletion_count",
		"pyroscope_compactor_blocks_marked_for_deletion_total",
	))
}

func TestBlocksCleaner_ShouldNotRemovePartialBlocksInsideDelayPeriod(t *testing.T) {
	bucketClient, _ := objstore_testutil.NewFilesystemBucket(t, context.Background(), t.TempDir())
	bucketClient = block.BucketWithGlobalMarkers(bucketClient)

	ts := func(hours int) int64 {
		return time.Now().Add(time.Duration(hours)*time.Hour).Unix() * 1000
	}

	block1 := createDBBlock(t, bucketClient, "user-1", ts(-10), ts(-8), 2, nil)
	block2 := createDBBlock(t, bucketClient, "user-2", ts(-8), ts(-6), 2, nil)

	cfg := BlocksCleanerConfig{
		DeletionDelay:           time.Hour,
		CleanupInterval:         time.Minute,
		CleanupConcurrency:      1,
		DeleteBlocksConcurrency: 1,
	}

	ctx := context.Background()
	logger := test.NewTestingLogger(t)
	reg := prometheus.NewPedanticRegistry()
	cfgProvider := newMockConfigProvider()

	cleaner := NewBlocksCleaner(cfg, bucketClient, bucket.AllTenants, cfgProvider, logger, reg)

	makeBlockPartial := func(user string, blockID ulid.ULID) {
		err := bucketClient.Delete(ctx, path.Join(user, "phlaredb/", blockID.String(), block.MetaFilename))
		require.NoError(t, err)
	}

	corruptMeta := func(user string, blockID ulid.ULID) {
		err := bucketClient.Upload(ctx, path.Join(user, "phlaredb/", blockID.String(), block.MetaFilename), strings.NewReader("corrupted file contents"))
		require.NoError(t, err)
	}

	checkBlock(t, "user-1", bucketClient, block1, true, false)
	checkBlock(t, "user-2", bucketClient, block2, true, false)

	makeBlockPartial("user-1", block1)
	corruptMeta("user-2", block2)

	checkBlock(t, "user-1", bucketClient, block1, false, false)
	checkBlock(t, "user-2", bucketClient, block2, true, false)

	// Set partial block delay such that block will not be marked for deletion
	// The comparison is based on inode modification time, so anything more than very recent (< 1 second) won't be
	// out of range
	cfgProvider.userPartialBlockDelay["user-1"] = 1 * time.Hour
	cfgProvider.userPartialBlockDelay["user-2"] = 1 * time.Nanosecond

	require.NoError(t, cleaner.cleanUser(ctx, "user-1", logger))
	checkBlock(t, "user-1", bucketClient, block1, false, false) // This block was updated too recently, so we don't mark it for deletion just yet.
	checkBlock(t, "user-2", bucketClient, block2, true, false)  // No change for user-2.

	require.NoError(t, cleaner.cleanUser(ctx, "user-2", logger))
	checkBlock(t, "user-1", bucketClient, block1, false, false) // No change for user-1
	checkBlock(t, "user-2", bucketClient, block2, true, false)  // Block with corrupted meta is NOT marked for deletion.

	// The pyroscope_compactor_blocks_marked_for_deletion_total{reason="partial"} counter should be zero since for user-1
	// the time since modification is shorter than the delay, and for user-2, the metadata is corrupted but the file
	// is still present in the bucket so the block is not partial
	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
			# HELP pyroscope_bucket_blocks_marked_for_deletion_count Total number of blocks marked for deletion in the bucket.
			# TYPE pyroscope_bucket_blocks_marked_for_deletion_count gauge
			pyroscope_bucket_blocks_marked_for_deletion_count{user="user-1"} 0
			pyroscope_bucket_blocks_marked_for_deletion_count{user="user-2"} 0
			# HELP pyroscope_compactor_blocks_marked_for_deletion_total Total number of blocks marked for deletion in compactor.
			# TYPE pyroscope_compactor_blocks_marked_for_deletion_total counter
			pyroscope_compactor_blocks_marked_for_deletion_total{reason="partial"} 0
			pyroscope_compactor_blocks_marked_for_deletion_total{reason="retention"} 0
			`),
		"pyroscope_bucket_blocks_count",
		"pyroscope_bucket_blocks_marked_for_deletion_count",
		"pyroscope_compactor_blocks_marked_for_deletion_total",
	))
}

func TestBlocksCleaner_ShouldNotRemovePartialBlocksIfConfiguredDelayIsInvalid(t *testing.T) {
	ctx := context.Background()
	reg := prometheus.NewPedanticRegistry()
	logs := &concurrency.SyncBuffer{}
	logger := log.NewLogfmtLogger(logs)

	bucketClient, _ := objstore_testutil.NewFilesystemBucket(t, context.Background(), t.TempDir())
	bucketClient = block.BucketWithGlobalMarkers(bucketClient)

	ts := func(hours int) int64 {
		return time.Now().Add(time.Duration(hours)*time.Hour).Unix() * 1000
	}

	// Create a partial block.
	block1 := createDBBlock(t, bucketClient, "user-1", ts(-10), ts(-8), 2, nil)
	err := bucketClient.Delete(ctx, path.Join("user-1", "phlaredb/", block1.String(), block.MetaFilename))
	require.NoError(t, err)

	cfg := BlocksCleanerConfig{
		DeletionDelay:           time.Hour,
		CleanupInterval:         time.Minute,
		CleanupConcurrency:      1,
		DeleteBlocksConcurrency: 1,
	}

	// Configure an invalid delay.
	cfgProvider := newMockConfigProvider()
	cfgProvider.userPartialBlockDelay["user-1"] = 0
	cfgProvider.userPartialBlockDelayInvalid["user-1"] = true

	// Pre-condition check: block should be partial and not being marked for deletion.
	checkBlock(t, "user-1", bucketClient, block1, false, false)

	// Run the cleanup.
	cleaner := NewBlocksCleaner(cfg, bucketClient, bucket.AllTenants, cfgProvider, logger, reg)
	require.NoError(t, cleaner.cleanUser(ctx, "user-1", logger))

	// Ensure the block has NOT been marked for deletion.
	checkBlock(t, "user-1", bucketClient, block1, false, false)
	assert.Contains(t, logs.String(), "partial blocks deletion has been disabled for tenant because the delay has been set lower than the minimum value allowed")

	require.NoError(t, testutil.GatherAndCompare(reg, strings.NewReader(`
			# HELP pyroscope_bucket_blocks_marked_for_deletion_count Total number of blocks marked for deletion in the bucket.
			# TYPE pyroscope_bucket_blocks_marked_for_deletion_count gauge
			pyroscope_bucket_blocks_marked_for_deletion_count{user="user-1"} 0
			# HELP pyroscope_compactor_blocks_marked_for_deletion_total Total number of blocks marked for deletion in compactor.
			# TYPE pyroscope_compactor_blocks_marked_for_deletion_total counter
			pyroscope_compactor_blocks_marked_for_deletion_total{reason="partial"} 0
			pyroscope_compactor_blocks_marked_for_deletion_total{reason="retention"} 0
			`),
		"pyroscope_bucket_blocks_count",
		"pyroscope_bucket_blocks_marked_for_deletion_count",
		"pyroscope_compactor_blocks_marked_for_deletion_total",
	))
}

func TestStalePartialBlockLastModifiedTime(t *testing.T) {
	dir := t.TempDir()
	b, _ := objstore_testutil.NewFilesystemBucket(t, context.Background(), dir)

	const tenantId = "user"

	objectTime := time.Now().Add(-1 * time.Hour).Truncate(time.Second) // ignore milliseconds, as not all filesystems store them.
	blockID := createDBBlock(t, b, tenantId, objectTime.UnixMilli(), time.Now().UnixMilli(), 2, nil)
	err := filepath.Walk(filepath.Join(dir, tenantId, "phlaredb/", blockID.String()), func(path string, info os.FileInfo, err error) error {
		require.NoError(t, err)
		require.NoError(t, os.Chtimes(path, objectTime, objectTime))
		return nil
	})
	require.NoError(t, err)

	userBucket := objstore.NewTenantBucketClient(tenantId, b, nil)

	emptyBlockID := ulid.ULID{}
	require.NotEqual(t, blockID, emptyBlockID)
	empty := true
	err = userBucket.Iter(context.Background(), emptyBlockID.String(), func(_ string) error {
		empty = false
		return nil
	})
	require.NoError(t, err)
	require.True(t, empty)

	testCases := []struct {
		name                 string
		blockID              ulid.ULID
		cutoff               time.Time
		expectedLastModified time.Time
	}{
		{name: "no objects", blockID: emptyBlockID, cutoff: objectTime, expectedLastModified: time.Time{}},
		{name: "objects newer than delay cutoff", blockID: blockID, cutoff: objectTime.Add(-1 * time.Second), expectedLastModified: time.Time{}},
		{name: "objects equal to delay cutoff", blockID: blockID, cutoff: objectTime, expectedLastModified: objectTime},
		{name: "objects older than delay cutoff", blockID: blockID, cutoff: objectTime.Add(1 * time.Second), expectedLastModified: objectTime},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			lastModified, err := stalePartialBlockLastModifiedTime(context.Background(), tc.blockID, userBucket, tc.cutoff)
			require.NoError(t, err)
			require.Equal(t, tc.expectedLastModified, lastModified)
		})
	}
}

type mockBucketFailure struct {
	objstore.Bucket

	DeleteFailures []string
}

func (m *mockBucketFailure) Delete(ctx context.Context, name string) error {
	if util.StringsContain(m.DeleteFailures, name) {
		return errors.New("mocked delete failure")
	}
	return m.Bucket.Delete(ctx, name)
}

type mockConfigProvider struct {
	userRetentionPeriods         map[string]time.Duration
	splitAndMergeShards          map[string]int
	instancesShardSize           map[string]int
	splitGroups                  map[string]int
	splitAndMergeStageSize       map[string]int
	blockUploadEnabled           map[string]bool
	blockUploadValidationEnabled map[string]bool
	blockUploadMaxBlockSizeBytes map[string]int64
	userPartialBlockDelay        map[string]time.Duration
	userPartialBlockDelayInvalid map[string]bool
	verifyChunks                 map[string]bool
	downsamplerEnabled           map[string]bool
}

func newMockConfigProvider() *mockConfigProvider {
	return &mockConfigProvider{
		userRetentionPeriods:         make(map[string]time.Duration),
		splitAndMergeShards:          make(map[string]int),
		splitGroups:                  make(map[string]int),
		splitAndMergeStageSize:       make(map[string]int),
		blockUploadEnabled:           make(map[string]bool),
		blockUploadValidationEnabled: make(map[string]bool),
		blockUploadMaxBlockSizeBytes: make(map[string]int64),
		userPartialBlockDelay:        make(map[string]time.Duration),
		userPartialBlockDelayInvalid: make(map[string]bool),
		verifyChunks:                 make(map[string]bool),
		downsamplerEnabled:           make(map[string]bool),
	}
}

func (m *mockConfigProvider) CompactorBlocksRetentionPeriod(user string) time.Duration {
	if result, ok := m.userRetentionPeriods[user]; ok {
		return result
	}
	return 0
}

func (m *mockConfigProvider) CompactorSplitAndMergeShards(user string) int {
	if result, ok := m.splitAndMergeShards[user]; ok {
		return result
	}
	return 0
}

func (m *mockConfigProvider) CompactorSplitAndMergeStageSize(user string) int {
	if result, ok := m.splitAndMergeStageSize[user]; ok {
		return result
	}
	return 0
}

func (m *mockConfigProvider) CompactorSplitGroups(user string) int {
	if result, ok := m.splitGroups[user]; ok {
		return result
	}
	return 0
}

func (m *mockConfigProvider) CompactorTenantShardSize(user string) int {
	if result, ok := m.instancesShardSize[user]; ok {
		return result
	}
	return 0
}

func (m *mockConfigProvider) CompactorBlockUploadEnabled(tenantID string) bool {
	return m.blockUploadEnabled[tenantID]
}

func (m *mockConfigProvider) CompactorBlockUploadValidationEnabled(tenantID string) bool {
	return m.blockUploadValidationEnabled[tenantID]
}

func (m *mockConfigProvider) CompactorPartialBlockDeletionDelay(user string) (time.Duration, bool) {
	return m.userPartialBlockDelay[user], !m.userPartialBlockDelayInvalid[user]
}

func (m *mockConfigProvider) CompactorBlockUploadVerifyChunks(tenantID string) bool {
	return m.verifyChunks[tenantID]
}

func (m *mockConfigProvider) CompactorBlockUploadMaxBlockSizeBytes(user string) int64 {
	return m.blockUploadMaxBlockSizeBytes[user]
}

func (m *mockConfigProvider) CompactorDownsamplerEnabled(user string) bool {
	return m.downsamplerEnabled[user]
}

func (m *mockConfigProvider) S3SSEType(string) string {
	return ""
}

func (m *mockConfigProvider) S3SSEKMSKeyID(string) string {
	return ""
}

func (m *mockConfigProvider) S3SSEKMSEncryptionContext(string) string {
	return ""
}

func (c *BlocksCleaner) runCleanupWithErr(ctx context.Context) error {
	allUsers, isDeleted, err := c.refreshOwnedUsers(ctx)
	if err != nil {
		return err
	}

	return c.cleanUsers(ctx, allUsers, isDeleted, log.NewNopLogger())
}

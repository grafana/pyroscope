// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/storage/tsdb/bucketindex/updater_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package bucketindex

import (
	"bytes"
	"context"
	"path"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/objstore"
	objstore_testutil "github.com/grafana/pyroscope/pkg/objstore/testutil"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	block_testutil "github.com/grafana/pyroscope/pkg/phlaredb/block/testutil"
	"github.com/grafana/pyroscope/pkg/phlaredb/sharding"
)

func TestUpdater_UpdateIndex(t *testing.T) {
	const userID = "user-1"

	ctx := context.Background()
	logger := log.NewNopLogger()
	bkt, _ := objstore_testutil.NewFilesystemBucket(t, ctx, t.TempDir())

	// Generate the initial index.
	bkt = block.BucketWithGlobalMarkers(bkt)
	block1 := block_testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 10, 20, nil)
	block_testutil.MockNoCompactMark(t, bkt, userID, block1) // no-compact mark is ignored by bucket index updater.
	block2 := block_testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 20, 30, map[string]string{sharding.CompactorShardIDLabel: "1_of_5"})
	block2Mark := block_testutil.MockStorageDeletionMark(t, bkt, userID, block2)

	w := NewUpdater(bkt, userID, nil, logger)
	returnedIdx, _, err := w.UpdateIndex(ctx, nil)
	require.NoError(t, err)
	assertBucketIndexEqual(t, returnedIdx, bkt, userID,
		[]block.Meta{block1, block2},
		[]*block.DeletionMark{block2Mark})

	// Create new blocks, and update the index.
	block3 := block_testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 30, 40, map[string]string{"aaa": "bbb"})
	block4 := block_testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 40, 50, map[string]string{sharding.CompactorShardIDLabel: "2_of_5"})
	block4Mark := block_testutil.MockStorageDeletionMark(t, bkt, userID, block4)

	returnedIdx, _, err = w.UpdateIndex(ctx, returnedIdx)
	require.NoError(t, err)
	assertBucketIndexEqual(t, returnedIdx, bkt, userID,
		[]block.Meta{block1, block2, block3, block4},
		[]*block.DeletionMark{block2Mark, block4Mark})

	// Hard delete a block and update the index.
	require.NoError(t, block.Delete(ctx, log.NewNopLogger(), objstore.NewTenantBucketClient(userID, bkt, nil), block2.ULID))

	returnedIdx, _, err = w.UpdateIndex(ctx, returnedIdx)
	require.NoError(t, err)
	assertBucketIndexEqual(t, returnedIdx, bkt, userID,
		[]block.Meta{block1, block3, block4},
		[]*block.DeletionMark{block4Mark})
}

func TestUpdater_UpdateIndex_ShouldSkipPartialBlocks(t *testing.T) {
	const userID = "user-1"

	ctx := context.Background()
	logger := log.NewNopLogger()

	bkt, _ := objstore_testutil.NewFilesystemBucket(t, ctx, t.TempDir())

	// Mock some blocks in the storage.
	bkt = block.BucketWithGlobalMarkers(bkt)
	block1 := block_testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 10, 20, map[string]string{"hello": "world"})
	block2 := block_testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 20, 30, map[string]string{sharding.CompactorShardIDLabel: "3_of_10"})
	block3 := block_testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 30, 40, nil)
	block2Mark := block_testutil.MockStorageDeletionMark(t, bkt, userID, block2)

	// No compact marks are ignored by bucket index.
	block_testutil.MockNoCompactMark(t, bkt, userID, block3)

	// Delete a block's meta.json to simulate a partial block.
	require.NoError(t, bkt.Delete(ctx, path.Join(userID, "phlaredb/", block3.ULID.String(), block.MetaFilename)))

	w := NewUpdater(bkt, userID, nil, logger)
	idx, partials, err := w.UpdateIndex(ctx, nil)
	require.NoError(t, err)
	assertBucketIndexEqual(t, idx, bkt, userID,
		[]block.Meta{block1, block2},
		[]*block.DeletionMark{block2Mark})

	assert.Len(t, partials, 1)
	assert.True(t, errors.Is(partials[block3.ULID], ErrBlockMetaNotFound))
}

func TestUpdater_UpdateIndex_ShouldSkipBlocksWithCorruptedMeta(t *testing.T) {
	const userID = "user-1"

	ctx := context.Background()
	logger := log.NewNopLogger()

	bkt, _ := objstore_testutil.NewFilesystemBucket(t, ctx, t.TempDir())

	// Mock some blocks in the storage.
	bkt = block.BucketWithGlobalMarkers(bkt)
	block1 := block_testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 10, 20, nil)
	block2 := block_testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 20, 30, map[string]string{sharding.CompactorShardIDLabel: "55_of_64"})
	block3 := block_testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 30, 40, nil)
	block2Mark := block_testutil.MockStorageDeletionMark(t, bkt, userID, block2)

	// Overwrite a block's meta.json with invalid data.
	require.NoError(t, bkt.Upload(ctx, path.Join(userID, "phlaredb/", block3.ULID.String(), block.MetaFilename), bytes.NewReader([]byte("invalid!}"))))

	w := NewUpdater(bkt, userID, nil, logger)
	idx, partials, err := w.UpdateIndex(ctx, nil)
	require.NoError(t, err)
	assertBucketIndexEqual(t, idx, bkt, userID,
		[]block.Meta{block1, block2},
		[]*block.DeletionMark{block2Mark})

	assert.Len(t, partials, 1)
	assert.True(t, errors.Is(partials[block3.ULID], ErrBlockMetaCorrupted))
}

func TestUpdater_UpdateIndex_ShouldSkipCorruptedDeletionMarks(t *testing.T) {
	const userID = "user-1"

	ctx := context.Background()
	logger := log.NewNopLogger()

	bkt, _ := objstore_testutil.NewFilesystemBucket(t, ctx, t.TempDir())

	// Mock some blocks in the storage.
	bkt = block.BucketWithGlobalMarkers(bkt)
	block1 := block_testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 10, 20, nil)
	block2 := block_testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 20, 30, nil)
	block3 := block_testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 30, 40, map[string]string{sharding.CompactorShardIDLabel: "2_of_7"})
	block2Mark := block_testutil.MockStorageDeletionMark(t, bkt, userID, block2)

	// Overwrite a block's deletion-mark.json with invalid data.
	require.NoError(t, bkt.Upload(ctx, path.Join(userID, "phlaredb/", block2Mark.ID.String(), block.DeletionMarkFilename), bytes.NewReader([]byte("invalid!}"))))

	w := NewUpdater(bkt, userID, nil, logger)
	idx, partials, err := w.UpdateIndex(ctx, nil)
	require.NoError(t, err)
	assertBucketIndexEqual(t, idx, bkt, userID,
		[]block.Meta{block1, block2, block3},
		[]*block.DeletionMark{})
	assert.Empty(t, partials)
}

func TestUpdater_UpdateIndex_NoTenantInTheBucket(t *testing.T) {
	const userID = "user-1"

	ctx := context.Background()
	bkt, _ := objstore_testutil.NewFilesystemBucket(t, ctx, t.TempDir())

	for _, oldIdx := range []*Index{nil, {}} {
		w := NewUpdater(bkt, userID, nil, log.NewNopLogger())
		idx, partials, err := w.UpdateIndex(ctx, oldIdx)

		require.NoError(t, err)
		assert.Equal(t, IndexVersion3, idx.Version)
		assert.InDelta(t, time.Now().Unix(), idx.UpdatedAt, 2)
		assert.Len(t, idx.Blocks, 0)
		assert.Len(t, idx.BlockDeletionMarks, 0)
		assert.Empty(t, partials)
	}
}

func TestUpdater_UpdateIndexFromVersion1ToVersion2(t *testing.T) {
	const userID = "user-2"

	ctx := context.Background()
	logger := log.NewNopLogger()

	bkt, _ := objstore_testutil.NewFilesystemBucket(t, ctx, t.TempDir())

	// Generate blocks with compactor shard ID.
	bkt = block.BucketWithGlobalMarkers(bkt)
	block1 := block_testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 10, 20, map[string]string{sharding.CompactorShardIDLabel: "1_of_4"})
	block2 := block_testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 20, 30, map[string]string{sharding.CompactorShardIDLabel: "3_of_4"})

	block1WithoutCompactorShardID := block1
	block1WithoutCompactorShardID.Labels = nil

	block2WithoutCompactorShardID := block2
	block2WithoutCompactorShardID.Labels = nil

	// Double check that original block1 and block2 still have compactor shards set.
	require.Equal(t, "1_of_4", block1.Labels[sharding.CompactorShardIDLabel])
	require.Equal(t, "3_of_4", block2.Labels[sharding.CompactorShardIDLabel])

	// Generate index (this produces V2 index, with compactor shard IDs).
	w := NewUpdater(bkt, userID, nil, logger)
	returnedIdx, _, err := w.UpdateIndex(ctx, nil)
	require.NoError(t, err)
	assertBucketIndexEqual(t, returnedIdx, bkt, userID,
		[]block.Meta{block1, block2},
		[]*block.DeletionMark{})

	// Now remove Compactor Shard ID from index.
	for _, b := range returnedIdx.Blocks {
		b.CompactorShardID = ""
	}

	// Try to update existing index. Since we didn't change the version, updater will reuse the index, and not update CompactorShardID field.
	returnedIdx, _, err = w.UpdateIndex(ctx, returnedIdx)
	require.NoError(t, err)
	assertBucketIndexEqual(t, returnedIdx, bkt, userID,
		[]block.Meta{block1WithoutCompactorShardID, block2WithoutCompactorShardID}, // No compactor shards in bucket index.
		[]*block.DeletionMark{})

	// Now set index version to old version 1. Rerunning updater should rebuild index from scratch.
	returnedIdx.Version = IndexVersion1

	returnedIdx, _, err = w.UpdateIndex(ctx, returnedIdx)
	require.NoError(t, err)
	assertBucketIndexEqual(t, returnedIdx, bkt, userID,
		[]block.Meta{block1, block2}, // Compactor shards are back.
		[]*block.DeletionMark{})
}

func TestUpdater_UpdateIndexFromVersion2ToVersion3(t *testing.T) {
	const userID = "user-2"

	ctx := context.Background()
	logger := log.NewNopLogger()

	bkt, _ := objstore_testutil.NewFilesystemBucket(t, ctx, t.TempDir())

	// Generate blocks with compactor shard ID.
	bkt = block.BucketWithGlobalMarkers(bkt)
	block1 := block_testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 10, 20, map[string]string{sharding.CompactorShardIDLabel: "1_of_4"})
	block2 := block_testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 20, 30, map[string]string{sharding.CompactorShardIDLabel: "3_of_4"})

	block1WithoutCompactionLevel := block1
	block1WithoutCompactionLevel.Compaction.Level = 0

	block2WithoutCompactionLevel := block2
	block2WithoutCompactionLevel.Compaction.Level = 0

	// Double check that original block1 and block2 still have compactor shards set.
	require.Equal(t, 1, block1.Compaction.Level)
	require.Equal(t, 1, block2.Compaction.Level)

	// Generate index (this produces V3 index, with compaction levels).
	w := NewUpdater(bkt, userID, nil, logger)
	returnedIdx, _, err := w.UpdateIndex(ctx, nil)
	require.NoError(t, err)
	assertBucketIndexEqual(t, returnedIdx, bkt, userID,
		[]block.Meta{block1, block2},
		[]*block.DeletionMark{})

	// Now remove compaction levels from index.
	for _, b := range returnedIdx.Blocks {
		b.CompactionLevel = 0
	}

	// Try to update existing index. Since we didn't change the version, updater will reuse the index, and not update compaction levels.
	returnedIdx, _, err = w.UpdateIndex(ctx, returnedIdx)
	require.NoError(t, err)
	assertBucketIndexEqual(t, returnedIdx, bkt, userID,
		[]block.Meta{block1WithoutCompactionLevel, block2WithoutCompactionLevel}, // No compactor shards in bucket index.
		[]*block.DeletionMark{})

	// Now set index version to old version 2. Rerunning updater should rebuild index from scratch.
	returnedIdx.Version = IndexVersion2

	returnedIdx, _, err = w.UpdateIndex(ctx, returnedIdx)
	require.NoError(t, err)
	assertBucketIndexEqual(t, returnedIdx, bkt, userID,
		[]block.Meta{block1, block2}, // Compaction levels are back.
		[]*block.DeletionMark{})
}

func getBlockUploadedAt(t testing.TB, bkt objstore.Bucket, userID string, blockID ulid.ULID) int64 {
	metaFile := path.Join(userID, "phlaredb/", blockID.String(), block.MetaFilename)

	attrs, err := bkt.Attributes(context.Background(), metaFile)
	require.NoError(t, err)

	return attrs.LastModified.Unix()
}

func assertBucketIndexEqual(t testing.TB, idx *Index, bkt objstore.Bucket, userID string, expectedBlocks []block.Meta, expectedDeletionMarks []*block.DeletionMark) {
	assert.Equal(t, IndexVersion3, idx.Version)
	assert.InDelta(t, time.Now().Unix(), idx.UpdatedAt, 2)

	// Build the list of expected block index entries.
	var expectedBlockEntries []*Block
	for _, b := range expectedBlocks {
		expectedBlockEntries = append(expectedBlockEntries, &Block{
			ID:               b.ULID,
			MinTime:          b.MinTime,
			MaxTime:          b.MaxTime,
			UploadedAt:       getBlockUploadedAt(t, bkt, userID, b.ULID),
			CompactorShardID: b.Labels[sharding.CompactorShardIDLabel],
			CompactionLevel:  b.Compaction.Level,
		})
	}

	assert.ElementsMatch(t, expectedBlockEntries, idx.Blocks)

	// Build the list of expected block deletion mark index entries.
	var expectedMarkEntries []*BlockDeletionMark
	for _, m := range expectedDeletionMarks {
		expectedMarkEntries = append(expectedMarkEntries, &BlockDeletionMark{
			ID:           m.ID,
			DeletionTime: m.DeletionTime,
		})
	}

	assert.ElementsMatch(t, expectedMarkEntries, idx.BlockDeletionMarks)
}

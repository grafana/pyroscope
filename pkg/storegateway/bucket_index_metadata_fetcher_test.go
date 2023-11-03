// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/storegateway/bucket_index_metadata_fetcher_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package storegateway

import (
	"bytes"
	"context"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/concurrency"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/pkg/objstore"
	objstore_testutil "github.com/grafana/pyroscope/pkg/objstore/testutil"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/phlaredb/bucketindex"
)

func TestBucketIndexMetadataFetcher_Fetch(t *testing.T) {
	const userID = "user-1"

	ctx := context.Background()

	bkt, _ := objstore_testutil.NewFilesystemBucket(t, ctx, t.TempDir())
	reg := prometheus.NewPedanticRegistry()
	now := time.Now()
	logs := &concurrency.SyncBuffer{}
	logger := log.NewLogfmtLogger(logs)

	// Create a bucket index.
	block1 := &bucketindex.Block{ID: ulid.MustNew(1, nil)}
	block2 := &bucketindex.Block{ID: ulid.MustNew(2, nil)}
	block3 := &bucketindex.Block{ID: ulid.MustNew(3, nil)}
	block4 := &bucketindex.Block{ID: ulid.MustNew(4, nil), MinTime: model.Time(now.Add(-30 * time.Minute).UnixMilli())} // Has most-recent data, to be ignored by minTimeMetaFilter.

	mark1 := &bucketindex.BlockDeletionMark{ID: block1.ID, DeletionTime: now.Add(-time.Hour).Unix()}     // Below the ignore delay threshold.
	mark2 := &bucketindex.BlockDeletionMark{ID: block2.ID, DeletionTime: now.Add(-3 * time.Hour).Unix()} // Above the ignore delay threshold.

	require.NoError(t, bucketindex.WriteIndex(ctx, bkt, userID, nil, &bucketindex.Index{
		Version:            bucketindex.IndexVersion1,
		Blocks:             bucketindex.Blocks{block1, block2, block3, block4},
		BlockDeletionMarks: bucketindex.BlockDeletionMarks{mark1, mark2},
		UpdatedAt:          now.Unix(),
	}))

	// Create a metadata fetcher with filters.
	filters := []block.MetadataFilter{
		NewIgnoreDeletionMarkFilter(logger, objstore.NewTenantBucketClient(userID, bkt, nil), 2*time.Hour, 1),
		newMinTimeMetaFilter(1 * time.Hour),
	}

	fetcher := NewBucketIndexMetadataFetcher(userID, bkt, nil, logger, reg, filters)
	metas, partials, err := fetcher.Fetch(ctx)
	require.NoError(t, err)
	assert.Equal(t, map[ulid.ULID]*block.Meta{
		block1.ID: block1.Meta(),
		block3.ID: block3.Meta(),
	}, metas)
	assert.Empty(t, partials)
	assert.Empty(t, logs)

	assert.NoError(t, testutil.GatherAndCompare(reg, bytes.NewBufferString(`
		# HELP blocks_meta_sync_failures_total Total blocks metadata synchronization failures
		# TYPE blocks_meta_sync_failures_total counter
		blocks_meta_sync_failures_total 0

		# HELP blocks_meta_synced Number of block metadata synced
		# TYPE blocks_meta_synced gauge
		blocks_meta_synced{state="corrupted-bucket-index"} 0
		blocks_meta_synced{state="corrupted-meta-json"} 0
		blocks_meta_synced{state="duplicate"} 0
		blocks_meta_synced{state="failed"} 0
		blocks_meta_synced{state="label-excluded"} 0
		blocks_meta_synced{state="loaded"} 2
		blocks_meta_synced{state="marked-for-deletion"} 1
		blocks_meta_synced{state="marked-for-no-compact"} 0
		blocks_meta_synced{state="no-bucket-index"} 0
		blocks_meta_synced{state="no-meta-json"} 0
		blocks_meta_synced{state="time-excluded"} 0
		blocks_meta_synced{state="min-time-excluded"} 1

		# HELP blocks_meta_syncs_total Total blocks metadata synchronization attempts
		# TYPE blocks_meta_syncs_total counter
		blocks_meta_syncs_total 1
	`),
		"blocks_meta_sync_failures_total",
		"blocks_meta_synced",
		"blocks_meta_syncs_total",
	))
}

func TestBucketIndexMetadataFetcher_Fetch_NoBucketIndex(t *testing.T) {
	const userID = "user-1"

	ctx := context.Background()
	bkt, _ := objstore_testutil.NewFilesystemBucket(t, ctx, t.TempDir())
	reg := prometheus.NewPedanticRegistry()
	logs := &concurrency.SyncBuffer{}
	logger := log.NewLogfmtLogger(logs)

	fetcher := NewBucketIndexMetadataFetcher(userID, bkt, nil, logger, reg, nil)
	metas, partials, err := fetcher.Fetch(ctx)
	require.NoError(t, err)
	assert.Empty(t, metas)
	assert.Empty(t, partials)
	assert.Contains(t, logs.String(), "no bucket index found, falling back to fetching directly from bucket")

	assert.NoError(t, testutil.GatherAndCompare(reg, bytes.NewBufferString(`
		# HELP blocks_meta_sync_failures_total Total blocks metadata synchronization failures
		# TYPE blocks_meta_sync_failures_total counter
		blocks_meta_sync_failures_total 0

		# HELP blocks_meta_synced Number of block metadata synced
		# TYPE blocks_meta_synced gauge
		blocks_meta_synced{state="corrupted-bucket-index"} 0
		blocks_meta_synced{state="corrupted-meta-json"} 0
		blocks_meta_synced{state="duplicate"} 0
		blocks_meta_synced{state="failed"} 0
		blocks_meta_synced{state="label-excluded"} 0
		blocks_meta_synced{state="loaded"} 0
		blocks_meta_synced{state="marked-for-deletion"} 0
		blocks_meta_synced{state="marked-for-no-compact"} 0
		blocks_meta_synced{state="no-bucket-index"} 1
		blocks_meta_synced{state="no-meta-json"} 0
		blocks_meta_synced{state="time-excluded"} 0
		blocks_meta_synced{state="min-time-excluded"} 0

		# HELP blocks_meta_syncs_total Total blocks metadata synchronization attempts
		# TYPE blocks_meta_syncs_total counter
		blocks_meta_syncs_total 1
	`),
		"blocks_meta_sync_failures_total",
		"blocks_meta_synced",
		"blocks_meta_syncs_total",
	))
}

func TestBucketIndexMetadataFetcher_Fetch_CorruptedBucketIndex(t *testing.T) {
	const userID = "user-1"

	ctx := context.Background()

	bkt, _ := objstore_testutil.NewFilesystemBucket(t, ctx, t.TempDir())
	reg := prometheus.NewPedanticRegistry()
	logs := &concurrency.SyncBuffer{}
	logger := log.NewLogfmtLogger(logs)

	// Upload a corrupted bucket index.
	require.NoError(t, bkt.Upload(ctx, path.Join(userID, "phlaredb/", bucketindex.IndexCompressedFilename), strings.NewReader("invalid}!")))

	fetcher := NewBucketIndexMetadataFetcher(userID, bkt, nil, logger, reg, nil)
	metas, partials, err := fetcher.Fetch(ctx)
	require.NoError(t, err)
	assert.Empty(t, metas)
	assert.Empty(t, partials)
	assert.Regexp(t, "corrupted bucket index found", logs)

	assert.NoError(t, testutil.GatherAndCompare(reg, bytes.NewBufferString(`
		# HELP blocks_meta_sync_failures_total Total blocks metadata synchronization failures
		# TYPE blocks_meta_sync_failures_total counter
		blocks_meta_sync_failures_total 0

		# HELP blocks_meta_synced Number of block metadata synced
		# TYPE blocks_meta_synced gauge
		blocks_meta_synced{state="corrupted-bucket-index"} 1
		blocks_meta_synced{state="corrupted-meta-json"} 0
		blocks_meta_synced{state="duplicate"} 0
		blocks_meta_synced{state="failed"} 0
		blocks_meta_synced{state="label-excluded"} 0
		blocks_meta_synced{state="loaded"} 0
		blocks_meta_synced{state="marked-for-deletion"} 0
		blocks_meta_synced{state="marked-for-no-compact"} 0
		blocks_meta_synced{state="no-bucket-index"} 0
		blocks_meta_synced{state="no-meta-json"} 0
		blocks_meta_synced{state="time-excluded"} 0
		blocks_meta_synced{state="min-time-excluded"} 0

		# HELP blocks_meta_syncs_total Total blocks metadata synchronization attempts
		# TYPE blocks_meta_syncs_total counter
		blocks_meta_syncs_total 1
	`),
		"blocks_meta_sync_failures_total",
		"blocks_meta_synced",
		"blocks_meta_syncs_total",
	))
}

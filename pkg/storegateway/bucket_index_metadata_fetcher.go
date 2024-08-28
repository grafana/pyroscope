// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/storegateway/bucket_index_metadata_fetcher.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package storegateway

import (
	"context"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/phlaredb/bucketindex"
)

const (
	corruptedBucketIndex     = "corrupted-bucket-index"
	noBucketIndex            = "no-bucket-index"
	bucketIndexOlderThanHour = "bucket-index-older-than-hour"
)

// BucketIndexMetadataFetcher is a Thanos MetadataFetcher implementation leveraging on the Mimir bucket index.
type BucketIndexMetadataFetcher struct {
	userID      string
	bkt         objstore.Bucket
	cfgProvider objstore.TenantConfigProvider
	logger      log.Logger
	filters     []block.MetadataFilter
	metrics     *block.FetcherMetrics
	fallback    block.MetadataFetcher
}

func NewBucketIndexMetadataFetcher(
	userID string,
	bkt objstore.Bucket,
	cfgProvider objstore.TenantConfigProvider,
	logger log.Logger,
	reg prometheus.Registerer,
	filters []block.MetadataFilter,
) *BucketIndexMetadataFetcher {
	return &BucketIndexMetadataFetcher{
		userID:      userID,
		bkt:         bkt,
		cfgProvider: cfgProvider,
		logger:      logger,
		filters:     filters,
		metrics:     block.NewFetcherMetrics(reg, [][]string{{corruptedBucketIndex}, {noBucketIndex}, {minTimeExcludedMeta}}),
	}
}

func (f *BucketIndexMetadataFetcher) fallbackFetch(ctx context.Context) (metas map[ulid.ULID]*block.Meta, partial map[ulid.ULID]error, err error) {
	if f.fallback == nil {
		userBucket := objstore.NewTenantBucketClient(f.userID, f.bkt, f.cfgProvider)
		// Empty cache dir path disables on-disk cache.
		// Provided that an in-memory cache is maintained,
		// and store gateway does not have a persistent
		// file system, on-disk cache is not helpful.
		fetcher, err := block.NewMetaFetcherWithMetrics(f.logger, 16, userBucket, "", f.metrics, f.filters)
		if err != nil {
			return nil, nil, err
		}
		f.fallback = fetcher
	}

	return f.fallback.Fetch(ctx)
}

// Fetch implements block.MetadataFetcher. Not goroutine-safe.
func (f *BucketIndexMetadataFetcher) Fetch(ctx context.Context) (metas map[ulid.ULID]*block.Meta, partial map[ulid.ULID]error, err error) {
	f.metrics.ResetTx()

	start := time.Now()

	// Fetch the bucket index.
	idx, err := bucketindex.ReadIndex(ctx, f.bkt, f.userID, f.cfgProvider, f.logger)
	if errors.Is(err, bucketindex.ErrIndexNotFound) {
		// This is a legit case happening when the first blocks of a tenant have recently been uploaded by ingesters
		// and their bucket index has not been created yet.
		defer func() {
			f.metrics.Synced.WithLabelValues(noBucketIndex).Set(1)
			f.metrics.Submit()
		}()

		level.Warn(f.logger).Log("msg", "no bucket index found, falling back to fetching directly from bucket", "user", f.userID)
		return f.fallbackFetch(ctx)
	}
	if errors.Is(err, bucketindex.ErrIndexCorrupted) {
		// In case a single tenant bucket index is corrupted, we don't want the store-gateway to fail at startup
		// because unable to fetch blocks metadata. We'll act as if the tenant has no bucket index, but the query
		// will fail anyway in the querier (the querier fails in the querier if bucket index is corrupted).
		level.Error(f.logger).Log("msg", "corrupted bucket index found, falling back to fetching directly from bucket", "user", f.userID, "err", err)
		defer func() {
			f.metrics.Synced.WithLabelValues(corruptedBucketIndex).Set(1)
			f.metrics.Submit()
		}()

		return f.fallbackFetch(ctx)
	}
	if err != nil {
		f.metrics.Synced.WithLabelValues(block.FailedMeta).Set(1)
		f.metrics.Submit()

		return nil, nil, errors.Wrapf(err, "read bucket index")
	}

	// check if index is older than 1 hour, fallback to metafetcher
	if time.Unix(idx.UpdatedAt, 0).Before(start.Add(-1 * time.Hour)) {
		defer func() {
			f.metrics.Synced.WithLabelValues(bucketIndexOlderThanHour).Set(1)
			f.metrics.Submit()
		}()

		level.Warn(f.logger).Log("msg", "bucket index is older than 1 hour, falling back to fetching directly from bucket", "user", f.userID)
		return f.fallbackFetch(ctx)
	}

	defer func() {
		f.metrics.SyncDuration.Observe(time.Since(start).Seconds())
		if err != nil {
			f.metrics.SyncFailures.Inc()
		}
	}()
	f.metrics.Syncs.Inc()

	// Build block metas out of the index.
	metas = make(map[ulid.ULID]*block.Meta, len(idx.Blocks))
	for _, b := range idx.Blocks {
		metas[b.ID] = b.Meta()
	}

	for _, filter := range f.filters {
		var err error

		// NOTE: filter can update synced metric accordingly to the reason of the exclude.
		if customFilter, ok := filter.(MetadataFilterWithBucketIndex); ok {
			err = customFilter.FilterWithBucketIndex(ctx, metas, idx, f.metrics.Synced)
		} else {
			err = filter.Filter(ctx, metas, f.metrics.Synced)
		}

		if err != nil {
			return nil, nil, errors.Wrap(err, "filter metas")
		}
	}

	f.metrics.Synced.WithLabelValues(block.LoadedMeta).Set(float64(len(metas)))
	f.metrics.Submit()

	return metas, nil, nil
}

package storegateway

import (
	"context"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/dskit/ring"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/model/timestamp"

	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/phlaredb/block"
	"github.com/grafana/pyroscope/pkg/phlaredb/bucketindex"
)

const (
	shardExcludedMeta = "shard-excluded"
)

var errStoreGatewayUnhealthy = errors.New("store-gateway is unhealthy in the ring")

type ShardingStrategy interface {
	// FilterUsers whose blocks should be loaded by the store-gateway. Returns the list of user IDs
	// that should be synced by the store-gateway.
	FilterUsers(ctx context.Context, userIDs []string) ([]string, error)

	// FilterBlocks filters metas in-place keeping only blocks that should be loaded by the store-gateway.
	// The provided loaded map contains blocks which have been previously returned by this function and
	// are now loaded or loading in the store-gateway.
	FilterBlocks(ctx context.Context, userID string, metas map[ulid.ULID]*block.Meta, loaded map[ulid.ULID]struct{}, synced block.GaugeVec) error
}

type shardingMetadataFilterAdapter struct {
	userID   string
	strategy ShardingStrategy

	// Keep track of the last blocks returned by the Filter() function.
	lastBlocks map[ulid.ULID]struct{}
}

// SardingStrategy is a shuffle sharding strategy, based on the hash ring formed by store-gateways,
// where each tenant blocks are sharded across a subset of store-gateway instances.
type ShuffleShardingStrategy struct {
	r            *ring.Ring
	instanceID   string
	instanceAddr string
	limits       ShardingLimits
	logger       log.Logger
}

// NewShuffleShardingStrategy makes a new ShuffleShardingStrategy.
func NewShuffleShardingStrategy(r *ring.Ring, instanceID, instanceAddr string, limits ShardingLimits, logger log.Logger) *ShuffleShardingStrategy {
	return &ShuffleShardingStrategy{
		r:            r,
		instanceID:   instanceID,
		instanceAddr: instanceAddr,
		limits:       limits,
		logger:       logger,
	}
}

// FilterUsers implements ShardingStrategy.
func (s *ShuffleShardingStrategy) FilterUsers(_ context.Context, userIDs []string) ([]string, error) {
	// As a protection, ensure the store-gateway instance is healthy in the ring. It could also be missing
	// in the ring if it was failing to heartbeat the ring and it got remove from another healthy store-gateway
	// instance, because of the auto-forget feature.
	if set, err := s.r.GetAllHealthy(BlocksOwnerSync); err != nil {
		return nil, err
	} else if !set.Includes(s.instanceAddr) {
		return nil, errStoreGatewayUnhealthy
	}

	var filteredIDs []string

	for _, userID := range userIDs {
		subRing := GetShuffleShardingSubring(s.r, userID, s.limits)

		// Include the user only if it belongs to this store-gateway shard.
		if subRing.HasInstance(s.instanceID) {
			filteredIDs = append(filteredIDs, userID)
		}
	}

	return filteredIDs, nil
}

// FilterBlocks implements ShardingStrategy.
func (s *ShuffleShardingStrategy) FilterBlocks(_ context.Context, userID string, metas map[ulid.ULID]*block.Meta, loaded map[ulid.ULID]struct{}, synced block.GaugeVec) error {
	// As a protection, ensure the store-gateway instance is healthy in the ring. If it's unhealthy because it's failing
	// to heartbeat or get updates from the ring, or even removed from the ring because of the auto-forget feature, then
	// keep the previously loaded blocks.
	if set, err := s.r.GetAllHealthy(BlocksOwnerSync); err != nil || !set.Includes(s.instanceAddr) {
		for blockID := range metas {
			if _, ok := loaded[blockID]; ok {
				level.Warn(s.logger).Log("msg", "store-gateway is unhealthy in the ring but block is kept because was previously loaded", "block", blockID.String(), "err", err)
			} else {
				level.Warn(s.logger).Log("msg", "store-gateway is unhealthy in the ring and block has been excluded because was not previously loaded", "block", blockID.String(), "err", err)

				// Skip the block.
				synced.WithLabelValues(shardExcludedMeta).Inc()
				delete(metas, blockID)
			}
		}

		return nil
	}

	r := GetShuffleShardingSubring(s.r, userID, s.limits)
	bufDescs, bufHosts, bufZones := ring.MakeBuffersForGet()

	for blockID := range metas {
		key := block.HashBlockID(blockID)

		// Check if the block is owned by the store-gateway
		set, err := r.Get(key, BlocksOwnerSync, bufDescs, bufHosts, bufZones)
		// If an error occurs while checking the ring, we keep the previously loaded blocks.
		if err != nil {
			if _, ok := loaded[blockID]; ok {
				level.Warn(s.logger).Log("msg", "failed to check block owner but block is kept because was previously loaded", "block", blockID.String(), "err", err)
			} else {
				level.Warn(s.logger).Log("msg", "failed to check block owner and block has been excluded because was not previously loaded", "block", blockID.String(), "err", err)

				// Skip the block.
				synced.WithLabelValues(shardExcludedMeta).Inc()
				delete(metas, blockID)
			}

			continue
		}

		// Keep the block if it is owned by the store-gateway.
		if set.Includes(s.instanceAddr) {
			continue
		}

		// The block is not owned by the store-gateway. However, if it's currently loaded
		// we can safely unload it only once at least 1 authoritative owner is available
		// for queries.
		if _, ok := loaded[blockID]; ok {
			// The ring Get() returns an error if there's no available instance.
			if _, err := r.Get(key, BlocksOwnerRead, bufDescs, bufHosts, bufZones); err != nil {
				// Keep the block.
				continue
			}
		}

		// The block is not owned by the store-gateway and there's at least 1 available
		// authoritative owner available for queries, so we can filter it out (and unload
		// it if it was loaded).
		synced.WithLabelValues(shardExcludedMeta).Inc()
		delete(metas, blockID)
	}

	return nil
}

// GetShuffleShardingSubring returns the subring to be used for a given user. This function
// should be used both by store-gateway and querier in order to guarantee the same logic is used.
func GetShuffleShardingSubring(ring *ring.Ring, userID string, limits ShardingLimits) ring.ReadRing {
	shardSize := limits.StoreGatewayTenantShardSize(userID)

	// A shard size of 0 means shuffle sharding is disabled for this specific user,
	// so we just return the full ring so that blocks will be sharded across all store-gateways.
	if shardSize <= 0 {
		return ring
	}

	return ring.ShuffleShard(userID, shardSize)
}

func NewShardingMetadataFilterAdapter(userID string, strategy ShardingStrategy) block.MetadataFilter {
	return &shardingMetadataFilterAdapter{
		userID:     userID,
		strategy:   strategy,
		lastBlocks: map[ulid.ULID]struct{}{},
	}
}

// Filter implements block.MetadataFilter.
// This function is NOT safe for use by multiple goroutines concurrently.
func (a *shardingMetadataFilterAdapter) Filter(ctx context.Context, metas map[ulid.ULID]*block.Meta, synced block.GaugeVec) error {
	if err := a.strategy.FilterBlocks(ctx, a.userID, metas, a.lastBlocks, synced); err != nil {
		return err
	}

	// Keep track of the last filtered blocks.
	a.lastBlocks = make(map[ulid.ULID]struct{}, len(metas))
	for blockID := range metas {
		a.lastBlocks[blockID] = struct{}{}
	}

	return nil
}

const minTimeExcludedMeta = "min-time-excluded"

// minTimeMetaFilter filters out blocks that contain the most recent data (based on block MinTime).
type minTimeMetaFilter struct {
	limit time.Duration
}

func newMinTimeMetaFilter(limit time.Duration) *minTimeMetaFilter {
	return &minTimeMetaFilter{limit: limit}
}

func (f *minTimeMetaFilter) Filter(_ context.Context, metas map[ulid.ULID]*block.Meta, synced block.GaugeVec) error {
	if f.limit <= 0 {
		return nil
	}

	limitTime := timestamp.FromTime(time.Now().Add(-f.limit))

	for id, m := range metas {
		if int64(m.MinTime) < limitTime {
			continue
		}

		synced.WithLabelValues(minTimeExcludedMeta).Inc()
		delete(metas, id)
	}
	return nil
}

type MetadataFilterWithBucketIndex interface {
	// FilterWithBucketIndex is like Thanos MetadataFilter.Filter() but it provides in input the bucket index too.
	FilterWithBucketIndex(ctx context.Context, metas map[ulid.ULID]*block.Meta, idx *bucketindex.Index, synced block.GaugeVec) error
}

// IgnoreDeletionMarkFilter is like the Thanos IgnoreDeletionMarkFilter, but it also implements
// the MetadataFilterWithBucketIndex interface.
type IgnoreDeletionMarkFilter struct {
	upstream *block.IgnoreDeletionMarkFilter

	delay           time.Duration
	deletionMarkMap map[ulid.ULID]*block.DeletionMark
}

// NewIgnoreDeletionMarkFilter creates IgnoreDeletionMarkFilter.
func NewIgnoreDeletionMarkFilter(logger log.Logger, bkt objstore.BucketReader, delay time.Duration, concurrency int) *IgnoreDeletionMarkFilter {
	return &IgnoreDeletionMarkFilter{
		upstream: block.NewIgnoreDeletionMarkFilter(logger, bkt, delay, concurrency),
		delay:    delay,
	}
}

// DeletionMarkBlocks returns blocks that were marked for deletion.
func (f *IgnoreDeletionMarkFilter) DeletionMarkBlocks() map[ulid.ULID]*block.DeletionMark {
	// If the cached deletion marks exist it means the filter function was called with the bucket
	// index, so it's safe to return it.
	if f.deletionMarkMap != nil {
		return f.deletionMarkMap
	}

	return f.upstream.DeletionMarkBlocks()
}

// Filter implements block.MetadataFilter.
func (f *IgnoreDeletionMarkFilter) Filter(ctx context.Context, metas map[ulid.ULID]*block.Meta, synced block.GaugeVec) error {
	return f.upstream.Filter(ctx, metas, synced)
}

// FilterWithBucketIndex implements MetadataFilterWithBucketIndex.
func (f *IgnoreDeletionMarkFilter) FilterWithBucketIndex(_ context.Context, metas map[ulid.ULID]*block.Meta, idx *bucketindex.Index, synced block.GaugeVec) error {
	// Build a map of block deletion marks
	marks := make(map[ulid.ULID]*block.DeletionMark, len(idx.BlockDeletionMarks))
	for _, mark := range idx.BlockDeletionMarks {
		marks[mark.ID] = mark.BlockDeletionMark()
	}

	// Keep it cached.
	f.deletionMarkMap = marks

	for _, mark := range marks {
		if _, ok := metas[mark.ID]; !ok {
			continue
		}

		if time.Since(time.Unix(mark.DeletionTime, 0)).Seconds() > f.delay.Seconds() {
			synced.WithLabelValues(block.MarkedForDeletionMeta).Inc()
			delete(metas, mark.ID)
		}
	}

	return nil
}

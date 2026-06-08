package index

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	indexstore "github.com/grafana/pyroscope/v2/pkg/metastore/index/store"
	kvstore "github.com/grafana/pyroscope/v2/pkg/metastore/store"
	"github.com/grafana/pyroscope/v2/pkg/test"
)

func TestCacheMetrics(t *testing.T) {
	const (
		tenant         = "tenant"
		shardID uint32 = 1
	)

	m := newMetrics(nil)

	db := test.BoltDB(t)
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		return indexstore.NewIndexStore().CreateBuckets(tx)
	}))

	p := indexstore.NewPartition(test.Time("2024-09-11T06:00:00.000Z"), DefaultConfig.partitionDuration)

	// Shard cache.
	sc := newShardCache(DefaultConfig.ShardCacheSize, indexstore.NewIndexStore(), m)

	// First write lookup: cache empty == miss.
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		shard, err := sc.getForWrite(tx, p, tenant, shardID)
		if err != nil {
			return err
		}
		require.Equal(t, shardID, shard.Shard)
		return nil
	}))
	require.Equal(t, float64(1), testutil.ToFloat64(m.cacheRequests.WithLabelValues("shard_write", "miss")))
	require.Equal(t, float64(0), testutil.ToFloat64(m.cacheRequests.WithLabelValues("shard_write", "hit")))

	// Second write lookup: cached writable == hit.
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		shard, err := sc.getForWrite(tx, p, tenant, shardID)
		if err != nil {
			return err
		}
		require.Equal(t, shardID, shard.Shard)
		return nil
	}))
	require.Equal(t, float64(1), testutil.ToFloat64(m.cacheRequests.WithLabelValues("shard_write", "hit")))

	// Read lookup on the same key: writable entry counts as a read hit too.
	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		_, err := sc.getForReadAtVersion(tx, p, tenant, shardID, 0)
		return err
	}))
	require.Equal(t, float64(1), testutil.ToFloat64(m.cacheRequests.WithLabelValues("shard_read", "hit")))

	// Read lookup on a new key: miss + entry inserted as read-only.
	require.NoError(t, db.View(func(tx *bbolt.Tx) error {
		_, err := sc.getForReadAtVersion(tx, p, tenant, shardID+1, 0)
		return err
	}))
	require.Equal(t, float64(1), testutil.ToFloat64(m.cacheRequests.WithLabelValues("shard_read", "miss")))

	// Now a write lookup on that key: cached entry is read-only == miss + reload.
	require.NoError(t, db.Update(func(tx *bbolt.Tx) error {
		shard, err := sc.getForWrite(tx, p, tenant, shardID+1)
		if err != nil {
			return err
		}
		require.Equal(t, shardID+1, shard.Shard)
		return nil
	}))
	require.Equal(t, float64(2), testutil.ToFloat64(m.cacheRequests.WithLabelValues("shard_write", "miss")))

	// Block cache.
	bc := newBlockCache(DefaultConfig.BlockReadCacheSize, DefaultConfig.BlockWriteCacheSize, m)
	shard := indexstore.NewShard(p, tenant, shardID)
	blockL0 := &metastorev1.BlockMeta{Id: test.ULID("2024-09-11T07:00:00.001Z"), CompactionLevel: 0}
	blockL2 := &metastorev1.BlockMeta{Id: test.ULID("2024-09-11T08:00:00.001Z"), CompactionLevel: 2}

	// Put both into their respective tiers.
	bc.put(shard, blockL0)
	bc.put(shard, blockL2)

	// Encode KV for getOrCreate.
	l0Bytes, err := blockL0.MarshalVT()
	require.NoError(t, err)
	l2Bytes, err := blockL2.MarshalVT()
	require.NoError(t, err)

	// blockL0 was put in the write tier (CompactionLevel < 2): write tier serves the lookup.
	bc.getOrCreate(shard, kvstore.KV{Key: []byte(blockL0.Id), Value: l0Bytes})
	require.Equal(t, float64(1), testutil.ToFloat64(m.cacheRequests.WithLabelValues("block", "write_hit")))
	require.Equal(t, float64(0), testutil.ToFloat64(m.cacheRequests.WithLabelValues("block", "read_hit")))
	require.Equal(t, float64(0), testutil.ToFloat64(m.cacheRequests.WithLabelValues("block", "miss")))

	// blockL2 was put in the read tier (CompactionLevel >= 2): read tier serves the lookup.
	bc.getOrCreate(shard, kvstore.KV{Key: []byte(blockL2.Id), Value: l2Bytes})
	require.Equal(t, float64(1), testutil.ToFloat64(m.cacheRequests.WithLabelValues("block", "read_hit")))
	require.Equal(t, float64(1), testutil.ToFloat64(m.cacheRequests.WithLabelValues("block", "write_hit")))
	require.Equal(t, float64(0), testutil.ToFloat64(m.cacheRequests.WithLabelValues("block", "miss")))

	// Unknown block: both tiers miss == decoded fresh.
	uncachedID := test.ULID("2024-09-11T09:00:00.001Z")
	uncached := &metastorev1.BlockMeta{Id: uncachedID, CompactionLevel: 0}
	uncachedBytes, err := uncached.MarshalVT()
	require.NoError(t, err)
	bc.getOrCreate(shard, kvstore.KV{Key: []byte(uncachedID), Value: uncachedBytes})
	require.Equal(t, float64(1), testutil.ToFloat64(m.cacheRequests.WithLabelValues("block", "miss")))
}

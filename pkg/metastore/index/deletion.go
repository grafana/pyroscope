package index

import (
	"context"
	"time"

	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	indexstore "github.com/grafana/pyroscope/v2/pkg/metastore/index/store"
)

// DeletionCandidates returns complete (not query-filtered) block metadata for
// single-tenant compacted blocks (CompactionLevel > 0 && BlockMeta.Tenant != 0).
// Scan all partitions: a block's ULID timestamp need not bound its data times.
func (i *Index) DeletionCandidates(ctx context.Context, tx *bbolt.Tx, tenant string, minTime, maxTime int64) ([]*metastorev1.BlockMeta, error) {
	if tenant == "" {
		return nil, nil
	}
	startTime, endTime := time.Unix(0, minTime), time.Unix(0, maxTime)
	overlaps := func(blockMinMilli, blockMaxMilli int64) bool {
		return !time.UnixMilli(blockMinMilli).After(endTime) && !time.UnixMilli(blockMaxMilli).Before(startTime)
	}
	var result []*metastorev1.BlockMeta
	for p := range i.Partitions(tx) {
		q := p.Query(tx)
		if q == nil {
			continue
		}
		for s := range q.Shards(tenant) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if !s.ShardIndex.Overlaps(startTime, endTime) {
				continue
			}
			it := s.Blocks(tx)
			if it == nil {
				continue
			}
			var shard *indexstore.Shard
			for it.Next() {
				kv := it.At()
				if blockMin, blockMax, err := blockMetaTimeBounds(kv.Value); err == nil && !overlaps(blockMin, blockMax) {
					continue
				}
				if shard == nil {
					var err error
					shard, err = i.shards.getForReadAtVersion(tx, p, tenant, s.Shard, s.ShardIndex.Version)
					if err != nil {
						_ = it.Close()
						return nil, err
					}
				}
				b := i.blocks.getOrCreate(shard, kv)
				if b.CompactionLevel == 0 || b.Tenant == 0 || !overlaps(b.MinTime, b.MaxTime) {
					continue
				}
				bCopy := b.CloneVT()
				shard.StringTable.Export(bCopy)
				result = append(result, bCopy)
			}
			err := it.Err()
			_ = it.Close()
			if err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}

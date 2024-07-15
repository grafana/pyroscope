package metastore

import (
	"context"

	"github.com/go-kit/log/level"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

func (m *Metastore) AddBlock(_ context.Context, req *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
	_, resp, err := applyCommand[*metastorev1.AddBlockRequest, *metastorev1.AddBlockResponse](m.raft, req, m.config.Raft.ApplyTimeout)
	if err == nil {
		m.addForCompaction(req.Block)
	}
	return resp, err
}

func (m *metastoreState) applyAddBlock(request *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
	_ = level.Info(m.logger).Log("msg", "adding block", "block_id", request.Block.Id)
	if request.Block.CompactionLevel != 0 {
		_ = level.Error(m.logger).Log(
			"msg", "compaction not implemented, ignoring block with non-zero compaction level",
			"compaction_level", request.Block.CompactionLevel,
			"block", request.Block.Id,
		)
		return &metastorev1.AddBlockResponse{}, nil
	}
	name, key := keyForBlockMeta(request.Block.Shard, "", request.Block.Id)
	value, err := request.Block.MarshalVT()
	if err != nil {
		return nil, err
	}
	err = m.db.boltdb.Update(func(tx *bbolt.Tx) error {
		return updateBlockMetadataBucket(tx, name, func(bucket *bbolt.Bucket) error {
			return bucket.Put(key, value)
		})
	})
	if err != nil {
		_ = level.Error(m.logger).Log(
			"msg", "failed to add block",
			"block", request.Block.Id,
			"err", err,
		)
		return nil, err
	}
	m.getOrCreateShard(request.Block.Shard).putSegment(request.Block)
	return &metastorev1.AddBlockResponse{}, nil
}

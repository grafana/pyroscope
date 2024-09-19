package metastore

import (
	"context"
	"time"

	"github.com/go-kit/log/level"

	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

func (m *Metastore) AddBlock(_ context.Context, req *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
	_ = level.Info(m.logger).Log(
		"msg", "adding block",
		"block_id", req.Block.Id,
		"shard", req.Block.Shard,
		"raft_commit_index", m.raft.CommitIndex(),
		"raft_last_index", m.raft.LastIndex(),
		"raft_applied_index", m.raft.AppliedIndex())
	t1 := time.Now()
	defer func() {
		m.metrics.raftAddBlockDuration.Observe(time.Since(t1).Seconds())
		level.Debug(m.logger).Log("msg", "add block duration", "block_id", req.Block.Id, "shard", req.Block.Shard, "duration", time.Since(t1))
	}()
	_, resp, err := applyCommand[*metastorev1.AddBlockRequest, *metastorev1.AddBlockResponse](m.raft, req, m.config.Raft.ApplyTimeout)
	if err != nil {
		_ = level.Error(m.logger).Log("msg", "failed to apply add block", "block_id", req.Block.Id, "shard", req.Block.Shard, "err", err)
	}
	return resp, err
}

func (m *metastoreState) applyAddBlock(log *raft.Log, request *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
	name, key := keyForBlockMeta(request.Block.Shard, "", request.Block.Id)
	value, err := request.Block.MarshalVT()
	if err != nil {
		return nil, err
	}

	err = m.db.boltdb.Update(func(tx *bbolt.Tx) error {
		err := updateBlockMetadataBucket(tx, name, func(bucket *bbolt.Bucket) error {
			return bucket.Put(key, value)
		})
		if err != nil {
			return err
		}
		if err = m.compactBlock(request.Block, tx, log.Index); err != nil {
			return err
		}
		return nil
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

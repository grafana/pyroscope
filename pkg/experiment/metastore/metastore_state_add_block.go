package metastore

import (
	"context"
	"github.com/go-kit/log"
	"time"

	"github.com/go-kit/log/level"

	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

func (m *Metastore) AddBlock(_ context.Context, req *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
	l := log.With(m.logger, "shard", req.Block.Shard, "block_id", req.Block.Id, "ts", req.Block.MinTime)
	_ = level.Info(l).Log("msg", "adding block")
	t1 := time.Now()
	defer func() {
		m.metrics.raftAddBlockDuration.Observe(time.Since(t1).Seconds())
	}()
	_, resp, err := applyCommand[*metastorev1.AddBlockRequest, *metastorev1.AddBlockResponse](m.raft, req, m.config.Raft.ApplyTimeout)
	if err != nil {
		_ = level.Error(l).Log("msg", "failed to apply add block", "err", err)
	}
	return resp, err
}

func (m *Metastore) AddRecoveredBlock(_ context.Context, req *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
	l := log.With(m.logger, "shard", req.Block.Shard, "block_id", req.Block.Id, "ts", req.Block.MinTime)
	_ = level.Info(l).Log("msg", "adding recovered block")
	t1 := time.Now()
	defer func() {
		m.metrics.raftAddRecoveredBlockDuration.Observe(time.Since(t1).Seconds())
	}()
	_, resp, err := applyCommand[*metastorev1.AddBlockRequest, *metastorev1.AddBlockResponse](m.raft, req, m.config.Raft.ApplyTimeout)
	if err != nil {
		_ = level.Error(l).Log("msg", "failed to apply add recovered block", "err", err)
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

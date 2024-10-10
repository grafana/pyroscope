package metastore

import (
	"context"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"github.com/oklog/ulid"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	adaptiveplacement "github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement"
	"github.com/grafana/pyroscope/pkg/iter"
)

func (m *Metastore) AddBlock(_ context.Context, req *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
	l := log.With(m.logger, "shard", req.Block.Shard, "block_id", req.Block.Id, "ts", req.Block.MinTime)
	_, err := ulid.Parse(req.Block.Id)
	if err != nil {
		_ = level.Warn(l).Log("failed to parse block id", "err", err)
		return nil, err
	}
	_ = level.Info(l).Log("msg", "adding block")
	t1 := time.Now()
	defer func() {
		if err == nil {
			m.placementMgr.RecordStats(statSamplesFromMeta(req.Block))
		}
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
	err := m.db.boltdb.Update(func(tx *bbolt.Tx) error {
		err := m.persistBlock(tx, request.Block)
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
	m.index.InsertBlock(request.Block)
	return &metastorev1.AddBlockResponse{}, nil
}

func (m *metastoreState) persistBlock(tx *bbolt.Tx, block *metastorev1.BlockMeta) error {
	key := []byte(block.Id)
	value, err := block.MarshalVT()
	if err != nil {
		return err
	}

	partKey := m.index.CreatePartitionKey(block.Id)

	return updateBlockMetadataBucket(tx, partKey, block.Shard, block.TenantId, func(bucket *bbolt.Bucket) error {
		return bucket.Put(key, value)
	})
}

func statSamplesFromMeta(md *metastorev1.BlockMeta) iter.Iterator[adaptiveplacement.Sample] {
	return &sampleIterator{md: md}
}

type sampleIterator struct {
	md  *metastorev1.BlockMeta
	cur int
}

func (s *sampleIterator) Err() error   { return nil }
func (s *sampleIterator) Close() error { return nil }

func (s *sampleIterator) Next() bool {
	if s.cur >= len(s.md.Datasets) {
		return false
	}
	s.cur++
	return true
}

func (s *sampleIterator) At() adaptiveplacement.Sample {
	ds := s.md.Datasets[s.cur-1]
	return adaptiveplacement.Sample{
		TenantID:    ds.TenantId,
		DatasetName: ds.Name,
		ShardOwner:  s.md.CreatedBy,
		ShardID:     s.md.Shard,
		Size:        ds.Size,
	}
}

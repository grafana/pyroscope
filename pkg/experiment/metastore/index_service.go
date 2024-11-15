package metastore

import (
	"context"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	placement "github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement"
	"github.com/grafana/pyroscope/pkg/iter"
)

type IndexCommandLog interface {
	AddBlock(*metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error)
}

type PlacementStats interface {
	RecordStats(iter.Iterator[placement.Sample])
}

func NewIndexService(
	logger log.Logger,
	raftLog IndexCommandLog,
	stats PlacementStats,
) *IndexService {
	return &IndexService{
		logger:  logger,
		raftLog: raftLog,
		stats:   stats,
	}
}

type IndexService struct {
	metastorev1.IndexServiceServer

	logger  log.Logger
	raftLog IndexCommandLog
	stats   PlacementStats
}

func (svc *IndexService) AddBlock(
	_ context.Context,
	req *metastorev1.AddBlockRequest,
) (resp *metastorev1.AddBlockResponse, err error) {
	if err = SanitizeMetadata(req.Block); err != nil {
		_ = level.Warn(svc.logger).Log("invalid metadata", "block_id", req.Block.Id, "err", err)
		return nil, err
	}
	defer func() {
		if err == nil {
			svc.stats.RecordStats(statsFromMetadata(req.Block))
		}
	}()
	if _, err = svc.raftLog.AddBlock(&metastorev1.AddBlockRequest{Block: req.Block}); err != nil {
		_ = level.Error(svc.logger).Log("msg", "failed to add block", "block_id", req.Block.Id, "err", err)
		return nil, err
	}
	return new(metastorev1.AddBlockResponse), nil
}

func (svc *IndexService) AddRecoveredBlock(
	_ context.Context,
	req *metastorev1.AddBlockRequest,
) (*metastorev1.AddBlockResponse, error) {
	logger := log.With(svc.logger, "shard", req.Block.Shard, "block_id", req.Block.Id, "ts", req.Block.MinTime)
	_ = level.Info(logger).Log("msg", "adding recovered block")
	if _, err := svc.raftLog.AddBlock(&metastorev1.AddBlockRequest{Block: req.Block}); err != nil {
		_ = level.Error(svc.logger).Log("msg", "failed to add block", "block_id", req.Block.Id, "err", err)
		return nil, err
	}
	return new(metastorev1.AddBlockResponse), nil
}

func statsFromMetadata(md *metastorev1.BlockMeta) iter.Iterator[placement.Sample] {
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

func (s *sampleIterator) At() placement.Sample {
	ds := s.md.Datasets[s.cur-1]
	return placement.Sample{
		TenantID:    ds.TenantId,
		DatasetName: ds.Name,
		ShardOwner:  s.md.CreatedBy,
		ShardID:     s.md.Shard,
		Size:        ds.Size,
	}
}

// TODO(kolesnikovae): Implement and refactor to the block package.

func SanitizeMetadata(md *metastorev1.BlockMeta) error {
	_, err := ulid.Parse(md.Id)
	return err
}

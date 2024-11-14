package metastore

import (
	"context"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	placement "github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement"
	"github.com/grafana/pyroscope/pkg/iter"
)

type IndexCommandLog interface {
	AddBlockMetadata(request *raft_log.AddBlockMetadataRequest) (*raft_log.AddBlockMetadataResponse, error)
}

type PlacementStats interface {
	RecordStats(iter.Iterator[placement.Sample])
}

func NewIndexService(
	logger log.Logger,
	raft IndexCommandLog,
	stats PlacementStats,
) *IndexService {
	return &IndexService{
		logger: logger,
		raft:   raft,
		stats:  stats,
	}
}

type IndexService struct {
	metastorev1.IndexServiceServer

	logger log.Logger
	raft   IndexCommandLog
	stats  PlacementStats
}

func (svc *IndexService) AddBlock(
	ctx context.Context,
	req *metastorev1.AddBlockRequest,
) (resp *metastorev1.AddBlockResponse, err error) {
	defer func() {
		if err == nil {
			svc.stats.RecordStats(statsFromMetadata(req.Block))
		}
	}()
	return svc.addBlockMetadata(ctx, req)
}

func (svc *IndexService) AddRecoveredBlock(
	ctx context.Context,
	req *metastorev1.AddBlockRequest,
) (*metastorev1.AddBlockResponse, error) {
	return svc.addBlockMetadata(ctx, req)
}

func (svc *IndexService) addBlockMetadata(
	_ context.Context,
	req *metastorev1.AddBlockRequest,
) (resp *metastorev1.AddBlockResponse, err error) {
	if err = SanitizeMetadata(req.Block); err != nil {
		_ = level.Warn(svc.logger).Log("invalid metadata", "block_id", req.Block.Id, "err", err)
		return nil, err
	}
	if _, err = svc.raft.AddBlockMetadata(&raft_log.AddBlockMetadataRequest{Metadata: req.Block}); err != nil {
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

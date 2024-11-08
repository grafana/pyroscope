package metastore

import (
	"context"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/block"
	"github.com/grafana/pyroscope/pkg/experiment/distributor/placement"
	"github.com/grafana/pyroscope/pkg/iter"
)

type IndexReader interface {
	GetBlockMetadata(context.Context, *metastorev1.BlockList) ([]*metastorev1.BlockMeta, error)
}

type IndexCommandLog interface {
	AddBlockMetadata(*raft_log.AddBlockMetadataRequest) (*raft_log.AddBlockMetadataResponse, error)
}

type PlacementStats interface {
	RecordStats(iter.Iterator[placement.StatsSample])
}

func NewIndexService(
	logger log.Logger,
	raftLog IndexCommandLog,
	follower RaftFollower,
	index IndexReader,
	stats PlacementStats,
) *IndexService {
	return &IndexService{
		logger:   logger,
		raftLog:  raftLog,
		follower: follower,
		index:    index,
		stats:    stats,
	}
}

type IndexService struct {
	metastorev1.IndexServiceServer

	logger   log.Logger
	raftLog  IndexCommandLog
	follower RaftFollower

	index IndexReader
	stats PlacementStats
}

func (svc *IndexService) AddBlock(
	_ context.Context,
	req *metastorev1.AddBlockRequest,
) (resp *metastorev1.AddBlockResponse, err error) {
	if err = block.SanitizeMetadata(req.Block); err != nil {
		_ = level.Warn(svc.logger).Log("invalid metadata", "block_id", req.Block.Id, "err", err)
		return nil, err
	}
	defer func() {
		if err == nil {
			svc.stats.RecordStats(statsFromMetadata(req.Block))
		}
	}()
	if _, err = svc.raftLog.AddBlockMetadata(&raft_log.AddBlockMetadataRequest{Metadata: req.Block}); err != nil {
		_ = level.Error(svc.logger).Log("msg", "failed to add block", "block_id", req.Block.Id, "err", err)
		return nil, err
	}
	return new(metastorev1.AddBlockResponse), nil
}

func (svc *IndexService) GetBlockMetadata(
	ctx context.Context,
	req *metastorev1.GetBlockMetadataRequest,
) (*metastorev1.GetBlockMetadataResponse, error) {
	if err := svc.follower.WaitLeaderCommitIndexAppliedLocally(ctx); err != nil {
		return nil, err
	}
	blocks, err := svc.index.GetBlockMetadata(ctx, req.Blocks)
	if err != nil {
		return nil, err
	}
	return &metastorev1.GetBlockMetadataResponse{Blocks: blocks}, nil
}

func statsFromMetadata(md *metastorev1.BlockMeta) iter.Iterator[placement.StatsSample] {
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

func (s *sampleIterator) At() placement.StatsSample {
	ds := s.md.Datasets[s.cur-1]
	return placement.StatsSample{
		TenantID:    ds.TenantId,
		DatasetName: ds.Name,
		ShardOwner:  s.md.CreatedBy,
		ShardID:     s.md.Shard,
		Size:        ds.Size,
	}
}

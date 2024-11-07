package metastore

import (
	"context"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	adaptiveplacement "github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement"
	"github.com/grafana/pyroscope/pkg/iter"
)

// TODO(kolesnikovae): Pass AddBlockCommandLog to DLQ.

type IndexReader interface {
	GetBlockMetadata(*bbolt.Tx, *metastorev1.BlockList) ([]*metastorev1.BlockMeta, error)
}

type IndexCommandLog interface {
	AddBlockMetadata(*raft_log.AddBlockMetadataRequest) (*raft_log.AddBlockMetadataResponse, error)
	ReplaceBlocks(*raft_log.ReplaceBlockMetadataRequest) (*raft_log.ReplaceBlockMetadataResponse, error)
}

type PlacementStats interface {
	RecordStats(iter.Iterator[adaptiveplacement.Sample])
}

func NewIndexService(
	logger log.Logger,
	raftLog IndexCommandLog,
	index IndexReader,
	stats PlacementStats,
) *IndexService {
	return &IndexService{
		logger: logger,
		raft:   raftLog,
		index:  index,
		stats:  stats,
	}
}

type IndexService struct {
	metastorev1.IndexServiceServer

	logger log.Logger
	raft   IndexCommandLog
	index  IndexReader
	stats  PlacementStats
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

	if _, err = svc.raft.AddBlockMetadata(&raft_log.AddBlockMetadataRequest{Metadata: req.Block}); err != nil {
		_ = level.Error(svc.logger).Log("msg", "failed to add block", "block_id", req.Block.Id, "err", err)
		return nil, err
	}

	return new(metastorev1.AddBlockResponse), nil
}

func (svc *IndexService) ReplaceBlocks(
	_ context.Context,
	req *metastorev1.ReplaceBlocksRequest,
) (resp *metastorev1.ReplaceBlocksResponse, err error) {
	replace := &raft_log.ReplaceBlockMetadataRequest{
		Tenant:       req.SourceBlocks.Tenant,
		Shard:        req.SourceBlocks.Shard,
		SourceBlocks: req.SourceBlocks.Blocks,
		NewBlocks:    req.NewBlocks,
	}
	if _, err = svc.raft.ReplaceBlocks(replace); err != nil {
		_ = level.Error(svc.logger).Log("msg", "failed to replace blocks in metadata index", "err", err)
		return nil, err
	}
	return new(metastorev1.ReplaceBlocksResponse), nil
}

func (svc *IndexService) GetBlockMetadata(
	_ context.Context,
	req *metastorev1.GetBlockMetadataRequest,
) (resp *metastorev1.GetBlockMetadataResponse, err error) {
	// TODO(kolesnikovae): Implement
	return nil, nil
}

func SanitizeMetadata(md *metastorev1.BlockMeta) error {
	// TODO(kolesnikovae): Implement and refactor to the block package.
	_, err := ulid.Parse(md.Id)
	return err
}

func statsFromMetadata(md *metastorev1.BlockMeta) iter.Iterator[adaptiveplacement.Sample] {
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

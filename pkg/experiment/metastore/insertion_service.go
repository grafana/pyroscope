package metastore

import (
	"context"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/ulid"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	adaptiveplacement "github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement"
	"github.com/grafana/pyroscope/pkg/iter"
)

// TODO(kolesnikovae): Pass AddBlockCommandLog to DLQ.

type DistributionStats interface {
	RecordStats(iter.Iterator[adaptiveplacement.Sample])
}

type AddBlockCommandLog interface {
	AddBlockMetadata(proposal *raft_log.AddBlockMetadataRequest) (*raft_log.AddBlockMetadataResponse, error)
}

func NewInsertionService(
	logger log.Logger,
	stats DistributionStats,
	raftLog AddBlockCommandLog,
) *InsertionService {
	return &InsertionService{
		logger: logger,
		stats:  stats,
		raft:   raftLog,
	}
}

type InsertionService struct {
	logger log.Logger
	stats  DistributionStats
	raft   AddBlockCommandLog
}

func (svc *InsertionService) AddBlock(
	_ context.Context,
	req *metastorev1.AddBlockRequest,
) (resp *metastorev1.AddBlockResponse, err error) {
	if err = SanitizeMetadata(req.Block); err != nil {
		_ = level.Warn(svc.logger).Log("invalid metadata", "block_id", req.Block.Id, "err", err)
		return nil, err
	}

	defer func() {
		if err == nil {
			svc.stats.RecordStats(statSamplesFromMeta(req.Block))
		}
	}()

	proposal := &raft_log.AddBlockMetadataRequest{Metadata: req.Block}
	if _, err = svc.raft.AddBlockMetadata(proposal); err != nil {
		_ = level.Error(svc.logger).Log("msg", "failed to add block", "block_id", req.Block.Id, "err", err)
		return nil, err
	}

	return new(metastorev1.AddBlockResponse), nil
}

func SanitizeMetadata(md *metastorev1.BlockMeta) error {
	// TODO(kolesnikovae): Implement and refactor to the block package.
	_, err := ulid.Parse(md.Id)
	return err
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

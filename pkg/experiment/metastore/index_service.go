package metastore

import (
	"context"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/block"
	placement "github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/fsm"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode"
	"github.com/grafana/pyroscope/pkg/iter"
)

type PlacementStats interface {
	RecordStats(iter.Iterator[placement.Sample])
}

func NewIndexService(
	logger log.Logger,
	raft Raft,
	state State,
	index IndexQuerier,
	stats PlacementStats,
) *IndexService {
	return &IndexService{
		logger: logger,
		raft:   raft,
		state:  state,
		index:  index,
		stats:  stats,
	}
}

type IndexService struct {
	metastorev1.IndexServiceServer

	logger log.Logger
	raft   Raft
	state  State
	index  IndexQuerier
	stats  PlacementStats
}

func (svc *IndexService) AddBlock(
	ctx context.Context,
	req *metastorev1.AddBlockRequest,
) (rsp *metastorev1.AddBlockResponse, err error) {
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
) (*metastorev1.AddBlockResponse, error) {
	if err := block.SanitizeMetadata(req.Block); err != nil {
		level.Warn(svc.logger).Log("invalid metadata", "block", block.ID(req.Block), "err", err)
		return nil, err
	}
	_, err := svc.raft.Propose(
		fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_ADD_BLOCK_METADATA),
		&raft_log.AddBlockMetadataRequest{Metadata: req.Block},
	)
	if err != nil {
		level.Error(svc.logger).Log("msg", "failed to add block", "block", block.ID(req.Block), "err", err)
		return nil, err
	}
	return new(metastorev1.AddBlockResponse), nil
}

func (svc *IndexService) GetBlockMetadata(
	ctx context.Context,
	req *metastorev1.GetBlockMetadataRequest,
) (*metastorev1.GetBlockMetadataResponse, error) {
	var found []*metastorev1.BlockMeta
	err := svc.state.ConsistentRead(ctx, func(tx *bbolt.Tx, _ raftnode.ReadIndex) {
		found = svc.index.FindBlocks(tx, req.GetBlocks())
	})
	if err != nil {
		return nil, err
	}
	return &metastorev1.GetBlockMetadataResponse{Blocks: found}, nil
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
		TenantID:    block.Tenant(s.md),
		DatasetName: s.md.StringTable[ds.Name],
		ShardOwner:  s.md.StringTable[s.md.CreatedBy],
		ShardID:     s.md.Shard,
		Size:        ds.Size,
	}
}

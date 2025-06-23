package metastore

import (
	"context"
	goiter "iter"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"go.etcd.io/bbolt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/block/metadata"
	placement "github.com/grafana/pyroscope/pkg/experiment/distributor/placement/adaptive_placement"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/fsm"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/index/cleaner/retention"
	indexstore "github.com/grafana/pyroscope/pkg/experiment/metastore/index/store"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode"
	"github.com/grafana/pyroscope/pkg/iter"
)

type PlacementStats interface {
	RecordStats(iter.Iterator[placement.Sample])
}

type IndexBlockFinder interface {
	GetBlocks(*bbolt.Tx, *metastorev1.BlockList) ([]*metastorev1.BlockMeta, error)
}

type IndexPartitionLister interface {
	// Partitions provide access to all partitions in the index.
	// They are iterated in the order of their creation and are
	// guaranteed to be thread-safe for reads.
	Partitions(*bbolt.Tx) goiter.Seq[indexstore.Partition]
}

type IndexReader interface {
	IndexBlockFinder
	IndexPartitionLister
}

func NewIndexService(
	logger log.Logger,
	raft Raft,
	state State,
	index IndexReader,
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
	index  IndexReader
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
) (*metastorev1.AddBlockResponse, error) {
	if err := metadata.Sanitize(req.Block); err != nil {
		level.Warn(svc.logger).Log("invalid metadata", "block", req.Block.Id, "err", err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	_, err := svc.raft.Propose(
		fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_ADD_BLOCK_METADATA),
		&raft_log.AddBlockMetadataRequest{Metadata: req.Block},
	)
	if err != nil {
		if !raftnode.IsRaftLeadershipError(err) {
			level.Error(svc.logger).Log("msg", "failed to add block", "block", req.Block.Id, "err", err)
		}
		return nil, err
	}
	return new(metastorev1.AddBlockResponse), nil
}

func (svc *IndexService) GetBlockMetadata(
	ctx context.Context,
	req *metastorev1.GetBlockMetadataRequest,
) (resp *metastorev1.GetBlockMetadataResponse, err error) {
	read := func(tx *bbolt.Tx, _ raftnode.ReadIndex) {
		resp, err = svc.getBlockMetadata(tx, req.GetBlocks())
	}
	if readErr := svc.state.ConsistentRead(ctx, read); readErr != nil {
		return nil, status.Error(codes.Unavailable, readErr.Error())
	}
	return resp, err
}

func (svc *IndexService) getBlockMetadata(tx *bbolt.Tx, list *metastorev1.BlockList) (*metastorev1.GetBlockMetadataResponse, error) {
	found, err := svc.index.GetBlocks(tx, list)
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
		TenantID:    s.md.StringTable[ds.Tenant],
		DatasetName: s.md.StringTable[ds.Name],
		ShardOwner:  s.md.StringTable[s.md.CreatedBy],
		ShardID:     s.md.Shard,
		Size:        ds.Size,
	}
}

func (svc *IndexService) TruncateIndex(ctx context.Context, rp retention.Policy) error {
	var req raft_log.TruncateIndexRequest
	read := func(tx *bbolt.Tx, r raftnode.ReadIndex) {
		for p := range svc.index.Partitions(tx) {
			if !rp.Visit(tx, p) {
				break
			}
		}
		req.Tombstones = rp.Tombstones()
		req.Term = r.Term // The leader may change after we read the index.
	}
	if readErr := svc.state.ConsistentRead(ctx, read); readErr != nil {
		return status.Error(codes.Unavailable, readErr.Error())
	}
	if len(req.Tombstones) == 0 {
		return nil
	}
	if _, err := svc.raft.Propose(fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_TRUNCATE_INDEX), &req); err != nil {
		if !raftnode.IsRaftLeadershipError(err) {
			level.Error(svc.logger).Log("msg", "failed to truncate index", "err", err)
		}
		return err
	}
	return nil
}

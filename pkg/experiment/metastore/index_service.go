package metastore

import (
	"context"
	goiter "iter"
	"time"

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
	Partitions() goiter.Seq[*indexstore.Partition]
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

func (svc *IndexService) TruncatePartitions(ctx context.Context, before time.Time, max int) error {
	req := &raft_log.TruncateIndexRequest{Tombstones: make([]*metastorev1.Tombstones, 0, max)}
	read := func(_ *bbolt.Tx, r raftnode.ReadIndex) {
		req.Term = r.Term // See "ABA problem".
		svc.createPartitionTombstones(before, req)
	}
	if readErr := svc.state.ConsistentRead(ctx, read); readErr != nil {
		return status.Error(codes.Unavailable, readErr.Error())
	}
	if len(req.Tombstones) == 0 {
		return nil
	}
	if _, err := svc.raft.Propose(fsm.RaftLogEntryType(raft_log.RaftCommand_RAFT_COMMAND_TRUNCATE_INDEX), req); err != nil {
		if !raftnode.IsRaftLeadershipError(err) {
			level.Error(svc.logger).Log("msg", "failed to truncate index", "err", err)
		}
		return err
	}
	return nil
}

func (svc *IndexService) createPartitionTombstones(before time.Time, req *raft_log.TruncateIndexRequest) {
	m := cap(req.Tombstones)
	var shards []indexstore.Shard
	for p := range svc.index.Partitions() {
		// We pick all partitions that ended before the given time;
		// the partitions cannot contain blocks created after the EndTime.
		// However, the blocks may contain data created after the EndTime:
		// the boundary is determined by the max time delta allowed for
		// compaction – retention policy should include the period.
		// TODO(kolesnikovae):
		//  * We can rely on Max and Min time stored in shard index to
		//    delete based on the data time.
		//  * Tenant-level overrides.
		if !p.EndTime().Before(before) {
			break
		}
		shards = p.Shards(shards)
		for _, shard := range shards {
			if len(req.Tombstones) >= m {
				break
			}
			req.Tombstones = append(req.Tombstones, &metastorev1.Tombstones{
				Partition: &metastorev1.PartitionTombstone{
					Name:      shard.TombstoneName(),
					Timestamp: shard.Partition.Timestamp.UnixNano(),
					Shard:     shard.Shard,
					Tenant:    shard.Tenant,
				},
			})
		}
	}
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

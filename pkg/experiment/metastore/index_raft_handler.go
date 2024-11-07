package metastore

import (
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction"
)

type IndexWriter interface {
	InsertBlock(*bbolt.Tx, ...*metastorev1.BlockMeta) error
	ReplaceBlockMetadata(*bbolt.Tx, *metastorev1.BlockList, ...*metastorev1.BlockMeta) error
}

type IndexCommandHandler struct {
	logger    log.Logger
	index     IndexWriter
	compactor compaction.Compactor
}

func NewIndexCommandHandler(
	logger log.Logger,
	index IndexWriter,
	compactor compaction.Compactor,
) *IndexCommandHandler {
	return &IndexCommandHandler{
		logger:    logger,
		index:     index,
		compactor: compactor,
	}
}

func (m *IndexCommandHandler) AddBlockMetadata(
	tx *bbolt.Tx, cmd *raft.Log, req *raft_log.AddBlockMetadataRequest,
) (*raft_log.AddBlockMetadataResponse, error) {
	// We first try to add the block to the compactor, as it may be already
	// scheduled for compaction or removed after: compactor keeps track of
	// tombstones for objects deleted from the object store.
	if err := m.compactor.AddBlocks(tx, cmd, req.Metadata); err != nil {
		_ = level.Error(m.logger).Log("msg", "failed to add block metadata to compactor", "block", req.Metadata.Id, "err", err)
		return nil, err
	}
	if err := m.index.InsertBlock(tx, req.Metadata); err != nil {
		_ = level.Error(m.logger).Log("msg", "failed to add block metadata to index", "block", req.Metadata.Id, "err", err)
		return nil, err
	}
	return new(raft_log.AddBlockMetadataResponse), nil
}

func (m *IndexCommandHandler) ReplaceBlockMetadata(
	tx *bbolt.Tx, cmd *raft.Log, req *raft_log.ReplaceBlockMetadataRequest,
) (*raft_log.ReplaceBlockMetadataResponse, error) {
	source := &metastorev1.BlockList{
		Tenant: req.Tenant,
		Shard:  req.Shard,
		Blocks: req.SourceBlocks,
	}
	if err := m.compactor.DeleteBlocks(tx, cmd, source); err != nil {
		_ = level.Error(m.logger).Log("msg", "failed to schedule removal of compacted blocks", "err", err)
		return nil, err
	}
	if err := m.compactor.AddBlocks(tx, cmd, req.NewBlocks...); err != nil {
		_ = level.Error(m.logger).Log("msg", "failed to schedule compaction of new blocks", "err", err)
		return nil, err
	}
	if err := m.index.ReplaceBlockMetadata(tx, source, req.NewBlocks...); err != nil {
		_ = level.Error(m.logger).Log("msg", "failed to replace blocks in metadata index", "err", err)
		return nil, err
	}
	return new(raft_log.ReplaceBlockMetadataResponse), nil
}

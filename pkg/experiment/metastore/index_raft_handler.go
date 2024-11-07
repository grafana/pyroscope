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
	// InsertBlock must only insert the new entry after checking if the block
	// is already added or compacted:
	//
	//   if index.Exists() {
	//      if err := compactor.AddBlock(tx, cmd, req.Metadata); err != nil {
	//          if errors.Is(err, compaction.ErrAlreadyCompacted) {
	//              return nil
	//          }
	//      }
	//   }
	//
	// In fact, the block is added to compactor before it is written to the
	// index, as the compactor keeps track of tombstones for objects deleted.
	InsertBlock(*bbolt.Tx, ...*metastorev1.BlockMeta) error

	// ReplaceBlockMetadata swaps the metadata of the source blocks with the
	// new metadata. As in the case with InsertBlock, the writer should check
	// if the blocks are already added or compacted.
	//
	//   compactor.AddBlock(tx, cmd, meta)
	//   compactor.DeleteBlocks(tx, cmd, source)
	//
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
	tx *bbolt.Tx, _ *raft.Log, req *raft_log.AddBlockMetadataRequest,
) (*raft_log.AddBlockMetadataResponse, error) {
	if err := m.index.InsertBlock(tx, req.Metadata); err != nil {
		_ = level.Error(m.logger).Log("msg", "failed to add block metadata to index", "block", req.Metadata.Id, "err", err)
		return nil, err
	}
	return new(raft_log.AddBlockMetadataResponse), nil
}

func (m *IndexCommandHandler) ReplaceBlockMetadata(
	tx *bbolt.Tx, _ *raft.Log, req *raft_log.ReplaceBlockMetadataRequest,
) (*raft_log.ReplaceBlockMetadataResponse, error) {
	source := &metastorev1.BlockList{
		Tenant: req.Tenant,
		Shard:  req.Shard,
		Blocks: req.SourceBlocks,
	}
	if err := m.index.ReplaceBlockMetadata(tx, source, req.NewBlocks...); err != nil {
		_ = level.Error(m.logger).Log("msg", "failed to replace blocks in metadata index", "err", err)
		return nil, err
	}
	return new(raft_log.ReplaceBlockMetadataResponse), nil
}

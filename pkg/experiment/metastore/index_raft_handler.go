package metastore

import (
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/index"
)

type IndexInserter interface {
	FindBlock(tx *bbolt.Tx, shard uint32, tenant string, block string) *metastorev1.BlockMeta
	InsertBlock(*bbolt.Tx, *metastorev1.BlockMeta)
	CreatePartitionKey(string) index.PartitionKey
}

type DeletionMarkChecker interface {
	IsMarked(string) bool
}

type Compactor interface {
	CompactBlock(*bbolt.Tx, *raft.Log, *metastorev1.BlockMeta) error
}

type IndexCommandHandler struct {
	logger    log.Logger
	index     IndexInserter
	marks     DeletionMarkChecker
	compactor Compactor
}

func NewIndexCommandHandler(
	logger log.Logger,
	index IndexInserter,
	marks DeletionMarkChecker,
	compactor Compactor,
) *IndexCommandHandler {
	return &IndexCommandHandler{
		logger:    logger,
		index:     index,
		marks:     marks,
		compactor: compactor,
	}
}

func (m *IndexCommandHandler) AddBlock(tx *bbolt.Tx, cmd *raft.Log, request *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
	if m.marks.IsMarked(request.Block.Id) {
		_ = level.Warn(m.logger).Log("msg", "block already added and compacted", "block_id", request.Block.Id)
		return &metastorev1.AddBlockResponse{}, nil
	}
	if m.index.FindBlock(tx, request.Block.Shard, request.Block.TenantId, request.Block.Id) != nil {
		_ = level.Warn(m.logger).Log("msg", "block already added", "block_id", request.Block.Id)
		return &metastorev1.AddBlockResponse{}, nil
	}

	partKey := m.index.CreatePartitionKey(request.Block.Id)
	err := persistBlock(tx, partKey, request.Block)
	if err == nil {
		err = m.compactor.CompactBlock(tx, cmd, request.Block)
	}
	if err != nil {
		_ = level.Error(m.logger).Log(
			"msg", "failed to add block",
			"block", request.Block.Id,
			"err", err,
		)
		return nil, err
	}
	m.index.InsertBlock(tx, request.Block)
	return &metastorev1.AddBlockResponse{}, nil
}

func persistBlock(tx *bbolt.Tx, partKey index.PartitionKey, block *metastorev1.BlockMeta) error {
	key := []byte(block.Id)
	value, err := block.MarshalVT()
	if err != nil {
		return err
	}
	return index.UpdateBlockMetadataBucket(tx, partKey, block.Shard, block.TenantId, func(bucket *bbolt.Bucket) error {
		return bucket.Put(key, value)
	})
}

package metastore

import (
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/index"
)

type Index interface {
	InsertBlock(*bbolt.Tx, *metastorev1.BlockMeta) error
}

type Tombstones interface {
	Exists(*metastorev1.BlockMeta) bool
}

type Compactor interface {
	Compact(*bbolt.Tx, *raft.Log, *metastorev1.BlockMeta) error
}

type IndexCommandHandler struct {
	logger     log.Logger
	index      Index
	tombstones Tombstones
	compactor  Compactor
}

func NewIndexCommandHandler(
	logger log.Logger,
	index Index,
	tombstones Tombstones,
	compactor Compactor,
) *IndexCommandHandler {
	return &IndexCommandHandler{
		logger:     logger,
		index:      index,
		tombstones: tombstones,
		compactor:  compactor,
	}
}

func (m *IndexCommandHandler) AddBlock(tx *bbolt.Tx, cmd *raft.Log, req *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
	if m.tombstones.Exists(req.Block) {
		level.Warn(m.logger).Log("msg", "block already added and compacted", "block_id", req.Block.Id)
		return new(metastorev1.AddBlockResponse), nil
	}
	if err := m.index.InsertBlock(tx, req.Block); err != nil {
		if errors.Is(err, index.ErrBlockExists) {
			level.Warn(m.logger).Log("msg", "block already added", "block_id", req.Block.Id)
			return new(metastorev1.AddBlockResponse), nil
		}
		level.Error(m.logger).Log("msg", "failed to add block to index", "block_id", req.Block.Id)
		return nil, err
	}
	if err := m.compactor.Compact(tx, cmd, req.Block); err != nil {
		level.Error(m.logger).Log("msg", "failed to add block to compaction", "block", req.Block.Id, "err", err)
		return nil, err
	}
	return &metastorev1.AddBlockResponse{}, nil
}

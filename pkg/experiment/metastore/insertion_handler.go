package metastore

import (
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/fsm"
)

var _ fsm.RaftHandler[*metastorev1.AddBlockRequest, *metastorev1.AddBlockResponse] = (*AddBlockRequestHandler)(nil)

type InserterIndex interface {
	InsertBlock(*bbolt.Tx, *metastorev1.BlockMeta) (bool, error)
}

type Compactor interface {
	CompactBlock(tx *bbolt.Tx, cmd *raft.Log, md *metastorev1.BlockMeta) error
}

type AddBlockRequestHandler struct {
	logger    log.Logger
	index     InserterIndex
	compactor Compactor
}

func NewAddBlockHandler(
	logger log.Logger,
	index InserterIndex,
	compactor Compactor,
) *AddBlockRequestHandler {
	return &AddBlockRequestHandler{
		logger:    logger,
		index:     index,
		compactor: compactor,
	}
}

func (m *AddBlockRequestHandler) Apply(tx *bbolt.Tx, cmd *raft.Log, req *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
	found, err := m.index.InsertBlock(tx, req.Block)
	if err != nil {
		_ = level.Error(m.logger).Log("msg", "failed to add block to index", "block", req.Block.Id, "err", err)
		return nil, err
	}
	if found {
		_ = level.Warn(m.logger).Log("msg", "discarding block duplicate", "block", req.Block.Id)
		return &metastorev1.AddBlockResponse{}, nil
	}
	if err = m.compactor.CompactBlock(tx, cmd, req.Block); err != nil {
		_ = level.Error(m.logger).Log("msg", "failed to add block to compactor", "block", req.Block.Id, "err", err)
		return nil, err
	}
	return &metastorev1.AddBlockResponse{}, nil
}

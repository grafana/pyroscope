package metastore

import (
	"errors"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1/raft_log"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction"
)

type IndexInserter interface {
	Exists(*bbolt.Tx, *metastorev1.BlockMeta) error
	InsertBlock(*bbolt.Tx, ...*metastorev1.BlockMeta) error
}

type IndexCommandHandler struct {
	logger    log.Logger
	index     IndexInserter
	compactor compaction.Compactor
}

func NewIndexCommandHandler(
	logger log.Logger,
	index IndexInserter,
	compactor compaction.Compactor,
) *IndexCommandHandler {
	return &IndexCommandHandler{
		logger:    logger,
		index:     index,
		compactor: compactor,
	}
}

var ErrExists = errors.New("block already exists in the index")

func (m *IndexCommandHandler) AddBlockMetadata(
	tx *bbolt.Tx, cmd *raft.Log, req *raft_log.AddBlockMetadataRequest,
) (*raft_log.AddBlockMetadataResponse, error) {
	if err := m.index.Exists(tx, req.Metadata); err != nil {
		if errors.Is(err, ErrExists) { // TODO: index.ErrExists
			_ = level.Warn(m.logger).Log("msg", "block already exists in the index", "block", req.Metadata.Id)
			return new(raft_log.AddBlockMetadataResponse), nil
		}
		_ = level.Error(m.logger).Log("msg", "failed to lookup metadata in index", "block", req.Metadata.Id, "err", err)
		return nil, err
	}

	if err := m.compactor.AddBlock(tx, cmd, req.Metadata); err != nil {
		_ = level.Error(m.logger).Log("msg", "failed to lookup metadata in compactor", "block", req.Metadata.Id, "err", err)
		return nil, err
	}

	if err := m.index.InsertBlock(tx, req.Metadata); err != nil {
		_ = level.Error(m.logger).Log("msg", "failed to add block metadata to index", "block", req.Metadata.Id, "err", err)
		return nil, err
	}

	return new(raft_log.AddBlockMetadataResponse), nil
}

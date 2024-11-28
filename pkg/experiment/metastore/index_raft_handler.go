package metastore

import (
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/hashicorp/raft"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/compaction"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/index"
)

type Index interface {
	InsertBlock(*bbolt.Tx, *metastorev1.BlockMeta) error
}

type Tombstones interface {
	Exists(tenant string, shard uint32, block string) bool
}

type IndexCommandHandler struct {
	logger     log.Logger
	index      Index
	tombstones Tombstones
	compactor  compaction.Compactor
}

func NewIndexCommandHandler(
	logger log.Logger,
	index Index,
	tombstones Tombstones,
	compactor compaction.Compactor,
) *IndexCommandHandler {
	return &IndexCommandHandler{
		logger:     logger,
		index:      index,
		tombstones: tombstones,
		compactor:  compactor,
	}
}

func (m *IndexCommandHandler) AddBlock(tx *bbolt.Tx, cmd *raft.Log, req *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
	e := compaction.NewBlockEntry(cmd, req.Block)
	if m.tombstones.Exists(e.Tenant, e.Shard, e.ID) {
		level.Warn(m.logger).Log("msg", "block already added and compacted", "block", e.ID)
		return new(metastorev1.AddBlockResponse), nil
	}
	if err := m.index.InsertBlock(tx, req.Block); err != nil {
		if errors.Is(err, index.ErrBlockExists) {
			level.Warn(m.logger).Log("msg", "block already added", "block", e.ID)
			return new(metastorev1.AddBlockResponse), nil
		}
		level.Error(m.logger).Log("msg", "failed to add block to index", "block", e.ID, "err", err)
		return nil, err
	}
	if err := m.compactor.Compact(tx, e); err != nil {
		level.Error(m.logger).Log("msg", "failed to add block to compaction", "block", e.ID, "err", err)
		return nil, err
	}
	return new(metastorev1.AddBlockResponse), nil
}

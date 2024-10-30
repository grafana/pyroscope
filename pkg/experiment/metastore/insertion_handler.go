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
	FindBlock(shard uint32, tenant, block string) *metastorev1.BlockMeta
	InsertBlock(*bbolt.Tx, *metastorev1.BlockMeta)
}

type Compactor interface {
	CompactBlock(*bbolt.Tx, *raft.Log, *metastorev1.BlockMeta) error
}

type DeletionMarkers interface {
	IsMarked(block string) bool
}

type AddBlockRequestHandler struct {
	logger    log.Logger
	markers   DeletionMarkers
	index     InserterIndex
	compactor Compactor
}

func NewAddBlockHandler(
	logger log.Logger,
	markers DeletionMarkers,
	index InserterIndex,
	compactor Compactor,
) *AddBlockRequestHandler {
	return &AddBlockRequestHandler{
		logger:    logger,
		markers:   markers,
		index:     index,
		compactor: compactor,
	}
}

func (m *AddBlockRequestHandler) Apply(tx *bbolt.Tx, cmd *raft.Log, req *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
	if m.markers.IsMarked(req.Block.Id) {
		_ = level.Warn(m.logger).Log("msg", "block already added and compacted", "block_id", req.Block.Id)
		return &metastorev1.AddBlockResponse{}, nil
	}
	if m.index.FindBlock(req.Block.Shard, req.Block.TenantId, req.Block.Id) != nil {
		_ = level.Warn(m.logger).Log("msg", "block already added", "block_id", req.Block.Id)
		return &metastorev1.AddBlockResponse{}, nil
	}
	if err := m.compactor.CompactBlock(tx, cmd, req.Block); err != nil {
		_ = level.Error(m.logger).Log("msg", "failed to add block to compactor", "block", req.Block.Id, "err", err)
		return nil, err
	}
	m.index.InsertBlock(tx, req.Block)
	return &metastorev1.AddBlockResponse{}, nil
}

/*

func (m *Metastore) AddRecoveredBlock(_ context.Context, req *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
	l := log.With(m.logger, "shard", req.Block.Shard, "block_id", req.Block.Id, "ts", req.Block.MinTime)
	_ = level.Info(l).Log("msg", "adding recovered block")
	t1 := time.Now()
	defer func() {
		m.metrics.raftAddRecoveredBlockDuration.Observe(time.Since(t1).Seconds())
	}()
	_, resp, err := proposeCommand[*metastorev1.AddBlockRequest, *metastorev1.AddBlockResponse](m.raft, 0, req, m.config.Raft.ApplyTimeout)
	if err != nil {
		_ = level.Error(l).Log("msg", "failed to apply add recovered block", "err", err)
	}
	return resp, err
}

*/

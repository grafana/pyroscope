package metastore

import (
	"context"

	"github.com/go-kit/log/level"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

func (m *Metastore) AddBlock(_ context.Context, req *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
	_ = level.Info(m.logger).Log("msg", "AddBlock called")
	_, resp, err := applyCommand[*metastorev1.AddBlockRequest, *metastorev1.AddBlockResponse](m.raft, req, m.config.Raft.ApplyTimeout)
	if err != nil {
		_ = level.Error(m.logger).Log("msg", "AddBlock failed", "err", err)
		return nil, err
	}
	return resp, nil
}

func (m *metastoreState) applyAddBlock(request *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
	_ = level.Info(m.logger).Log("msg", "adding block", "block_id", request.Block.Id)
	if request.Block.CompactionLevel == 0 {
		m.getOrCreateShard(request.Block.Shard).put(request.Block)
	} else {
		_ = level.Error(m.logger).Log("msg", "compaction not implemented, ignoring block", "block", request.Block.Id)
	}
	return &metastorev1.AddBlockResponse{}, nil
}

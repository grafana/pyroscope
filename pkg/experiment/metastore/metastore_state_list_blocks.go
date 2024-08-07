package metastore

import (
	"context"

	"github.com/go-kit/log/level"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
)

// TODO(kolesnikovae): Remove.

func (m *Metastore) ListBlocks(ctx context.Context, request *metastorev1.ListBlocksRequest) (*metastorev1.ListBlocksResponse, error) {
	_ = level.Info(m.logger).Log("msg", "ListBlocks called")
	return &metastorev1.ListBlocksResponse{}, nil
}

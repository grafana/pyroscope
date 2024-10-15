package metastore

import (
	"context"
	"math"
	"sync"

	"github.com/go-kit/log/level"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/index"
)

func (m *Metastore) GetProfileStats(
	ctx context.Context,
	r *metastorev1.GetProfileStatsRequest,
) (*typesv1.GetProfileStatsResponse, error) {
	if err := m.waitLeaderCommitIndexAppliedLocally(ctx); err != nil {
		level.Error(m.logger).Log("msg", "failed to wait for leader commit index", "err", err)
		return nil, err
	}
	return m.state.getProfileStats(r.TenantId, ctx)
}

func (m *metastoreState) getProfileStats(tenant string, ctx context.Context) (*typesv1.GetProfileStatsResponse, error) {
	var respMutex sync.Mutex
	resp := typesv1.GetProfileStatsResponse{
		DataIngested:      false,
		OldestProfileTime: math.MaxInt64,
		NewestProfileTime: math.MinInt64,
	}
	err := m.index.ForEachPartition(ctx, func(p *index.PartitionMeta) error {
		if !p.HasTenant(tenant) {
			return nil
		}
		oldest := p.StartTime().UnixMilli()
		newest := p.EndTime().UnixMilli()
		respMutex.Lock()
		defer respMutex.Unlock()
		resp.DataIngested = true
		if oldest < resp.OldestProfileTime {
			resp.OldestProfileTime = oldest
		}
		if newest > resp.NewestProfileTime {
			resp.NewestProfileTime = newest
		}
		return nil
	})

	return &resp, err
}

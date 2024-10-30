package metastore

import (
	"context"
	"math"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/index"
)

type PartitionIndexIterator interface {
	ForEachPartition(ctx context.Context, f func(*index.PartitionMeta) error) error
}

type TenantService struct {
	logger         log.Logger
	partitionIndex PartitionIndexIterator
	raftFollower   RaftFollower
}

func NewTenantService(
	logger log.Logger,
	partitionIndex PartitionIndexIterator,
	raftFollower RaftFollower,
) *TenantService {
	return &TenantService{
		logger:         logger,
		partitionIndex: partitionIndex,
		raftFollower:   raftFollower,
	}
}

func (svc *TenantService) GetTenant(
	ctx context.Context,
	r *metastorev1.GetTenantRequest,
) (*metastorev1.GetTenantResponse, error) {
	if err := svc.raftFollower.WaitLeaderCommitIndexAppliedLocally(ctx); err != nil {
		level.Error(svc.logger).Log("msg", "failed to wait for leader commit index", "err", err)
		return nil, err
	}
	return svc.getTenantStats(r.TenantId, ctx)
}

func (svc *TenantService) getTenantStats(tenant string, ctx context.Context) (*metastorev1.GetTenantResponse, error) {
	var respMutex sync.Mutex
	stats := &metastorev1.TenantStats{
		DataIngested:      false,
		OldestProfileTime: math.MaxInt64,
		NewestProfileTime: math.MinInt64,
	}
	err := svc.partitionIndex.ForEachPartition(ctx, func(p *index.PartitionMeta) error {
		if !p.HasTenant(tenant) {
			return nil
		}
		oldest := p.StartTime().UnixMilli()
		newest := p.EndTime().UnixMilli()
		respMutex.Lock()
		defer respMutex.Unlock()
		stats.DataIngested = true
		if oldest < stats.OldestProfileTime {
			stats.OldestProfileTime = oldest
		}
		if newest > stats.NewestProfileTime {
			stats.NewestProfileTime = newest
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &metastorev1.GetTenantResponse{Stats: stats}, nil
}

func (svc *TenantService) DeleteTenant(
	context.Context,
	*metastorev1.DeleteTenantRequest,
) (*metastorev1.DeleteTenantResponse, error) {
	// TODO(kolesnikovae): Implement.
	return new(metastorev1.DeleteTenantResponse), nil
}

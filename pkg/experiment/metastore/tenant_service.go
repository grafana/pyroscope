package metastore

import (
	"context"
	"math"
	"sync"

	"github.com/go-kit/log"
	"go.etcd.io/bbolt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/index"
)

// TODO(kolesnikovae): The service should not know
//  about partitions and the index implementation.

type TenantService struct {
	metastorev1.TenantServiceServer

	logger log.Logger
	state  State
	index  IndexQuerier
}

func NewTenantService(
	logger log.Logger,
	state State,
	index IndexQuerier,
) *TenantService {
	return &TenantService{
		logger: logger,
		state:  state,
		index:  index,
	}
}

func (svc *TenantService) GetTenant(
	ctx context.Context,
	req *metastorev1.GetTenantRequest,
) (resp *metastorev1.GetTenantResponse, err error) {
	read := func(_ *bbolt.Tx) {
		// Although we're not using transaction here, we need to ensure
		// strong consistency of the read operation.
		resp, err = svc.getTenantStats(req.TenantId, ctx)
	}
	if readErr := svc.state.ConsistentRead(ctx, read); readErr != nil {
		return nil, status.Error(codes.Unavailable, readErr.Error())
	}
	return resp, err
}

func (svc *TenantService) getTenantStats(tenant string, ctx context.Context) (*metastorev1.GetTenantResponse, error) {
	var respMutex sync.Mutex
	stats := &metastorev1.TenantStats{
		DataIngested:      false,
		OldestProfileTime: math.MaxInt64,
		NewestProfileTime: math.MinInt64,
	}
	err := svc.index.ForEachPartition(ctx, func(p *index.PartitionMeta) error {
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

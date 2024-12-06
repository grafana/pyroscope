package metastore

import (
	"context"

	"github.com/go-kit/log"
	"go.etcd.io/bbolt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/experiment/metastore/raftnode"
)

type TenantIndex interface {
	GetTenantStats(tenant string) *metastorev1.TenantStats
}

type TenantService struct {
	metastorev1.TenantServiceServer

	logger log.Logger
	state  State
	index  TenantIndex
}

func NewTenantService(
	logger log.Logger,
	state State,
	index TenantIndex,
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
	read := func(*bbolt.Tx, raftnode.ReadIndex) {
		resp = &metastorev1.GetTenantResponse{Stats: svc.index.GetTenantStats(req.TenantId)}
	}
	if readErr := svc.state.ConsistentRead(ctx, read); readErr != nil {
		return nil, status.Error(codes.Unavailable, readErr.Error())
	}
	return resp, err
}

func (svc *TenantService) DeleteTenant(
	context.Context,
	*metastorev1.DeleteTenantRequest,
) (*metastorev1.DeleteTenantResponse, error) {
	// TODO(kolesnikovae): Implement.
	return new(metastorev1.DeleteTenantResponse), nil
}

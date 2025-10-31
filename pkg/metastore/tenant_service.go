package metastore

import (
	"context"

	"github.com/go-kit/log"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"go.etcd.io/bbolt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/metastore/raftnode"
)

type TenantIndex interface {
	GetTenants(tx *bbolt.Tx) []string
	GetTenantStats(tx *bbolt.Tx, tenant string) *metastorev1.TenantStats
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

func (svc *TenantService) GetTenants(
	ctx context.Context,
	_ *metastorev1.GetTenantsRequest,
) (resp *metastorev1.GetTenantsResponse, err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "TenantService.GetTenants")
	defer func() {
		if err != nil {
			ext.LogError(span, err)
		}
		span.Finish()
	}()

	read := func(tx *bbolt.Tx, _ raftnode.ReadIndex) {
		resp = &metastorev1.GetTenantsResponse{TenantIds: svc.index.GetTenants(tx)}
	}
	if readErr := svc.state.ConsistentRead(ctx, read); readErr != nil {
		return nil, status.Error(codes.Unavailable, readErr.Error())
	}
	span.SetTag("tenant_count", len(resp.GetTenantIds()))
	return resp, err
}

func (svc *TenantService) GetTenant(
	ctx context.Context,
	req *metastorev1.GetTenantRequest,
) (resp *metastorev1.GetTenantResponse, err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "TenantService.GetTenant")
	defer func() {
		if err != nil {
			ext.LogError(span, err)
		}
		span.Finish()
	}()

	span.SetTag("tenant_id", req.GetTenantId())

	read := func(tx *bbolt.Tx, _ raftnode.ReadIndex) {
		resp = &metastorev1.GetTenantResponse{Stats: svc.index.GetTenantStats(tx, req.TenantId)}
	}
	if readErr := svc.state.ConsistentRead(ctx, read); readErr != nil {
		return nil, status.Error(codes.Unavailable, readErr.Error())
	}
	if stats := resp.GetStats(); stats != nil {
		span.SetTag("data_ingested", stats.GetDataIngested())
		span.SetTag("oldest_profile_time", stats.GetOldestProfileTime())
		span.SetTag("newest_profile_time", stats.GetNewestProfileTime())
	} else {
		span.SetTag("data_ingested", false)
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

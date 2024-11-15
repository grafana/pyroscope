package query_frontend

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

func (q *QueryFrontend) GetProfileStats(
	ctx context.Context,
	_ *connect.Request[typesv1.GetProfileStatsRequest],
) (*connect.Response[typesv1.GetProfileStatsResponse], error) {
	tenants, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if len(tenants) != 1 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("expected 1 tenant, got %d", len(tenants)))
	}

	resp, err := q.tenantServiceClient.GetTenant(ctx, &metastorev1.GetTenantRequest{
		TenantId: tenants[0],
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	stats := resp.GetStats()
	return connect.NewResponse(&typesv1.GetProfileStatsResponse{
		DataIngested:      stats.DataIngested,
		OldestProfileTime: stats.OldestProfileTime,
		NewestProfileTime: stats.NewestProfileTime,
	}), nil
}

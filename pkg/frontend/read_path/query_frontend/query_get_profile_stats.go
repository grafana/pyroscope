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
	req *connect.Request[typesv1.GetProfileStatsRequest],
) (*connect.Response[typesv1.GetProfileStatsResponse], error) {

	tenants, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if len(tenants) != 1 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("expected 1 tenant, got %d", len(tenants)))
	}

	stats, err := q.metastore.GetProfileStats(ctx, &metastorev1.GetProfileStatsRequest{
		TenantId: tenants[0],
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(stats), nil
}

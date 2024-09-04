package query_frontend

import (
	"context"
	"sort"

	"connectrpc.com/connect"
	"github.com/grafana/dskit/tenant"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/validation"
)

func (q *QueryFrontend) ProfileTypes(
	ctx context.Context,
	req *connect.Request[querierv1.ProfileTypesRequest],
) (*connect.Response[querierv1.ProfileTypesResponse], error) {

	tenants, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	empty, err := validation.SanitizeTimeRange(q.limits, tenants, &req.Msg.Start, &req.Msg.End)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if empty {
		return connect.NewResponse(&querierv1.ProfileTypesResponse{}), nil
	}

	md, err := q.metastore.QueryMetadata(ctx, &metastorev1.QueryMetadataRequest{
		TenantId:  tenants,
		StartTime: req.Msg.Start,
		EndTime:   req.Msg.End,
		Query:     "{}",
	})

	if err != nil {
		return nil, err
	}

	pTypesFromMetadata := make(map[string]*typesv1.ProfileType)
	for _, b := range md.Blocks {
		for _, d := range b.Datasets {
			for _, pType := range d.ProfileTypes {
				if _, ok := pTypesFromMetadata[pType]; !ok {
					profileType, err := phlaremodel.ParseProfileTypeSelector(pType)
					if err != nil {
						return nil, err
					}
					pTypesFromMetadata[pType] = profileType
				}
			}
		}
	}

	var profileTypes []*typesv1.ProfileType
	for _, pType := range pTypesFromMetadata {
		profileTypes = append(profileTypes, pType)
	}

	sort.Slice(profileTypes, func(i, j int) bool {
		return profileTypes[i].ID < profileTypes[j].ID
	})

	return connect.NewResponse(&querierv1.ProfileTypesResponse{
		ProfileTypes: profileTypes,
	}), nil
}

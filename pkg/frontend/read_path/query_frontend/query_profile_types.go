package query_frontend

import (
	"context"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/go-kit/log/level"
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

	resp, err := q.metadataQueryClient.QueryMetadataLabels(ctx, &metastorev1.QueryMetadataLabelsRequest{
		TenantId:  tenants,
		StartTime: req.Msg.Start,
		EndTime:   req.Msg.End,
		Query:     "{}",
		Labels:    []string{phlaremodel.LabelNameProfileType},
	})

	if err != nil {
		return nil, err
	}

	types := make([]*typesv1.ProfileType, 0, len(resp.Labels))
	for _, ls := range resp.Labels {
		var typ *typesv1.ProfileType
		if len(ls.Labels) == 1 && ls.Labels[0].Name == phlaremodel.LabelNameProfileType {
			typ, err = phlaremodel.ParseProfileTypeSelector(ls.Labels[0].Value)
		}
		if err != nil || typ == nil {
			level.Warn(q.logger).Log("msg", "malformed label set", "labels", phlaremodel.LabelPairsString(ls.Labels))
			continue
		}
		types = append(types, typ)
	}

	slices.SortFunc(types, func(a, b *typesv1.ProfileType) int {
		return strings.Compare(a.ID, b.ID)
	})

	return connect.NewResponse(&querierv1.ProfileTypesResponse{ProfileTypes: types}), nil
}

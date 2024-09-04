package query_frontend

import (
	"context"
	"sort"

	"connectrpc.com/connect"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

func (q *QueryFrontend) ProfileTypes(
	ctx context.Context,
	req *connect.Request[querierv1.ProfileTypesRequest],
) (*connect.Response[querierv1.ProfileTypesResponse], error) {

	lblReq := connect.NewRequest(&typesv1.LabelValuesRequest{
		Start:    req.Msg.Start,
		End:      req.Msg.End,
		Matchers: []string{"{}"},
		Name:     phlaremodel.LabelNameProfileType,
	})

	lblRes, err := q.LabelValues(ctx, lblReq)
	if err != nil {
		return nil, err
	}

	var profileTypes []*typesv1.ProfileType

	for _, profileTypeStr := range lblRes.Msg.Names {
		profileType, err := phlaremodel.ParseProfileTypeSelector(profileTypeStr)
		if err != nil {
			return nil, err
		}
		profileTypes = append(profileTypes, profileType)
	}

	sort.Slice(profileTypes, func(i, j int) bool {
		return profileTypes[i].ID < profileTypes[j].ID
	})

	return connect.NewResponse(&querierv1.ProfileTypesResponse{
		ProfileTypes: profileTypes,
	}), nil
}

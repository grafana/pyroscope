package frontend

import (
	"context"
	"github.com/go-kit/log/level"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"sort"

	"connectrpc.com/connect"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/common/model"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
)

func (f *Frontend) ProfileTypes(ctx context.Context, c *connect.Request[querierv1.ProfileTypesRequest]) (*connect.Response[querierv1.ProfileTypesResponse], error) {
	opentracing.SpanFromContext(ctx).
		SetTag("start", model.Time(c.Msg.Start).Time().String()).
		SetTag("end", model.Time(c.Msg.End).Time().String())

	lblRes, err := f.LabelValues(ctx, &connect.Request[typesv1.LabelValuesRequest]{
		Msg: &typesv1.LabelValuesRequest{
			Start:    c.Msg.Start,
			End:      c.Msg.End,
			Matchers: []string{"{}"},
			Name:     phlaremodel.LabelNameProfileType,
		},
	})
	if err != nil {
		return nil, err
	}

	var profileTypes []*typesv1.ProfileType
	for _, profileTypeStr := range lblRes.Msg.Names {
		profileType, err := phlaremodel.ParseProfileTypeSelector(profileTypeStr)
		if err != nil {
			level.Error(f.log).Log("msg", "ProfileTypes: failed to parse profile type", "profileType", profileTypeStr, "err", err)
			continue
		}
		profileTypes = append(profileTypes, profileType)
	}

	sort.Slice(profileTypes, func(i, j int) bool {
		return profileTypes[i].ID < profileTypes[j].ID
	})
	return connect.NewResponse(&querierv1.ProfileTypesResponse{ProfileTypes: profileTypes}), nil
}

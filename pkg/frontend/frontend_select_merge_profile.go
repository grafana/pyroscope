package frontend

import (
	"context"
	"net/http"

	"github.com/bufbuild/connect-go"
	"github.com/prometheus/common/model"

	"github.com/grafana/dskit/tenant"

	profilev1 "github.com/grafana/phlare/api/gen/proto/go/google/v1"
	querierv1 "github.com/grafana/phlare/api/gen/proto/go/querier/v1"
	"github.com/grafana/phlare/pkg/util/connectgrpc"
	"github.com/grafana/phlare/pkg/validation"
)

func (f *Frontend) SelectMergeProfile(ctx context.Context, c *connect.Request[querierv1.SelectMergeProfileRequest]) (*connect.Response[profilev1.Profile], error) {
	tenantIDs, err := tenant.TenantIDs(ctx)
	if err != nil {
		return nil, connect.NewError(http.StatusBadRequest, err)
	}
	validated, err := validation.ValidateRangeRequest(f.limits, tenantIDs, model.Interval{Start: model.Time(c.Msg.Start), End: model.Time(c.Msg.End)}, model.Now())
	if err != nil {
		return nil, connect.NewError(http.StatusBadRequest, err)
	}
	if validated.IsEmpty {
		return connect.NewResponse(&profilev1.Profile{}), nil
	}
	c.Msg.Start = int64(validated.Start)
	c.Msg.End = int64(validated.End)
	return connectgrpc.RoundTripUnary[querierv1.SelectMergeProfileRequest, profilev1.Profile](ctx, f, c)
}

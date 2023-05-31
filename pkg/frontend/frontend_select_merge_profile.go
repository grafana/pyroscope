package frontend

import (
	"context"

	"github.com/bufbuild/connect-go"

	profilev1 "github.com/grafana/phlare/api/gen/proto/go/google/v1"
	querierv1 "github.com/grafana/phlare/api/gen/proto/go/querier/v1"
	"github.com/grafana/phlare/pkg/util/connectgrpc"
)

func (f *Frontend) SelectMergeProfile(ctx context.Context, c *connect.Request[querierv1.SelectMergeProfileRequest]) (*connect.Response[profilev1.Profile], error) {
	return connectgrpc.RoundTripUnary[querierv1.SelectMergeProfileRequest, profilev1.Profile](ctx, f, c)
}

package frontend

import (
	"context"

	"connectrpc.com/connect"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

// GetProfile is a stub for v1 frontend compatibility.
// This feature only implemented in v2 architecture.
func (f *Frontend) GetProfile(
	ctx context.Context,
	req *connect.Request[querierv1.GetProfileRequest],
) (*connect.Response[querierv1.GetProfileResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

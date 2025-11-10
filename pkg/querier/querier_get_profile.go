package querier

import (
	"context"

	"connectrpc.com/connect"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

// GetProfile is a stub for v1 architecture compatibility.
// This feature is only implemented in v2 architecture.
func (q *Querier) GetProfile(
	ctx context.Context,
	req *connect.Request[querierv1.GetProfileRequest],
) (*connect.Response[querierv1.GetProfileResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

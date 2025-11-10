package queryfrontend

import (
	"context"

	"connectrpc.com/connect"

	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
)

func (q *QueryFrontend) GetProfile(
	ctx context.Context,
	req *connect.Request[querierv1.GetProfileRequest],
) (*connect.Response[querierv1.GetProfileResponse], error) {
	// TODO: Implement profile retrieval by UUID for v2 architecture
	// This should:
	// 1. Query blocks via metastore for rows matching the UUID
	// 2. Merge split profiles if necessary (using pprof.ProfileMerge)
	// 3. Return the complete profile
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}
